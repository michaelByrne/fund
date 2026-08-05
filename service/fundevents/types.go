// Package fundevents records what happened to a fund, append-only.
//
// The domain tables cannot answer this on their own: `created` is immutable and
// therefore exact, but `updated` is overwritten on every change, so a row shows
// only its most recent state transition and never says who caused it.
package fundevents

import (
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindDonationStarted     Kind = "donation_started"
	KindDonationCancelled   Kind = "donation_cancelled"
	KindPaymentReceived     Kind = "payment_received"
	KindMemberEnrolled      Kind = "member_enrolled"
	KindEnrollmentCancelled Kind = "enrollment_cancelled"
	KindBatchPlanned        Kind = "payout_batch_planned"
	KindBatchApproved       Kind = "payout_batch_approved"
	KindBatchRejected       Kind = "payout_batch_rejected"
	KindBatchSubmitted      Kind = "payout_batch_submitted"
	KindBatchSettled        Kind = "payout_batch_settled"
)

// Record is one thing that happened.
type Record struct {
	FundID uuid.UUID
	Kind   Kind

	// OccurredAt is when the thing happened, which is not always now: a provider
	// webhook reports something that already took place. Zero means now.
	OccurredAt time.Time

	// ActorMemberID is who caused it. Nil when nobody did -- a provider webhook,
	// the approval sweep, a reconciliation run. That distinction is the reason
	// the column exists, so it is left nil rather than filled with a stand-in.
	ActorMemberID *uuid.UUID

	// SubjectMemberID is who it concerns: the donor, or the enrollee being paid.
	SubjectMemberID *uuid.UUID

	AmountCents *int32

	// Detail is free text for the reader, such as a provider's cancellation
	// reason. Not parsed by anything.
	Detail string

	// ReferenceID is the domain row this describes: a donation, an enrollment,
	// a batch.
	ReferenceID *uuid.UUID
}

// Event is a recorded Record, with the actor and subject names resolved.
type Event struct {
	ID              uuid.UUID
	FundID          uuid.UUID
	Kind            Kind
	OccurredAt      time.Time
	ActorMemberID   *uuid.UUID
	ActorName       string
	SubjectMemberID *uuid.UUID
	SubjectName     string
	AmountCents     *int32
	Detail          string
	ReferenceID     *uuid.UUID
	Created         time.Time
}

// ByProvider reports whether this happened without a person doing it -- a
// webhook, a sweep, a reconciler. The feed says so explicitly, because "nobody
// is listed" and "we do not know who" must not look the same.
func (e Event) ByProvider() bool {
	return e.ActorMemberID == nil
}
