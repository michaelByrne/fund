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
	KindPaymentRefunded     Kind = "payment_refunded"
	KindMemberEnrolled      Kind = "member_enrolled"
	KindEnrollmentCancelled Kind = "enrollment_cancelled"
	KindBatchPlanned        Kind = "payout_batch_planned"
	KindBatchApproved       Kind = "payout_batch_approved"
	KindBatchRejected       Kind = "payout_batch_rejected"
	KindBatchExpired        Kind = "payout_batch_expired"
	KindBatchSubmitted      Kind = "payout_batch_submitted"
	KindBatchSettled        Kind = "payout_batch_settled"
	KindFundClosed          Kind = "fund_closed"
	KindFundUpdated         Kind = "fund_updated"
)

// Public reports whether this kind belongs on a timeline that donors can read.
//
// The list is an allowlist, and that is the whole design: a kind added to the
// enum and forgotten here stays out of the public feed. The reverse default --
// public unless excluded -- would make every future event kind a privacy
// decision that nobody remembers to make.
//
// What is in: the fund's own lifecycle and the movement of money in bulk. A
// donor's question is "was the money collected actually paid out, and when",
// and the batch events answer it.
//
// What is out: everything about one identifiable person. A donation starting, a
// payment arriving, a member enrolling. Those are the fund's business and the
// admin feed's, and the page already shows their totals -- a donor giving
// quietly should not be listed, and a recipient of mutual aid should never be.
func (k Kind) Public() bool {
	switch k {
	case KindBatchPlanned,
		KindBatchApproved,
		KindBatchRejected,
		KindBatchExpired,
		KindBatchSubmitted,
		KindBatchSettled,
		KindFundUpdated,
		KindFundClosed:
		return true
	default:
		return false
	}
}

// detailPublic reports whether this kind's Detail was composed by the
// application rather than typed by a person.
//
// A public kind does not make its detail public. RejectBatch stores whatever
// reason the treasurer wrote, which is a sentence about why a payout was held
// back and can perfectly reasonably name the person it concerns -- so it is the
// one field on an otherwise public event that must not travel. Every other
// detail on the list is generated: "4 payees", "approval window expired", the
// provider's settlement status, the list of fields an edit changed.
func (k Kind) detailPublic() bool {
	return k != KindBatchRejected
}

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

// PublicEvent is one line of the timeline a donor sees.
//
// A separate type rather than a filtered Event, because the difference that
// matters is which fields exist. There is no subject here at all, so no template
// on a public page can name the donor or the recipient an event was about -- not
// by mistake, and not by a later edit that looks harmless. The one name it can
// carry is the actor's, and only for events where a person acted in an official
// capacity: a treasurer who approved a payout should be nameable, because that
// is what accountability for the money means.
type PublicEvent struct {
	Kind       Kind
	OccurredAt time.Time
	ActorName  string

	// Automatic is carried rather than inferred from an empty ActorName.
	// member.bco_name is nullable, so a person can act and have no name to show,
	// and rendering that as "automatic" would credit a sweep for a decision
	// somebody made.
	Automatic bool

	AmountCents *int32
	Detail      string
}

// ByProvider reports whether this happened without a person doing it -- a sweep,
// a webhook, an expiry. Said explicitly for the same reason as on Event: a blank
// where a name should be must not read as an omission.
func (e PublicEvent) ByProvider() bool {
	return e.Automatic
}

// Publish projects an event onto the public timeline, dropping everything that
// identifies whoever it was about.
//
// The actor's name survives; the subject's does not, and neither do the member
// ids or the reference id -- an id is an identifier whether or not it is
// rendered, and a page that carries one invites the query that resolves it.
func (e Event) Publish() PublicEvent {
	public := PublicEvent{
		Kind:        e.Kind,
		OccurredAt:  e.OccurredAt,
		ActorName:   e.ActorName,
		Automatic:   e.ByProvider(),
		AmountCents: e.AmountCents,
	}

	if e.Kind.detailPublic() {
		public.Detail = e.Detail
	}

	return public
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
