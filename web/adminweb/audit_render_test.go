package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/adminevents"
	"boardfund/service/members"

	"github.com/google/uuid"
)

func renderAudit(t *testing.T, events []adminevents.Event) string {
	t.Helper()

	var out strings.Builder
	member := members.Member{ID: uuid.New(), BCOName: "michael"}

	if err := AdminAudit(events, &member, "/admin/audit").Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

func grantedBy(actor *uuid.UUID, actorName string, subject uuid.UUID) adminevents.Event {
	return adminevents.Event{
		ID:              uuid.New(),
		Kind:            adminevents.KindAdminGranted,
		OccurredAt:      time.Now(),
		ActorMemberID:   actor,
		ActorName:       actorName,
		SubjectMemberID: subject,
		SubjectName:     "promoted",
	}
}

// The two directions must not read alike. A log where every line says the same
// thing records only that something happened.
func TestGrantAndRevokeReadDifferently(t *testing.T) {
	actor := uuid.New()

	granted := grantedBy(&actor, "granter", uuid.New())
	revoked := granted
	revoked.Kind = adminevents.KindAdminRevoked

	if !strings.Contains(renderAudit(t, []adminevents.Event{granted}), "granted admin") {
		t.Error("a grant should say so")
	}

	page := renderAudit(t, []adminevents.Event{revoked})
	if !strings.Contains(page, "revoked admin") {
		t.Error("a revocation should say so")
	}
	if strings.Contains(page, ">granted admin<") {
		t.Error("a revocation must not read as a grant")
	}
}

// An event from before this log existed, or made in the Cognito console, has no
// actor. The nil uuid is a real value that would render as a link to a member
// page for nobody, and a blank cell would read as a rendering fault rather than
// as an honest gap.
func TestAnUnattributedChangeSaysSoRatherThanLinkingToNobody(t *testing.T) {
	page := renderAudit(t, []adminevents.Event{grantedBy(nil, "", uuid.New())})

	if !strings.Contains(page, "not recorded") {
		t.Error("an event with no actor should say the actor was not recorded")
	}

	if strings.Contains(page, uuid.Nil.String()) {
		t.Errorf("the nil uuid must never reach a link:\n%s", page)
	}
}

// Self-promotion and bootstrapping share this shape, and it is the one an
// auditor is looking for. Named rather than left as a repeated name in two
// columns.
func TestChangingYourOwnAccessIsCalledOut(t *testing.T) {
	self := uuid.New()

	event := grantedBy(&self, "michael", self)
	event.SubjectName = "michael"

	if !strings.Contains(renderAudit(t, []adminevents.Event{event}), "themselves") {
		t.Error("an admin changing their own access should be marked")
	}
}

// An empty table is the ordinary state of a fresh deployment. It must not be
// mistaken for a log that is not being written, and it must say what it does not
// cover.
func TestAnEmptyLogExplainsItself(t *testing.T) {
	page := renderAudit(t, nil)

	if !strings.Contains(page, "no admin access has been granted or revoked") {
		t.Error("an empty log should say it is empty")
	}
	if !strings.Contains(page, "cognito") {
		t.Error("an empty log should say that changes made outside the app are not recorded")
	}
}

// The subject is always a real member, so their name is a link into the admin
// section rather than text an auditor has to search for.
func TestBothPartiesLinkToTheirMemberPage(t *testing.T) {
	actor, subject := uuid.New(), uuid.New()

	page := renderAudit(t, []adminevents.Event{grantedBy(&actor, "granter", subject)})

	for _, id := range []uuid.UUID{actor, subject} {
		if !strings.Contains(page, "/admin/member/"+id.String()) {
			t.Errorf("no link to the member page for %s", id)
		}
	}
}
