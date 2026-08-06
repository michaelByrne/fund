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
	KindDonationResumed     Kind = "donation_resumed"
	KindPaymentReceived     Kind = "payment_received"
	KindPaymentFailed       Kind = "payment_failed"
	KindMemberEnrolled      Kind = "member_enrolled"
	KindEnrollmentCancelled Kind = "enrollment_cancelled"
	KindBatchPlanned        Kind = "payout_batch_planned"
	KindBatchApproved       Kind = "payout_batch_approved"
	KindBatchRejected       Kind = "payout_batch_rejected"
	KindBatchExpired        Kind = "payout_batch_expired"
	KindBatchSubmitted      Kind = "payout_batch_submitted"
	KindBatchSettled        Kind = "payout_batch_settled"
	KindFundClosed          Kind = "fund_closed"
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

	// DedupeKey makes recording this event idempotent. Empty for anything that
	// happens once by construction -- an admin clicking a button -- and set by
	// webhook handlers, where at-least-once delivery means the same event arrives
	// again whenever an acknowledgement is lost after the work was done.
	//
	// It must identify the occurrence, not the subject: a donation receives many
	// payments, and keying on the donation would record only the first.
	DedupeKey string

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

// ActorIsSubject reports whether the person who acted is the person the event is
// about: a donor starting their own donation, who is both. The feed already
// names the subject, so repeating them as the actor reads as noise rather than
// as attribution.
func (e Event) ActorIsSubject() bool {
	return e.ActorMemberID != nil &&
		e.SubjectMemberID != nil &&
		*e.ActorMemberID == *e.SubjectMemberID
}
