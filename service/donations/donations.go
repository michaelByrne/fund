package donations

import (
	"boardfund/service/members"
	"context"
	"log/slog"
)

type donationStore interface {
	CreateDonationPlan(ctx context.Context, plan InsertDonationPlan) (*DonationPlan, error)
	CreateDonation(ctx context.Context, donation InsertDonation) (*Donation, error)
	CreateDonationPayment(ctx context.Context, payment InsertDonationPayment) (*DonationPayment, error)
	CreateDonationWithPayment(ctx context.Context, donation InsertDonation, payment InsertDonationPayment) (*Donation, error)
}

type memberStore interface {
	UpsertMember(ctx context.Context, member members.UpsertMember) (*members.Member, error)
}

type paymentsProvider interface {
	CreatePlan(ctx context.Context, plan CreatePlan) (string, error)
	CreateProduct(ctx context.Context, name, description string) (string, error)
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

func (s DonationService) CaptureDonationOrder(ctx context.Context, createCapture CreateCapture) error {
	upsertMember := members.UpsertMember{
		MemberProviderEmail: createCapture.PayerEmail,
		IPAddress:           createCapture.IPAddress,
		BCOName:             createCapture.BCOName,
		ProviderPayerID:     createCapture.PayerID,
		FirstName:           createCapture.PayerFirstName,
		LastName:            createCapture.PayerLastName,
	}

	member, err := s.memberStore.UpsertMember(ctx, upsertMember)
	if err != nil {
		s.logger.Error("failed to upsert member", slog.String("error", err.Error()))

		return err
	}

	insertPayment := InsertDonationPayment{
		AmountCents:       createCapture.AmountCents,
		ProviderPaymentID: "initial",
	}

	insertDonation := InsertDonation{
		DonorID:        member.ID,
		DonationPlanID: createCapture.PlanID,
	}

	_, err = s.donationStore.CreateDonationWithPayment(ctx, insertDonation, insertPayment)
	if err != nil {
		s.logger.Error("failed to create donation with payment", slog.String("error", err.Error()))

		return err
	}

	return nil
}

func (s DonationService) CreateProduct(ctx context.Context, name, description string) (string, error) {
	return s.paymentsProvider.CreateProduct(ctx, name, description)
}

func (s DonationService) CreateDonationPlan(ctx context.Context, plan CreatePlan) (*DonationPlan, error) {
	planID, err := s.paymentsProvider.CreatePlan(ctx, plan)
	if err != nil {
		s.logger.Error("failed to create plan with payments provider", slog.String("error", err.Error()))

		return nil, err
	}

	insertPlan := InsertDonationPlan{
		Name:           plan.Name,
		AmountCents:    plan.AmountCents,
		IntervalUnit:   string(plan.IntervalUnit),
		IntervalCount:  plan.IntervalCount,
		ProviderPlanID: planID,
		Active:         true,
	}

	newPlan, err := s.donationStore.CreateDonationPlan(ctx, insertPlan)
	if err != nil {
		s.logger.Error("failed to create donation plan", slog.String("error", err.Error()))

		return nil, err
	}

	return newPlan, nil
}
