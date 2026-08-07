package donations

import (
	"github.com/google/uuid"
	"time"
)

type IntervalUnit string
type PayoutFrequency string

const (
	IntervalUnitWeek  IntervalUnit = "WEEK"
	IntervalUnitMonth IntervalUnit = "MONTH"

	PayoutFrequencyMonthly PayoutFrequency = "monthly"
	PayoutFrequencyOnce    PayoutFrequency = "once"

	// PayoutFrequencyDaily exists to make the payout lifecycle testable. A monthly
	// fund takes a month per period, and a 'once' fund never advances to a second
	// one, so neither shows that the schedule actually rolls forward.
	PayoutFrequencyDaily PayoutFrequency = "daily"
)

// PayoutFrequencies is every frequency a fund can have.
//
// Somewhere to add one, so that code which must cover them all can iterate rather
// than repeat a literal. Reconciliation named "monthly" directly and silently
// stopped covering everything the day daily funds existed.
var PayoutFrequencies = []PayoutFrequency{
	PayoutFrequencyMonthly,
	PayoutFrequencyDaily,
	PayoutFrequencyOnce,
}

// Recurring reports whether the fund pays out more than once.
//
// The distinction used to be spelled `== PayoutFrequencyMonthly` at each call
// site, which read as "is this monthly" but meant "does this repeat". Every one
// of those would have treated a daily fund as a one-off: no rolling forward of
// the next payout date, and donors offered a single payment instead of a
// subscription.
func (f PayoutFrequency) Recurring() bool {
	return f == PayoutFrequencyMonthly || f == PayoutFrequencyDaily
}

// ProviderOrder is an order as the provider reports it, which is the only
// account of a one-time donation worth recording.
//
// The browser used to supply the amount and the payment id directly, and they
// were written to the database unchecked. A member could name any figure, and
// the fund balance is what the planner divides between enrollees -- so an
// invented donation disbursed real money, out of a PayPal balance shared with
// every other fund.
type ProviderOrder struct {
	Status string
	// FundReferenceID is the reference we set when creating the order, so it
	// cannot be chosen by whoever completes it.
	FundReferenceID   string
	ProviderPaymentID string
	AmountCents       int32
}

// MemberDonation is one row of a donor's own donations page.
type MemberDonation struct {
	ID     uuid.UUID
	FundID uuid.UUID

	FundName   string
	FundActive bool

	Recurring bool
	Active    bool
	// InactiveReason is the provider status that ended it, or the reason it was
	// cancelled here. Shown so a donor who did not cancel can see who did.
	InactiveReason string

	TotalGivenCents  int64
	PlanAmountCents  int32
	PlanIntervalUnit IntervalUnit

	Started     time.Time
	LastPayment *time.Time

	hasSubscription bool
}

// Cancellable reports whether the donor can end this from the page.
//
// A one-off donation has nothing to cancel -- the money has been given. An
// inactive one has already stopped. And a recurring donation with no subscription
// id recorded cannot be cancelled at the provider, so offering the control would
// promise something that cannot be delivered.
func (m MemberDonation) Cancellable() bool {
	return m.Active && m.Recurring && m.hasSubscription
}

// MemberDonationRow is what a store hands NewMemberDonation. It exists so
// hasSubscription stays unexported: whether a donation can be cancelled is a rule
// of this package, not something a caller assembles for itself.
type MemberDonationRow struct {
	ID              uuid.UUID
	FundID          uuid.UUID
	FundName        string
	FundActive      bool
	Recurring       bool
	Active          bool
	InactiveReason  string
	HasSubscription bool
	TotalGivenCents int64
	PlanAmountCents int32
	PlanInterval    string
	Started         time.Time
	LastPayment     *time.Time
}

func NewMemberDonation(row MemberDonationRow) MemberDonation {
	return MemberDonation{
		ID:               row.ID,
		FundID:           row.FundID,
		FundName:         row.FundName,
		FundActive:       row.FundActive,
		Recurring:        row.Recurring,
		Active:           row.Active,
		InactiveReason:   row.InactiveReason,
		TotalGivenCents:  row.TotalGivenCents,
		PlanAmountCents:  row.PlanAmountCents,
		PlanIntervalUnit: IntervalUnit(row.PlanInterval),
		Started:          row.Started,
		LastPayment:      row.LastPayment,
		hasSubscription:  row.HasSubscription,
	}
}

// ProviderSubscription is a subscription as the provider reports it.
//
// The browser used to supply the subscription id, the plan, the fund and the
// amount, and all four were written unchecked. The fund is the one that cost
// money: a subscription created against one fund's plan could be recorded
// against another, and every payment on it then joined the wrong fund's balance
// and was paid out to the wrong fund's enrollees.
type ProviderSubscription struct {
	Status string
	// ProviderPlanID is the plan the subscription actually pays into. Ours,
	// created per fund, so it is what ties the subscription to a fund.
	ProviderPlanID string
	AmountCents    int32
}

// Active reports whether the provider considers this subscription live.
//
// APPROVED counts: the donor has authorised it and PayPal has not yet taken the
// first payment, which is precisely the state the completion flow runs in.
func (p ProviderSubscription) Active() bool {
	return p.Status == "ACTIVE" || p.Status == "APPROVED"
}

// RefundEvent is the resource on PAYMENT.SALE.REFUNDED and .REVERSED.
//
// sale_id points at the payment being refunded, which is the id we store. A
// reversal reports the sale in id instead, so both are read and sale_id wins.
//
// total_refunded_amount is cumulative and is what gets recorded: a second partial
// refund reports the running total, so setting it is right where adding would
// double-count.
type RefundEvent struct {
	ID                  string    `json:"id"`
	SaleID              string    `json:"sale_id"`
	State               string    `json:"state"`
	CreateTime          time.Time `json:"create_time"`
	Amount              Amount    `json:"amount"`
	TotalRefundedAmount struct {
		Value string `json:"value"`
	} `json:"total_refunded_amount"`
}

// PaymentID is the payment this refund applies to.
func (r RefundEvent) PaymentID() string {
	if r.SaleID != "" {
		return r.SaleID
	}

	return r.ID
}

// RefundedTotal is how much of the payment has come back in all, preferring the
// provider's running total over this refund's own amount.
func (r RefundEvent) RefundedTotal() string {
	if r.TotalRefundedAmount.Value != "" {
		return r.TotalRefundedAmount.Value
	}

	return r.Amount.Total
}

// SetPaymentReconciliation records what the provider said about a payment.
//
// The pointers distinguish "the provider told us nothing" from "the provider told
// us zero", which for an amount are different answers and were the same blank
// column in the report this replaces.
type SetPaymentReconciliation struct {
	PaymentID           uuid.UUID
	ProviderStatus      *string
	ProviderAmountCents *int32
	ProviderFeeCents    *int32
}

// UpsertFundNote writes a donor's note on a fund.
type UpsertFundNote struct {
	FundID    uuid.UUID
	MemberID  uuid.UUID
	Body      string
	Anonymous bool
}

// RefundedPayment is what a refund changed, carrying the fund and donor so the
// activity entry can be written without a second lookup.
type RefundedPayment struct {
	PaymentID  uuid.UUID
	DonationID uuid.UUID
	FundID     uuid.UUID
	DonorID    uuid.UUID

	AmountCents int32

	// RefundedCents is the cumulative total returned for this payment, and
	// PreviouslyRefundedCents is what it was before this refund. The balance cares
	// about the total; the activity feed cares about the difference.
	RefundedCents           int32
	PreviouslyRefundedCents int32
}

// NewlyRefundedCents is how much came back in this refund alone.
//
// The feed reads as money moving, and a second partial refund moves only its own
// amount. Recording the running total instead would report the whole refunded sum
// again every time, and summing the feed would double-count.
func (r RefundedPayment) NewlyRefundedCents() int32 {
	return r.RefundedCents - r.PreviouslyRefundedCents
}

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

// NextPaymentAfter is when this fund next pays out.
//
// next_payment is the schedule's anchor -- the first payout date, set at fund
// creation -- not a pointer that something moves. Nothing ever advanced it, so a
// stored value went stale the moment it passed and the fund page told donors
// "next payment: paid" for the rest of the fund's life.
//
// Rolling it forward on read instead means the answer is right whether or not a
// payout actually ran, and there is no scheduled job to forget to wire up.
func (f Fund) NextPaymentAfter(now time.Time) time.Time {
	if f.NextPayment.IsZero() {
		return time.Time{}
	}

	// A one-off fund has a single payout date. There is nothing to roll forward
	// to, and a date in the past means it has been and gone.
	if !f.PayoutFrequency.Recurring() {
		return f.NextPayment
	}

	if !f.NextPayment.Before(now) {
		return f.NextPayment
	}

	// Days need no clamping -- every month has a tomorrow -- so the anchor's
	// time of day carries forward by whole days from the original date.
	if f.PayoutFrequency == PayoutFrequencyDaily {
		days := int(now.Sub(f.NextPayment) / (24 * time.Hour))

		next := f.NextPayment.AddDate(0, 0, days)
		if next.Before(now) {
			next = f.NextPayment.AddDate(0, 0, days+1)
		}

		return next
	}

	// Computed in one step from the anchor rather than by repeated addition,
	// which would accumulate the clamping below into real drift.
	months := (now.Year()-f.NextPayment.Year())*12 + int(now.Month()) - int(f.NextPayment.Month())

	next := addMonths(f.NextPayment, months)
	if next.Before(now) {
		next = addMonths(f.NextPayment, months+1)
	}

	return next
}

// addMonths keeps the day of the month where the target month has one, and
// clamps to its last day where it does not.
//
// time.AddDate normalises instead: 31 January plus one month is 3 March, which
// would walk a fund paying on the 31st steadily later every month.
func addMonths(t time.Time, months int) time.Time {
	year, month, day := t.Date()

	firstOfTarget := time.Date(year, month+time.Month(months), 1,
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())

	if last := lastDayOfMonth(firstOfTarget); day > last {
		day = last
	}

	return time.Date(firstOfTarget.Year(), firstOfTarget.Month(), day,
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

func lastDayOfMonth(t time.Time) int {
	// Day zero of the following month is the last day of this one.
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
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

// Expired reports whether the fund is past its end date. Distinct from Active,
// which is set by a person closing the fund: a fund can reach its expiry without
// anyone having touched it, and the admin listing shows the two differently
// because "it ran its course" and "someone shut it down" are not the same event.
func (f Fund) Expired() bool {
	return f.Expires != nil && !f.Expires.After(time.Now())
}

// Closed reports whether the fund still accepts donations.
func (f Fund) Closed() bool {
	return !f.Active || f.Expired()
}

// PayoutStats is what a fund disbursed. Reported alongside FundStats, which
// covers what came in: a finished fund is only legible with both halves, and
// "collected $500" on its own says nothing about whether it reached anyone.
type PayoutStats struct {
	TotalPaidCents  int64
	TotalRecipients int64
	TotalPayouts    int64
	LastPayoutDate  *time.Time
}

// ClosedFund is a fund that has ended, with both sides of its ledger.
type ClosedFund struct {
	Fund
	Payouts PayoutStats
}

// ClosedOn is when the fund stopped taking donations: its expiry if it had one,
// and otherwise the time it was last updated, which for a deactivated fund is
// when someone closed it.
func (c ClosedFund) ClosedOn() time.Time {
	if c.Expires != nil {
		return *c.Expires
	}

	return c.Updated
}

// Undisbursed is what was collected but never paid out. Non-zero is not
// necessarily wrong -- a fund can close holding a remainder too small to split
// -- but it is the first thing worth seeing on a fund that has ended.
func (c ClosedFund) Undisbursed() int64 {
	return int64(c.Stats.TotalDonated) - c.Payouts.TotalPaidCents
}

// UpsertFundImage is a fund picture on its way to storage: bytes this
// application produced, not the ones that were uploaded.
type UpsertFundImage struct {
	FundID      uuid.UUID
	S3Key       string
	ContentType string
	Width       int
	Height      int
	SHA256      string
}
