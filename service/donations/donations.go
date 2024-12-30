package donations

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
)

type DonationService struct {
	donationStore    donationStore
	documentStorage  documentStorage
	paymentsProvider PaymentsProvider

	reportBuckets []string

	logger *slog.Logger
}

func NewDonationService(donationStore donationStore, documentStorage documentStorage, provider PaymentsProvider, reportBuckets []string, logger *slog.Logger) *DonationService {
	return &DonationService{
		donationStore:    donationStore,
		documentStorage:  documentStorage,
		paymentsProvider: provider,
		logger:           logger,
		reportBuckets:    reportBuckets,
	}
}

func (s DonationService) GetTotalDonatedByFund(ctx context.Context, id uuid.UUID) (int64, error) {
	total, err := s.donationStore.GetTotalDonatedByFundID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get total donated by fund id", slog.String("error", err.Error()))

		return 0, err
	}

	return total, nil
}

func (s DonationService) ListActiveFunds(ctx context.Context) ([]Fund, error) {
	onceFunds, err := s.donationStore.GetActiveFunds(ctx, "once")
	if err != nil {
		s.logger.Error("failed to get active funds", slog.String("error", err.Error()))

		return nil, err
	}

	recurringFunds, err := s.donationStore.GetActiveFunds(ctx, "monthly")
	if err != nil {
		s.logger.Error("failed to get active funds", slog.String("error", err.Error()))

		return nil, err
	}

	funds := append(onceFunds, recurringFunds...)

	for _, fund := range funds {
		monthly, err := s.donationStore.GetMonthlyDonationTotalsForFund(ctx, fund.ID)
		if err != nil {
			s.logger.Error("failed to get monthly donation totals for fund", slog.String("error", err.Error()))

			return nil, err
		}

		fund.Stats.Monthly = monthly
	}

	return funds, nil
}

func (s DonationService) DeactivateFund(ctx context.Context, id uuid.UUID) error {
	deactivated, err := s.donationStore.SetFundAndDonationsToInactive(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate fund", slog.String("error", err.Error()))

		return err
	}

	toCancel := extractProviderSubscriptionIDs(deactivated)

	cancelled, err := s.paymentsProvider.CancelSubscriptions(ctx, toCancel)
	if err != nil {
		s.logger.Error("failed to cancel subscriptions", slog.String("error", err.Error()))

		return err
	}

	if len(cancelled) != len(toCancel) {
		uncancelled := uncancelledSubscriptions(cancelled, toCancel)
		s.logger.Error("failed to cancel all subscriptions", slog.String("uncancelled", fmt.Sprintf("%v", uncancelled)))

		for _, sub := range uncancelled {
			_, err = s.donationStore.SetDonationToActiveBySubscriptionID(ctx, sub)
			if err != nil {
				s.logger.Error("failed to reactivate donation", slog.String("error", err.Error()))

				return err
			}
		}
	}

	return nil
}

func (s DonationService) DeactivateDonation(ctx context.Context, id uuid.UUID, reason string) (*Donation, error) {
	donation, err := s.donationStore.SetDonationToInactive(ctx, DeactivateDonation{
		ID:     id,
		Reason: reason,
	})
	if err != nil {
		s.logger.Error("failed to set donation to inactive", slog.String("error", err.Error()))

		return nil, err
	}

	return donation, nil
}

func (s DonationService) ListFunds(ctx context.Context) ([]Fund, error) {
	funds, err := s.donationStore.GetFunds(ctx)
	if err != nil {
		s.logger.Error("failed to list funds", slog.String("error", err.Error()))

		return nil, err
	}

	return funds, nil
}

func (s DonationService) GetFundByID(ctx context.Context, id uuid.UUID) (*Fund, error) {
	fund, err := s.donationStore.GetFundByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get fund by id", slog.String("error", err.Error()))

		return nil, err
	}

	monthly, err := s.donationStore.GetMonthlyDonationTotalsForFund(ctx, fund.ID)
	if err != nil {
		s.logger.Error("failed to get monthly donation totals for fund", slog.String("error", err.Error()))

		return nil, err
	}

	fund.Stats.Monthly = monthly

	return fund, nil
}

func (s DonationService) CreateDonationPlan(ctx context.Context, plan CreatePlan) (*DonationPlan, error) {
	providerID, err := s.paymentsProvider.CreatePlan(ctx, plan)
	if err != nil {
		s.logger.Error("failed to create plan with provider", slog.String("error", err.Error()))

		return nil, err
	}

	upsertPlan := UpsertDonationPlan{
		ID:             uuid.New(),
		Name:           plan.Name,
		ProviderPlanID: providerID,
		AmountCents:    plan.AmountCents,
		IntervalUnit:   plan.IntervalUnit,
		IntervalCount:  plan.IntervalCount,
		Active:         true,
		FundID:         plan.FundID,
	}

	planOut, err := s.donationStore.UpsertDonationPlan(ctx, upsertPlan)
	if err != nil {
		s.logger.Error("failed to upsert donation plan", slog.String("error", err.Error()))

		return nil, err
	}

	return planOut, nil
}

func (s DonationService) CompleteRecurringDonation(ctx context.Context, memberID uuid.UUID, completion RecurringCompletion) error {
	insertDonation := InsertDonation{
		ID:                     uuid.New(),
		DonorID:                memberID,
		PlanID:                 completion.PlanID,
		FundID:                 completion.FundID,
		ProviderOrderID:        completion.ProviderOrderID,
		ProviderSubscriptionID: completion.ProviderSubscriptionID,
		Recurring:              true,
	}

	_, err := s.donationStore.InsertDonation(ctx, insertDonation)
	if err != nil {
		s.logger.Error("failed to insert donation", slog.String("error", err.Error()))

		return err
	}

	return nil
}

func (s DonationService) InitiateDonation(ctx context.Context, fundID uuid.UUID, amountCents int32) (string, error) {
	fund, err := s.donationStore.GetFundByID(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to get fund by id", slog.String("error", err.Error()))

		return "", err
	}

	orderID, err := s.paymentsProvider.InitiateDonation(ctx, *fund, amountCents)
	if err != nil {
		s.logger.Error("failed to initiate donation with provider", slog.String("error", err.Error()))

		return "", err
	}

	return orderID, nil
}

func (s DonationService) CompleteDonation(ctx context.Context, memberID uuid.UUID, completion OneTimeCompletion) error {
	insertDonation := InsertDonation{
		ID:              uuid.New(),
		DonorID:         memberID,
		FundID:          completion.FundID,
		ProviderOrderID: completion.ProviderOrderID,
	}

	insertPayment := InsertDonationPayment{
		ID:                uuid.New(),
		AmountCents:       completion.AmountCents,
		ProviderPaymentID: completion.ProviderPaymentID,
		DonationID:        insertDonation.ID,
	}

	_, err := s.donationStore.InsertDonationWithPayment(ctx, insertDonation, insertPayment)
	if err != nil {
		s.logger.Error("failed to create donation with payment", slog.String("error", err.Error()))

		return err
	}

	return nil
}

func (s DonationService) CreateFund(ctx context.Context, createFund Fund) (*Fund, error) {
	providerID, err := s.paymentsProvider.CreateFund(ctx, createFund.Name, createFund.Description)
	if err != nil {
		s.logger.Error("failed to create fund with provider", slog.String("error", err.Error()))

		return nil, err
	}

	insertFund := InsertFund{
		ID:              uuid.New(),
		Name:            createFund.Name,
		Description:     createFund.Description,
		ProviderID:      providerID,
		PayoutFrequency: string(createFund.PayoutFrequency),
		GoalCents:       createFund.GoalCents,
		Expires:         createFund.Expires,
		Active:          true,
		ProviderName:    "paypal",
	}

	fund, err := s.donationStore.InsertFund(ctx, insertFund)
	if err != nil {
		s.logger.Error("failed to insert fund", slog.String("error", err.Error()))

		return nil, err
	}

	err = s.createFundBuckets(ctx, fund.ID)
	if err != nil {
		s.logger.Error("failed to create fund buckets", slog.String("error", err.Error()))

		return nil, err
	}

	return fund, nil
}

func (s DonationService) UpdateFund(ctx context.Context, updateFund Fund) (*Fund, error) {
	update := UpdateFund{
		ID:              updateFund.ID,
		Name:            updateFund.Name,
		Description:     updateFund.Description,
		Active:          updateFund.Active,
		GoalCents:       updateFund.GoalCents,
		PayoutFrequency: string(updateFund.PayoutFrequency),
		Expires:         updateFund.Expires,
	}

	fund, err := s.donationStore.UpdateFund(ctx, update)
	if err != nil {
		s.logger.Error("failed to update fund", slog.String("error", err.Error()))

		return nil, err
	}

	return fund, nil
}

func (s DonationService) createFundBuckets(ctx context.Context, fundID uuid.UUID) error {
	for _, prefix := range s.reportBuckets {
		err := s.documentStorage.CreateFundBucket(ctx, prefix, fundID)
		if err != nil {
			s.logger.Error("failed to create fund bucket", slog.String("error", err.Error()))
		}
	}

	return nil
}

func extractProviderSubscriptionIDs(donations []Donation) []string {
	var subscriptionIDs []string

	for _, donation := range donations {
		if donation.ProviderSubscriptionID != "" {
			subscriptionIDs = append(subscriptionIDs, donation.ProviderSubscriptionID)
		}
	}

	return subscriptionIDs
}

func uncancelledSubscriptions(cancelled []string, all []string) []string {
	var uncancelled []string

	for _, sub := range all {
		var found bool
		for _, c := range cancelled {
			if sub == c {
				found = true
				break
			}
		}

		if !found {
			uncancelled = append(uncancelled, sub)
		}
	}

	return uncancelled
}
