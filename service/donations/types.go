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
	Active                 bool
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
	ProviderFeeCents    int32
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
	ProviderFeeCents  int32
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
	ProviderFeeCents  int32
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

type DeactivateDonation struct {
	ID     uuid.UUID
	Reason string
}

type DeactivateDonationBySubscription struct {
	SubscriptionID string
	Reason         string
}

type GetRecurringDonationsForFundRequest struct {
	FundID uuid.UUID
	Active bool
}

// Webhook events

type PaymentSaleEvent struct {
	BillingAgreementID        string         `json:"billing_agreement_id"`
	Amount                    Amount         `json:"amount"`
	PaymentMode               string         `json:"payment_mode"`
	UpdateTime                time.Time      `json:"update_time"`
	CreateTime                time.Time      `json:"create_time"`
	ProtectionEligibilityType string         `json:"protection_eligibility_type"`
	TransactionFee            TransactionFee `json:"transaction_fee"`
	ProtectionEligibility     string         `json:"protection_eligibility"`
	Links                     []Links        `json:"links"`
	ID                        string         `json:"id"`
	State                     string         `json:"state"`
	InvoiceNumber             string         `json:"invoice_number"`
}
type Details struct {
	Subtotal string `json:"subtotal"`
}
type Amount struct {
	Total    string  `json:"total"`
	Currency string  `json:"currency"`
	Details  Details `json:"details"`
}
type TransactionFee struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}
type Links struct {
	Method string `json:"method"`
	Rel    string `json:"rel"`
	Href   string `json:"href"`
}

type SubscriptionEvent struct {
	Quantity         string         `json:"quantity"`
	Subscriber       Subscriber     `json:"subscriber"`
	CreateTime       time.Time      `json:"create_time"`
	ShippingAmount   ShippingAmount `json:"shipping_amount"`
	StartTime        time.Time      `json:"start_time"`
	UpdateTime       time.Time      `json:"update_time"`
	BillingInfo      BillingInfo    `json:"billing_info"`
	Links            []Links        `json:"links"`
	ID               string         `json:"id"`
	PlanID           string         `json:"plan_id"`
	AutoRenewal      bool           `json:"auto_renewal"`
	Status           string         `json:"status"`
	StatusUpdateTime time.Time      `json:"status_update_time"`
}
type Name struct {
	GivenName string `json:"given_name"`
	Surname   string `json:"surname"`
}
type FullName struct {
	FullName string `json:"full_name"`
}
type Address struct {
	AddressLine1 string `json:"address_line_1"`
	AddressLine2 string `json:"address_line_2"`
	AdminArea2   string `json:"admin_area_2"`
	AdminArea1   string `json:"admin_area_1"`
	PostalCode   string `json:"postal_code"`
	CountryCode  string `json:"country_code"`
}
type ShippingAddress struct {
	Name    FullName `json:"name"`
	Address Address  `json:"address"`
}
type Subscriber struct {
	Name            Name            `json:"name"`
	EmailAddress    string          `json:"email_address"`
	ShippingAddress ShippingAddress `json:"shipping_address"`
}
type ShippingAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type OutstandingBalance struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}
type CycleExecutions struct {
	TenureType                  string `json:"tenure_type"`
	Sequence                    int    `json:"sequence"`
	CyclesCompleted             int    `json:"cycles_completed"`
	CyclesRemaining             int    `json:"cycles_remaining"`
	CurrentPricingSchemeVersion int    `json:"current_pricing_scheme_version"`
}

type LastPayment struct {
	Amount Amount    `json:"amount"`
	Time   time.Time `json:"time"`
}
type BillingInfo struct {
	OutstandingBalance  OutstandingBalance `json:"outstanding_balance"`
	CycleExecutions     []CycleExecutions  `json:"cycle_executions"`
	LastPayment         LastPayment        `json:"last_payment"`
	NextBillingTime     time.Time          `json:"next_billing_time"`
	FinalPaymentTime    time.Time          `json:"final_payment_time"`
	FailedPaymentsCount int                `json:"failed_payments_count"`
}

type GetOneTimeDonationsForFundRequest struct {
	FundID uuid.UUID
	Active bool
}

type UpdatePaymentPaypalFee struct {
	ID               uuid.UUID
	ProviderFeeCents int32
}
