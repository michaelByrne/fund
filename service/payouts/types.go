package payouts

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPlanned          Status = "planned"
	StatusAwaitingApproval Status = "awaiting_approval"
	StatusReady            Status = "ready"
	StatusPending          Status = "pending"
	StatusPaid             Status = "paid"
	StatusFailed           Status = "failed"
	StatusUnclaimed        Status = "unclaimed"
	StatusReturned         Status = "returned"
	StatusBlocked          Status = "blocked"
	StatusOnhold           Status = "onhold"
	StatusCancelled        Status = "cancelled"
)

// Terminal reports whether a status will never change again without operator
// action. Notably 'unclaimed' is not terminal: PayPal auto-returns unclaimed
// items after 30 days, which moves them to 'returned'.
func (s Status) Terminal() bool {
	switch s {
	case StatusPaid, StatusFailed, StatusReturned, StatusBlocked, StatusCancelled:
		return true
	default:
		return false
	}
}

type Batch struct {
	ID               uuid.UUID
	FundID           uuid.UUID
	SenderBatchID    uuid.UUID
	ProviderBatchID  string
	AmountCents      int32
	NumEnrollments   int32
	Status           Status
	FailureReason    string
	Notes            string
	Description      string
	PayoutDate       time.Time
	ApprovalDeadline *time.Time
	ApprovedBy       *uuid.UUID
	ApprovedAt       *time.Time
	ReminderSentAt   *time.Time
	Created          time.Time
	Updated          time.Time
}

// AwaitingApproval reports whether this batch is still gated on a treasurer.
func (b Batch) AwaitingApproval() bool {
	return b.Status == StatusAwaitingApproval
}

// ApprovalExpired reports whether the approval window has closed. A batch in this
// state has not yet been swept to 'cancelled' but is no longer approvable.
func (b Batch) ApprovalExpired(now time.Time) bool {
	if b.ApprovalDeadline == nil || !b.AwaitingApproval() {
		return false
	}

	return !now.Before(*b.ApprovalDeadline)
}

type Payout struct {
	ID                   uuid.UUID
	FundEnrollmentID     uuid.UUID
	BatchID              uuid.UUID
	ProviderPayoutItemID string
	AmountCents          int32
	ProviderFeeCents     int32
	DestinationEmail     string
	Status               Status
	FailureReason        string
	Notes                string
	Description          string
	PayoutDate           time.Time
	Created              time.Time
	Updated              time.Time
}

type PayoutEnrollment struct {
	ID              uuid.UUID
	FundID          uuid.UUID
	MemberID        uuid.UUID
	MemberBCOName   string
	PaypalEmail     string
	FirstPayoutDate time.Time
	Created         time.Time
	Updated         time.Time
}

type InsertBatch struct {
	ID               uuid.UUID
	FundID           uuid.UUID
	SenderBatchID    uuid.UUID
	AmountCents      int32
	NumEnrollments   int32
	Status           Status
	Description      string
	Notes            string
	PayoutDate       time.Time
	ApprovalDeadline *time.Time
}

type InsertPayout struct {
	ID               uuid.UUID
	FundEnrollmentID uuid.UUID
	BatchID          uuid.UUID
	AmountCents      int32
	Status           Status
	Description      string
	Notes            string
	PayoutDate       time.Time
	DestinationEmail string
}

type ApproveBatch struct {
	BatchID    uuid.UUID
	ApprovedBy uuid.UUID
}

type RejectBatch struct {
	BatchID uuid.UUID
	Reason  string
}

type SetBatchStatus struct {
	BatchID       uuid.UUID
	Status        Status
	FailureReason string
}

type SetBatchSubmitted struct {
	BatchID         uuid.UUID
	ProviderBatchID string
}

type SetPayoutProviderItem struct {
	PayoutID             uuid.UUID
	ProviderPayoutItemID string
}

type SetPayoutStatusByItem struct {
	ProviderPayoutItemID string
	Status               Status
	FailureReason        string
	ProviderFeeCents     int32
}

type SetPayoutResult struct {
	PayoutID             uuid.UUID
	ProviderPayoutItemID string
	Status               Status
	FailureReason        string
	ProviderFeeCents     int32
}

// PlanBatch is the request to build a batch for a fund's upcoming payout date.
type PlanBatch struct {
	FundID          uuid.UUID
	PayoutDate      time.Time
	AmountCents     int32
	Description     string
	Notes           string
	ApprovalWindow  time.Duration
	RequireApproval bool
}

// ProviderPayoutItem is one line of a batch as submitted to the payments provider.
type ProviderPayoutItem struct {
	PayoutID      uuid.UUID
	ReceiverEmail string
	AmountCents   int32
	Note          string
}

// ProviderBatchResult is what the provider returns after accepting a batch.
type ProviderBatchResult struct {
	ProviderBatchID string
	Status          string
	Items           []ProviderItemResult
}

type ProviderItemResult struct {
	PayoutID             uuid.UUID
	ProviderPayoutItemID string
	Status               string
	FeeCents             int32
}

// DueFund is a fund whose scheduled payout date has arrived. Name is carried so
// the planner's logs and errors can say which fund they mean without a second
// lookup -- these run unattended, and a bare uuid in a cron log is not enough to
// act on.
type DueFund struct {
	ID          uuid.UUID
	Name        string
	Frequency   string
	NextPayment time.Time
}
