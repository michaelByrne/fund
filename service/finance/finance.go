package finance

import (
	"boardfund/service/donations"
	"context"
	"github.com/google/uuid"
	"log/slog"
)

type AuditDonation struct {
	Active                 bool
	ProviderSubscriptionID string
	FirstName              string
	LastName               string
}

type donationStore interface {
	GetRecurringDonationsForFund(ctx context.Context, arg donations.GetRecurringDonationsForFundRequest) ([]donations.Donation, error)
	GetPaymentsForDonation(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
}

type paymentsProvider interface {
	ProviderDonationSubscriptionIsActive(ctx context.Context, providerSubscriptionID string) (bool, error)
}

type FinanceService struct {
	donationStore donationStore

	logger *slog.Logger
}

func NewFinanceService(donationStore donationStore, logger *slog.Logger) *FinanceService {
	return &FinanceService{
		donationStore: donationStore,
		logger:        logger,
	}
}
