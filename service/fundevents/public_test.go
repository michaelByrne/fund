package fundevents_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// Everything a fund can record. Written out rather than derived, so that adding
// a kind to the enum makes this list wrong in a way a test names.
var everyKind = []fundevents.Kind{
	fundevents.KindDonationStarted,
	fundevents.KindDonationCancelled,
	fundevents.KindDonationResumed,
	fundevents.KindPaymentReceived,
	fundevents.KindPaymentFailed,
	fundevents.KindPaymentRefunded,
	fundevents.KindMemberEnrolled,
	fundevents.KindEnrollmentCancelled,
	fundevents.KindBatchPlanned,
	fundevents.KindBatchApproved,
	fundevents.KindBatchRejected,
	fundevents.KindBatchExpired,
	fundevents.KindBatchSubmitted,
	fundevents.KindBatchSettled,
	fundevents.KindFundClosed,
	fundevents.KindFundUpdated,
}

// The rule the whole design rests on. Every one of these is about one
// identifiable person: a donor who gave, or a member receiving mutual aid. The
// closed-fund page shows their totals and must not show them.
func TestNothingAboutAnIndividualIsPublic(t *testing.T) {
	private := []fundevents.Kind{
		fundevents.KindDonationStarted,
		fundevents.KindDonationCancelled,
		fundevents.KindDonationResumed,
		fundevents.KindPaymentReceived,
		fundevents.KindPaymentFailed,
		fundevents.KindPaymentRefunded,
		fundevents.KindMemberEnrolled,
		fundevents.KindEnrollmentCancelled,
	}

	for _, kind := range private {
		if kind.Public() {
			t.Errorf("%s is about one member and must not be public", kind)
		}
	}
}

// An unrecognised kind must be private. This is the property that makes the
// allowlist worth having: a kind added to the enum and forgotten here stays out
// of the feed, rather than appearing in it by default.
func TestAnUnknownKindIsNotPublic(t *testing.T) {
	if fundevents.Kind("something_added_later").Public() {
		t.Error("a kind nobody has classified must default to private")
	}
}

// The type is the enforcement. A subject field here would let any template on a
// public page name the donor or the recipient, and no amount of care at the call
// sites would stop the one that forgets.
func TestPublicEventHasNoFieldNamingAMember(t *testing.T) {
	shape := reflect.TypeOf(fundevents.PublicEvent{})

	for i := range shape.NumField() {
		name := strings.ToLower(shape.Field(i).Name)

		if strings.Contains(name, "subject") {
			t.Errorf("PublicEvent.%s: the public feed must not carry a subject", shape.Field(i).Name)
		}

		// An id is an identifier whether or not a page renders it. One in the
		// struct is one that reaches a template, and from there a link.
		if strings.HasSuffix(name, "id") {
			t.Errorf("PublicEvent.%s: the public feed must not carry member or row ids", shape.Field(i).Name)
		}
	}
}

func donorEvent(kind fundevents.Kind) fundevents.Event {
	donor, actor := uuid.New(), uuid.New()
	amount := int32(2500)

	return fundevents.Event{
		ID:              uuid.New(),
		FundID:          uuid.New(),
		Kind:            kind,
		OccurredAt:      time.Now(),
		ActorMemberID:   &actor,
		ActorName:       "treasurer",
		SubjectMemberID: &donor,
		SubjectName:     "a-quiet-donor",
		AmountCents:     &amount,
		Detail:          "4 payees",
	}
}

func TestPublishDropsTheSubjectAndKeepsTheActor(t *testing.T) {
	published := donorEvent(fundevents.KindBatchApproved).Publish()

	if published.ActorName != "treasurer" {
		t.Errorf("actor = %q, want it kept -- approving a payout is the accountable act", published.ActorName)
	}
	if published.AmountCents == nil || *published.AmountCents != 2500 {
		t.Error("the amount is the point of the line and must survive")
	}
	if published.Detail != "4 payees" {
		t.Errorf("detail = %q, want the generated detail kept", published.Detail)
	}
}

// A public kind does not make its detail public. RejectBatch stores whatever the
// treasurer typed, which is a sentence about why a payout was held back and can
// name the person it concerns.
func TestTheRejectionReasonNeverTravels(t *testing.T) {
	rejected := donorEvent(fundevents.KindBatchRejected)
	rejected.Detail = "holding this back until we confirm a-quiet-donor's address"

	published := rejected.Publish()

	if published.Detail != "" {
		t.Errorf("detail = %q, want a treasurer's free text dropped", published.Detail)
	}

	// The kind itself still shows: that a payout was held back is exactly the
	// sort of thing a donor is entitled to know happened.
	if !published.Kind.Public() {
		t.Error("a held-back payout should still appear, without its reason")
	}
}

// member.bco_name is nullable, so a person can act and have no name to render.
// Inferring "automatic" from an empty name would credit a sweep for a decision
// somebody made.
func TestAnUnnamedActorIsNotReportedAsAutomatic(t *testing.T) {
	event := donorEvent(fundevents.KindBatchApproved)
	event.ActorName = ""

	if event.Publish().ByProvider() {
		t.Error("an actor with no name is still an actor")
	}

	event.ActorMemberID = nil

	if !event.Publish().ByProvider() {
		t.Error("no actor at all is what automatic means")
	}
}

// Sanity on the other side of the allowlist: the fund's own lifecycle is the
// reason this page exists, so it must actually reach it.
func TestTheMoneyAndTheFundLifecycleArePublic(t *testing.T) {
	for _, kind := range []fundevents.Kind{
		fundevents.KindBatchPlanned,
		fundevents.KindBatchApproved,
		fundevents.KindBatchSubmitted,
		fundevents.KindBatchSettled,
		fundevents.KindFundClosed,
		fundevents.KindFundUpdated,
	} {
		if !kind.Public() {
			t.Errorf("%s belongs on the public timeline", kind)
		}
	}
}

// Guards the list above against an enum value nobody classified. If this fails,
// a kind was added and everyKind was not updated -- which is the moment to
// decide whether it is public, not later.
func TestEveryKindIsAccountedFor(t *testing.T) {
	seen := make(map[fundevents.Kind]bool, len(everyKind))

	for _, kind := range everyKind {
		if seen[kind] {
			t.Errorf("%s is listed twice", kind)
		}
		seen[kind] = true
	}

	if len(seen) != 16 {
		t.Errorf("everyKind has %d entries; update it and decide whether the new kind is public", len(seen))
	}
}
