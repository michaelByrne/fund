package finance

import (
	"boardfund/service/donations"
	"context"
	"github.com/google/uuid"
	"log/slog"
	"strings"
	"time"
)

type AuditDonation struct {
	Active                 bool
	ProviderSubscriptionID string
	FirstName              string
	LastName               string
}

type ProviderTransaction struct {
	ProviderPaymentID string
	Status            string
	AmountCents       int32
}

type donationStore interface {
	GetRecurringDonationsForFund(ctx context.Context, arg donations.GetRecurringDonationsForFundRequest) ([]donations.Donation, error)
	GetPaymentsForDonation(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
	GetActiveFunds(ctx context.Context) ([]donations.Fund, error)
	SetDonationToInactive(ctx context.Context, arg donations.DeactivateDonation) (*donations.Donation, error)
	GetDonationPaymentsByDonationID(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error)
}

type paymentsProvider interface {
	GetProviderDonationSubscriptionStatus(ctx context.Context, providerSubscriptionID string) (string, error)
	GetTransactionsForDonationSubscription(ctx context.Context, subscriptionID string) ([]ProviderTransaction, error)
	GetTransaction(ctx context.Context, id string, start, end time.Time) (*ProviderTransaction, error)
}

type FinanceService struct {
	donationStore    donationStore
	paymentsProvider paymentsProvider

	logger *slog.Logger
}

func NewFinanceService(donationStore donationStore, paymentsProvider paymentsProvider, logger *slog.Logger) *FinanceService {
	return &FinanceService{
		donationStore:    donationStore,
		paymentsProvider: paymentsProvider,
		logger:           logger,
	}
}

func (s FinanceService) RunDonationReconciliation(ctx context.Context) error {
	funds, err := s.donationStore.GetActiveFunds(ctx)
	if err != nil {
		s.logger.Error("failed to get active funds", slog.String("error", err.Error()))

		return err
	}

	for _, fund := range funds {
		errInner := s.reconcileDonationsForFund(ctx, fund.ID)
		if errInner != nil {
			return errInner
		}
	}

	return nil
}

func (s FinanceService) reconcileDonationsForFund(ctx context.Context, fundID uuid.UUID) error {
	req := donations.GetRecurringDonationsForFundRequest{
		FundID: fundID,
		Active: true,
	}

	recurringDonations, err := s.donationStore.GetRecurringDonationsForFund(ctx, req)
	if err != nil {
		s.logger.Error("failed to get recurring donations for fund", slog.String("error", err.Error()))

		return err
	}

	for _, donation := range recurringDonations {
		status, errInner := s.paymentsProvider.GetProviderDonationSubscriptionStatus(ctx, donation.ProviderSubscriptionID)
		if errInner != nil {
			s.logger.Error("failed to get donation status from provider", slog.String("error", errInner.Error()))
		}

		if !(strings.ToUpper(status) == "ACTIVE") {
			s.logger.Info("donation is inactive at provider", slog.String("donation_id", donation.ID.String()))

			_, errInner = s.donationStore.SetDonationToInactive(ctx, donations.DeactivateDonation{
				ID:     donation.ID,
				Reason: status,
			})
			if errInner != nil {
				s.logger.Error("failed to set donation to inactive", slog.String("error", errInner.Error()))

				return errInner
			}
		}

		payments, errInner := s.donationStore.GetDonationPaymentsByDonationID(ctx, donation.ID)
		if errInner != nil {
			s.logger.Error("failed to get donation payments", slog.String("error", errInner.Error()))

			return errInner
		}

		for _, payment := range payments {
			transaction, errTrans := s.paymentsProvider.GetTransaction(ctx, payment.ProviderPaymentID, payment.Created.AddDate(0, 0, -1), time.Now())
			if errTrans != nil {
				s.logger.Error("failed to get transaction from provider", slog.String("error", errTrans.Error()))

				continue
			}

			if !(strings.ToUpper(transaction.Status) == "COMPLETED") {
				s.logger.Info("payment is incomplete at provider", slog.String("payment_id", payment.ID.String()))

				continue
			}

			if transaction.AmountCents != payment.AmountCents {
				s.logger.Info("payment amount does not match provider", slog.String("payment_id", payment.ID.String()), slog.Int("expected", int(payment.AmountCents)), slog.Int("actual", int(transaction.AmountCents)))
			}
		}
	}

	return nil
}
