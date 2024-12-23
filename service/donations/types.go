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
	Principal       uuid.NullUUID   `json:"principal"`
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
	Stats           FundStats       `json:"stats"`
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
	Principal       uuid.NullUUID
}

type UpdateFund struct {
	ID              uuid.UUID
	Name            string
	Description     string
	Active          bool
	GoalCents       int32
	PayoutFrequency string
	Expires         *time.Time
	Principal       uuid.NullUUID
}

type Donation struct {
	ID                     uuid.UUID
	DonorID                uuid.UUID
	DonationPlanID         uuid.NullUUID
	FundID                 uuid.UUID
	FundName               string
	Recurring              bool
	ProviderID             string
	ProviderOrderID        string
	ProviderSubscriptionID string
	Payment                *DonationPayment
	Payments               []DonationPayment
	Plan                   *DonationPlan
	Created                time.Time
	Updated                time.Time
}

func (d Donation) TotalDonatedCents() int32 {
	var total int32
	for _, payment := range d.Payments {
		total += payment.AmountCents
	}

	return total
}

func (d Donation) LastPayment() *DonationPayment {
	if len(d.Payments) == 0 {
		return nil
	}

	return &d.Payments[len(d.Payments)-1]
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
	PlanID                 uuid.NullUUID `json:"plan_id"`
	AmountCents            int32         `json:"amount_cents"`
	FundID                 uuid.UUID     `json:"fund_id"`
	ProviderOrderID        string        `json:"provider_order_id"`
	ProviderSubscriptionID string        `json:"provider_subscription_id"`
}

type OneTimeCompletion struct {
	AmountCents       int32
	FundID            uuid.UUID
	IPAddress         string
	BCOName           string
	PayerID           string
	PayerEmail        string
	PayerFirstName    string
	PayerLastName     string
	ProviderOrderID   string
	ProviderPaymentID string
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
	ID                     uuid.UUID
	DonorID                uuid.UUID
	FundID                 uuid.UUID
	Recurring              bool
	PlanID                 uuid.NullUUID
	ProviderOrderID        string
	ProviderSubscriptionID string
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

type CreateOrderResponse struct {
	OrderID     string `json:"order_id"`
	ApprovalURL string `json:"approval_url"`
}

type FundStats struct {
	TotalDonated    int32
	TotalDonations  int32
	AverageDonation int32
	TotalDonors     int32
	Monthly         []MonthTotal
}

type MonthTotal struct {
	MonthYear    string `json:"month"`
	TotalCents   int32  `json:"amount"`
	UniqueDonors int32  `json:"unique_donors"`
}
