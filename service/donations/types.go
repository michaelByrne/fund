package donations

import (
	"github.com/google/uuid"
	"time"
)

type IntervalUnit string
type PayoutFrequency string

const (
	IntervalUnitWeek       IntervalUnit    = "WEEK"
	IntervalUnitMonth      IntervalUnit    = "MONTH"
	PayoutFrequencyMonthly PayoutFrequency = "monthly"
	PayoutFrequencyOnce    PayoutFrequency = "once"
)

type Fund struct {
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ProviderID      string          `json:"provider_id"`
	ProviderName    string          `json:"provider_name"`
	Active          bool            `json:"active"`
	GoalCents       int32           `json:"goal_cents"`
	PayoutFrequency PayoutFrequency `json:"payout_frequency"`
	Expires         *time.Time      `json:"expires"`
	NextPayment     time.Time       `json:"next_payment"`
	Created         time.Time       `json:"created"`
	Updated         time.Time       `json:"updated"`
}

type InsertFund struct {
	ID              uuid.UUID
	Name            string
	Description     string
	ProviderID      string
	Active          bool
	ProviderName    string
	GoalCents       int32
	PayoutFrequency string
	Expires         *time.Time
}

type UpdateFund struct {
	ID              uuid.UUID
	Name            string
	Description     string
	Active          bool
	GoalCents       int32
	PayoutFrequency string
	Expires         *time.Time
}

type Donation struct {
	ID             uuid.UUID
	DonorID        uuid.UUID
	DonationPlanID uuid.NullUUID
	FundID         uuid.UUID
	Recurring      bool
	ProviderID     string
	Payment        *DonationPayment
	Created        time.Time
	Updated        time.Time
}

type DonationPayment struct {
	ID                  uuid.UUID
	DonationID          uuid.UUID
	ProviderPaymentID   string
	AmountCents         int32
	MemberProviderEmail string
	Created             time.Time
	Updated             time.Time
}

type DonationOrderCapture struct {
	ProviderOrderID     string
	PlanID              uuid.UUID
	MemberProviderEmail string
	ProviderPaymentID   string
	AmountCents         int32
}

type RecurringCompletion struct {
	ProviderOrderID    string        `json:"provider_order_id"`
	PlanID             uuid.NullUUID `json:"plan_id"`
	ProviderPlanID     string        `json:"provider_plan_id"`
	ProviderDonationID string        `json:"provider_donation_id"`
	IPAddress          string
	PayerEmail         string    `json:"payer_email"`
	AmountCents        int32     `json:"amount_cents"`
	PayerFirstName     string    `json:"first_name"`
	PayerLastName      string    `json:"last_name"`
	PayerID            string    `json:"payer_id"`
	BCOName            string    `json:"bco_name"`
	FundID             uuid.UUID `json:"fund_id"`
}

type OneTimeCompletion struct {
	AmountCents    int32
	FundID         uuid.UUID
	IPAddress      string
	BCOName        string
	PayerID        string
	PayerEmail     string
	PayerFirstName string
	PayerLastName  string
}

type DonationCompletionResponse struct {
	ProviderOrderID   string
	ProviderPaymentID string
	PayerID           string
	PayerEmail        string
	PayerFirstName    string
	PayerLastName     string
}

type DonationPlan struct {
	ID             uuid.UUID
	Name           string
	ProviderPlanID string
	AmountCents    int32
	IntervalUnit   IntervalUnit
	IntervalCount  int32
	Active         bool
	FundID         uuid.UUID
	Created        time.Time
	Updated        time.Time
}

type UpsertDonationPlan struct {
	ID             uuid.UUID
	Name           string
	ProviderPlanID string
	FundID         uuid.UUID
	AmountCents    int32
	IntervalUnit   IntervalUnit
	IntervalCount  int32
	Active         bool
}

type CreatePlan struct {
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	ProviderFundID string       `json:"product_id"`
	IntervalUnit   IntervalUnit `json:"interval_unit"`
	IntervalCount  int32        `json:"interval_count"`
	AmountCents    int32        `json:"amount_cents"`
	FundID         uuid.UUID    `json:"fund_id"`
}

type InsertDonation struct {
	ID        uuid.UUID
	DonorID   uuid.UUID
	FundID    uuid.UUID
	Recurring bool
	PlanID    uuid.NullUUID
}

type UpdateDonation struct {
	ID             uuid.UUID
	DonorID        uuid.UUID
	DonationPlanID uuid.NullUUID
}

type InsertDonationPayment struct {
	ID                uuid.UUID
	DonationID        uuid.UUID
	ProviderPaymentID string
	AmountCents       int32
}
