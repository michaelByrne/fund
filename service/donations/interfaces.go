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
	SetDonationToInactive(ctx context.Context, id uuid.UUID) (*Donation, error)
	SetFundAndDonationsToInactive(ctx context.Context, id uuid.UUID) ([]Donation, error)
	SetFundAndDonationsToActive(ctx context.Context, id uuid.UUID) ([]Donation, error)
	SetDonationToActiveBySubscriptionID(ctx context.Context, id string) (*Donation, error)
	GetActiveFunds(ctx context.Context) ([]Fund, error)
	GetMonthlyDonationTotalsForFund(ctx context.Context, id uuid.UUID) ([]MonthTotal, error)
	GetDonationByProviderSubscriptionID(ctx context.Context, id string) (*Donation, error)
}

type paymentsProvider interface {
	CreatePlan(ctx context.Context, plan CreatePlan) (string, error)
	CreateFund(ctx context.Context, name, description string) (string, error)
	InitiateDonation(ctx context.Context, fund Fund, amountCents int32) (string, error)
	CancelSubscriptions(ctx context.Context, ids []string) ([]string, error)
}

type subscriber interface {
	Subscribe(event string, cb func(data []byte)) error
}
