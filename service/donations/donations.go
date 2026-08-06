package donations

import (
	"boardfund/service/fundevents"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
)

// eventRecorder writes the fund activity feed. Record does not return an error
// on purpose: it runs after the operation it describes has committed, so there
// is nothing a caller could usefully do about a failure.
type eventRecorder interface {
	Record(ctx context.Context, record fundevents.Record)
}

type DonationService struct {
	donationStore    donationStore
	documentStorage  documentStorage
	paymentsProvider PaymentsProvider
	events           eventRecorder

	reportBuckets []string

	logger *slog.Logger
}

func NewDonationService(donationStore donationStore, documentStorage documentStorage, provider PaymentsProvider, events eventRecorder, reportBuckets []string, logger *slog.Logger) *DonationService {
	return &DonationService{
		donationStore:    donationStore,
		documentStorage:  documentStorage,
		paymentsProvider: provider,
		events:           events,
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

	// The monthly breakdown is deliberately not loaded here. This used to run a
	// query per fund and assign the result to `fund`, which is a copy of the slice
	// element, so every row was fetched and discarded. Nothing consumes Monthly
	// from a list -- the charts are on the fund detail page, which loads its own.
	return funds, nil
}

// ListAllFunds returns every fund, including closed and expired ones.
//
// Separate from ListActiveFunds rather than a flag on it, because the two have
// opposite defaults for a reason: the public page must not offer a donor a fund
// that is closed, and the admin page must not lose one. A shared function with a
// boolean would make it easy to get that backwards at a call site.
func (s DonationService) ListAllFunds(ctx context.Context) ([]Fund, error) {
	funds, err := s.donationStore.GetAllFundsWithStats(ctx)
	if err != nil {
		s.logger.Error("failed to list all funds", slog.String("error", err.Error()))

		return nil, err
	}

	return funds, nil
}

// ErrSubscriptionsNotCancelled means the provider would not cancel every
// subscription, so the fund was left open.
var ErrSubscriptionsNotCancelled = errors.New("could not cancel all subscriptions at the provider")

// DeactivateFund closes a fund: no more donations, and every recurring
// subscription cancelled at the provider.
//
// The provider is called before anything is written, and a failure leaves the
// fund open. The previous order committed first, so a PayPal outage produced a
// fund that was closed locally while donors carried on being charged, with
// nothing to retry it -- the reconciler only pushes the other way, marking
// donations inactive once the provider says they are gone.
//
// Reversing it means the surviving failure mode is subscriptions cancelled and
// the local commit lost, which that same reconciler already repairs.
//
// Partial cancellation is refused rather than half-applied. Closing a fund while
// some donors keep paying into it is worse than not closing it, and "nothing
// happened, try again" is a state an admin can act on.
func (s DonationService) DeactivateFund(ctx context.Context, id, actorID uuid.UUID) error {
	recurring, err := s.donationStore.GetRecurringDonationsForFund(ctx, GetRecurringDonationsForFundRequest{
		FundID: id,
		Active: true,
	})
	if err != nil {
		s.logger.Error("failed to read recurring donations before deactivating fund",
			slog.String("error", err.Error()))

		return err
	}

	toCancel := extractProviderSubscriptionIDs(recurring)

	if len(toCancel) > 0 {
		cancelled, errCancel := s.paymentsProvider.CancelSubscriptions(ctx, toCancel)
		if errCancel != nil {
			s.logger.Error("failed to cancel subscriptions, fund left active",
				slog.String("error", errCancel.Error()),
				slog.String("fund_id", id.String()),
			)

			return errCancel
		}

		// Compared by membership, not by count. Equal lengths would also be true
		// of a provider that returned duplicates or a different set of ids, and
		// the question here is whether anything we asked for is still running.
		uncancelled := uncancelledSubscriptions(cancelled, toCancel)
		if len(uncancelled) > 0 {
			s.logger.Error("could not cancel every subscription, fund left active",
				slog.String("fund_id", id.String()),
				slog.String("uncancelled", fmt.Sprintf("%v", uncancelled)),
			)

			return fmt.Errorf("%w: %d of %d remain", ErrSubscriptionsNotCancelled,
				len(uncancelled), len(toCancel))
		}
	}

	deactivated, err := s.donationStore.SetFundAndDonationsToInactive(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate fund", slog.String("error", err.Error()))

		return err
	}

	// Recorded only once everything has actually happened. They used to be
	// written before the provider was called, so a cancellation that failed and
	// was rolled back left the history asserting a donation had been cancelled
	// while it was still running.
	for _, cancelled := range deactivated {
		s.events.Record(ctx, fundevents.Record{
			FundID:          cancelled.FundID,
			Kind:            fundevents.KindDonationCancelled,
			ActorMemberID:   &actorID,
			SubjectMemberID: &cancelled.DonorID,
			Detail:          "fund deactivated",
			ReferenceID:     &cancelled.ID,
		})
	}

	return nil
}

func (s DonationService) DeactivateDonation(ctx context.Context, id, actorID uuid.UUID, reason string) (*Donation, error) {
	donation, err := s.donationStore.SetDonationToInactive(ctx, DeactivateDonation{
		ID:     id,
		Reason: reason,
	})
	if err != nil {
		s.logger.Error("failed to set donation to inactive", slog.String("error", err.Error()))

		return nil, err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:          donation.FundID,
		Kind:            fundevents.KindDonationCancelled,
		ActorMemberID:   &actorID,
		SubjectMemberID: &donation.DonorID,
		Detail:          reason,
		ReferenceID:     &donation.ID,
	})

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

	s.events.Record(ctx, fundevents.Record{
		FundID:          completion.FundID,
		Kind:            fundevents.KindDonationStarted,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		AmountCents:     &completion.AmountCents,
		Detail:          "recurring",
		ReferenceID:     &insertDonation.ID,
	})

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

	s.events.Record(ctx, fundevents.Record{
		FundID:          completion.FundID,
		Kind:            fundevents.KindDonationStarted,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		AmountCents:     &completion.AmountCents,
		Detail:          "one-time",
		ReferenceID:     &insertDonation.ID,
	})

	s.events.Record(ctx, fundevents.Record{
		FundID:          completion.FundID,
		Kind:            fundevents.KindPaymentReceived,
		ActorMemberID:   &memberID,
		SubjectMemberID: &memberID,
		AmountCents:     &completion.AmountCents,
		ReferenceID:     &insertDonation.ID,
	})

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
