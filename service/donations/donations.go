package donations

import (
	"boardfund/service/members"
	"context"
	"github.com/google/uuid"
	"log/slog"
)

type donationStore interface {
	InsertFund(ctx context.Context, fund InsertFund) (*Fund, error)
	UpdateFund(ctx context.Context, fund UpdateFund) (*Fund, error)
	UpsertDonationPlan(ctx context.Context, plan UpsertDonationPlan) (*DonationPlan, error)
	InsertDonation(ctx context.Context, donation InsertDonation) (*Donation, error)
	InsertDonationPayment(ctx context.Context, payment InsertDonationPayment) (*DonationPayment, error)
	InsertDonationWithPayment(ctx context.Context, donation InsertDonation, payment InsertDonationPayment) (*Donation, error)
	GetFunds(ctx context.Context) ([]Fund, error)
	GetFundByID(ctx context.Context, uuid uuid.UUID) (*Fund, error)
}

type memberStore interface {
	UpsertMember(ctx context.Context, member members.UpsertMember) (*members.Member, error)
}

type paymentsProvider interface {
	CreatePlan(ctx context.Context, plan CreatePlan) (string, error)
	CreateFund(ctx context.Context, name, description string) (string, error)
	InitiateDonation(ctx context.Context, fund Fund, amountCents int32) (string, error)
	FinalizeDonation(ctx context.Context, internalDonationID uuid.UUID, orderID string) error
}

type DonationService struct {
	donationStore    donationStore
	memberStore      memberStore
	paymentsProvider paymentsProvider

	logger *slog.Logger
}

func NewDonationService(donationStore donationStore, memberStore memberStore, provider paymentsProvider, logger *slog.Logger) *DonationService {
	return &DonationService{
		donationStore:    donationStore,
		memberStore:      memberStore,
		paymentsProvider: provider,
		logger:           logger,
	}
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

func (s DonationService) CompleteRecurringDonation(ctx context.Context, completion RecurringCompletion) error {
	upsertMember := members.UpsertMember{
		ID:                  uuid.New(),
		MemberProviderEmail: completion.PayerEmail,
		IPAddress:           completion.IPAddress,
		BCOName:             completion.BCOName,
		ProviderPayerID:     completion.PayerID,
		FirstName:           completion.PayerFirstName,
		LastName:            completion.PayerLastName,
	}

	member, err := s.memberStore.UpsertMember(ctx, upsertMember)
	if err != nil {
		s.logger.Error("failed to upsert member", slog.String("error", err.Error()))

		return err
	}

	insertDonation := InsertDonation{
		ID:        uuid.New(),
		DonorID:   member.ID,
		PlanID:    completion.PlanID,
		FundID:    completion.FundID,
		Recurring: true,
	}

	insertPayment := InsertDonationPayment{
		ID:                uuid.New(),
		DonationID:        insertDonation.ID,
		AmountCents:       completion.AmountCents,
		ProviderPaymentID: "initial",
	}

	_, err = s.donationStore.InsertDonationWithPayment(ctx, insertDonation, insertPayment)
	if err != nil {
		s.logger.Error("failed to create donation with payment", slog.String("error", err.Error()))

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

	providerOrderID, err := s.paymentsProvider.InitiateDonation(ctx, *fund, amountCents)
	if err != nil {
		s.logger.Error("failed to initiate donation with provider", slog.String("error", err.Error()))

		return "", err
	}

	return providerOrderID, nil
}

func (s DonationService) CompleteDonation(ctx context.Context, completion OneTimeCompletion) error {
	upsertMember := members.UpsertMember{
		MemberProviderEmail: completion.PayerEmail,
		IPAddress:           completion.IPAddress,
		BCOName:             completion.BCOName,
		ProviderPayerID:     completion.PayerID,
		FirstName:           completion.PayerFirstName,
		LastName:            completion.PayerLastName,
	}

	member, err := s.memberStore.UpsertMember(ctx, upsertMember)
	if err != nil {
		s.logger.Error("failed to upsert member", slog.String("error", err.Error()))

		return err
	}

	insertDonation := InsertDonation{
		ID:      uuid.New(),
		DonorID: member.ID,
		FundID:  completion.FundID,
	}

	insertPayment := InsertDonationPayment{
		ID:                uuid.New(),
		AmountCents:       completion.AmountCents,
		ProviderPaymentID: "initial",
		DonationID:        insertDonation.ID,
	}

	_, err = s.donationStore.InsertDonationWithPayment(ctx, insertDonation, insertPayment)
	if err != nil {
		s.logger.Error("failed to create donation with payment", slog.String("error", err.Error()))

		return err
	}

	err = s.paymentsProvider.FinalizeDonation(ctx, insertDonation.ID, "orderID")
	if err != nil {
		s.logger.Error("failed to finalize donation with provider", slog.String("error", err.Error()))

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
		Active:          true,
		ProviderName:    "paypal",
	}

	fund, err := s.donationStore.InsertFund(ctx, insertFund)
	if err != nil {
		s.logger.Error("failed to insert fund", slog.String("error", err.Error()))

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
