package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/enrollments"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func payee(t *testing.T) enrollments.Enrollment {
	t.Helper()

	return enrollments.Enrollment{
		ID:              uuid.New(),
		MemberID:        uuid.New(),
		MemberBCOName:   "ada",
		PaypalEmail:     "ada@test.org",
		FirstPayoutDate: time.Now().Add(-24 * time.Hour),
		Created:         time.Now().Add(-48 * time.Hour),
	}
}

func renderRow(t *testing.T, enrollment enrollments.Enrollment, inUnsentBatch bool) string {
	t.Helper()

	var out strings.Builder
	require.NoError(t, EnrollmentRow(enrollment, inUnsentBatch).Render(context.Background(), &out))

	return out.String()
}

// Removing somebody sets fund_enrollment.active = false, which every later plan
// honours -- and does nothing to a batch already planned, because SubmitBatch
// reads the payout rows by batch id and those froze the amount and address when
// the batch was built. The row has to say so, or removal looks like it stopped
// the payment.
func TestAQueuedPayoutIsMarkedOnTheRow(t *testing.T) {
	marked := renderRow(t, payee(t), true)

	if !strings.Contains(marked, "payout already queued") {
		t.Errorf("a queued payout should be marked:\n%s", marked)
	}
}

// Every other marker on this row appears only when something is wrong. A line on
// each row saying "nothing queued" is a line people learn to skip, which is the
// one time it matters that they do not.
func TestNothingIsSaidWhenNothingIsQueued(t *testing.T) {
	quiet := renderRow(t, payee(t), false)

	if strings.Contains(quiet, "already queued") {
		t.Errorf("an ordinary row should carry no marker:\n%s", quiet)
	}
}

// The marker sits a few pixels from a control that is one click from done, so
// the warning also goes where the decision is taken.
func TestTheConfirmationWarnsThatRemovalWillNotStopThePayout(t *testing.T) {
	queued := renderRow(t, payee(t), true)

	if !strings.Contains(queued, "will not stop it") {
		t.Errorf("the confirm should say removal does not stop the payout:\n%s", queued)
	}

	// And say what would, because "you cannot undo this here" without a next step
	// is a dead end.
	if !strings.Contains(queued, "reject the batch") {
		t.Errorf("the confirm should point at the thing that does stop it:\n%s", queued)
	}

	ordinary := renderRow(t, payee(t), false)

	if strings.Contains(ordinary, "will not stop it") {
		t.Error("an ordinary removal should ask the plain question")
	}
	if !strings.Contains(ordinary, "deactivate enrollment for ada") {
		t.Errorf("the plain question should still be asked:\n%s", ordinary)
	}
}

// A missing marker reads as "safe to remove". A lookup that failed and produced
// an empty map would be the page giving an assurance it never checked, which is
// the failure mode this whole panel exists to remove.
func TestAFailedLookupSaysSoRatherThanImplyingNothingIsQueued(t *testing.T) {
	var out strings.Builder
	require.NoError(t, CurrentEnrollments(
		[]enrollments.Enrollment{payee(t)}, unknownPending(),
	).Render(context.Background(), &out))

	page := out.String()

	if !strings.Contains(page, "could not check") {
		t.Errorf("an unknown state should be stated:\n%s", page)
	}

	// Known-and-empty is the other thing, and must stay silent.
	out.Reset()
	require.NoError(t, CurrentEnrollments(
		[]enrollments.Enrollment{payee(t)}, knownPending(nil),
	).Render(context.Background(), &out))

	if strings.Contains(out.String(), "could not check") {
		t.Error("a successful lookup that found nothing should say nothing")
	}
}

// The map is keyed by enrollment id, not member id. Keying it wrong would mark
// the wrong people, and marking everybody is as useless as marking nobody.
func TestOnlyTheQueuedEnrollmentIsMarked(t *testing.T) {
	queued, other := payee(t), payee(t)
	other.MemberBCOName = "grace"

	var out strings.Builder
	require.NoError(t, EnrollmentsList(
		[]enrollments.Enrollment{queued, other},
		knownPending(map[uuid.UUID]bool{queued.ID: true}),
	).Render(context.Background(), &out))

	page := out.String()

	if strings.Count(page, "payout already queued") != 1 {
		t.Errorf("want exactly one marked row:\n%s", page)
	}

	ada := strings.Index(page, "ada")
	grace := strings.Index(page, "grace")
	marker := strings.Index(page, "payout already queued")

	if !(ada < marker && marker < grace) {
		t.Error("the marker landed on the wrong row")
	}
}
