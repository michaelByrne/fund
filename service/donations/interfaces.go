package donations

import (
	"context"
	"github.com/google/uuid"
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
	GetTotalDonatedByFundID(ctx context.Context, id uuid.UUID) (int64, error)
	SetDonationToInactive(ctx context.Context, arg DeactivateDonation) (*Donation, error)
	SetFundAndDonationsToInactive(ctx context.Context, id uuid.UUID) ([]Donation, error)
	GetActiveFunds(ctx context.Context, freq string) ([]Fund, error)
	GetAllFundsWithStats(ctx context.Context) ([]Fund, error)
	GetClosedFundsWithStats(ctx context.Context) ([]ClosedFund, error)
	GetExpiredActiveFunds(ctx context.Context) ([]Fund, error)
	GetFundPayoutStats(ctx context.Context, fundID uuid.UUID) (PayoutStats, error)
	GetRecurringDonationsForFund(ctx context.Context, arg GetRecurringDonationsForFundRequest) ([]Donation, error)
	GetMonthlyDonationTotalsForFund(ctx context.Context, id uuid.UUID) ([]MonthTotal, error)
	GetDonationByProviderSubscriptionID(ctx context.Context, id string) (*Donation, error)
	GetDonationPlanByID(ctx context.Context, id uuid.UUID) (*DonationPlan, error)
	GetDonationsForDonor(ctx context.Context, donorID uuid.UUID) ([]MemberDonation, error)
	GetDonationByID(ctx context.Context, id uuid.UUID) (*Donation, error)
	SetDonationToInactiveBySubscriptionID(ctx context.Context, arg DeactivateDonationBySubscription) (*Donation, error)
	ReactivateSuspendedDonation(ctx context.Context, subscriptionID string) (*Donation, error)
	SetDonationPaymentRefunded(ctx context.Context, providerPaymentID string, refundedCents int32) (*RefundedPayment, error)
}

//go:generate moq -pkg mocks -out ../mocks/payments_moq.go . PaymentsProvider
type PaymentsProvider interface {
	CreatePlan(ctx context.Context, plan CreatePlan) (string, error)
	CreateFund(ctx context.Context, name, description string) (string, error)
	InitiateDonation(ctx context.Context, fund Fund, amountCents int32) (string, error)
	GetOrder(ctx context.Context, orderID string) (*ProviderOrder, error)
	GetSubscription(ctx context.Context, subscriptionID string) (*ProviderSubscription, error)
	CancelSubscriptions(ctx context.Context, ids []string) ([]string, error)
}

type subscriber interface {
	Subscribe(event string, cb func(data []byte) error) error
}

type documentStorage interface {
	CreateFundBucket(ctx context.Context, prefix string, fundID uuid.UUID) error
}
