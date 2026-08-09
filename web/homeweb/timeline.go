package homeweb

import (
	"strings"

	"boardfund/service/fundevents"
)

// timelineLabel is the donor-facing wording for an event.
//
// Deliberately not the admin feed's wording. "payout batch submitted" describes
// an operation; "payouts sent" describes what happened to the money, which is
// what someone who gave to this fund came to find out. The two lists are allowed
// to drift, because they are written for different readers.
func timelineLabel(kind fundevents.Kind) string {
	switch kind {
	case fundevents.KindBatchPlanned:
		return "payouts planned"
	case fundevents.KindBatchApproved:
		return "payouts approved"
	case fundevents.KindBatchRejected:
		return "payouts held back"
	case fundevents.KindBatchExpired:
		return "payouts expired unapproved"
	case fundevents.KindBatchSubmitted:
		return "payouts sent"
	case fundevents.KindBatchSettled:
		return "payouts completed"
	case fundevents.KindFundUpdated:
		return "fund details changed"
	case fundevents.KindFundCreated:
		return "fund opened"
	case fundevents.KindFundClosed:
		return "fund closed"
	default:
		// Unreachable while Kind.Public is an allowlist and every entry on it is
		// named above. Still legible if the two ever disagree, rather than a row
		// with a blank where the label goes.
		return strings.ReplaceAll(string(kind), "_", " ")
	}
}

// chronological reverses the feed, oldest first.
//
// The admin feed reads newest first because the question there is "what just
// happened". This page is read after the fact, by someone asking what became of
// a fund that has ended, and that is a story: money planned, approved, sent,
// then the fund closed. Told backwards it is a list of unrelated states.
//
// Copied rather than reversed in place. The slice belongs to the caller, and a
// helper that quietly reorders its argument is the kind of thing that breaks
// something else later.
func chronological(events []fundevents.PublicEvent) []fundevents.PublicEvent {
	ordered := make([]fundevents.PublicEvent, len(events))

	for i, event := range events {
		ordered[len(events)-1-i] = event
	}

	return ordered
}
