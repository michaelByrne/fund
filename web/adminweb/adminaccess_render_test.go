package adminweb

import (
	"context"
	"strings"
	"testing"

	"boardfund/service/members"

	"github.com/google/uuid"
)

func TestAdminAccessControl(t *testing.T) {
	viewed := members.Member{ID: uuid.New(), BCOName: "michael"}

	promote := "hx-post=\"/admin/member/promote/" + viewed.ID.String() + "\""
	revoke := "hx-post=\"/admin/member/demote/" + viewed.ID.String() + "\""

	cases := []struct {
		name       string
		state      AdminAccessState
		wantValue  string
		wantAction string
	}{
		{"not an admin", AdminAccessState{}, ">no<", promote},
		{"already an admin", AdminAccessState{IsAdmin: true}, ">yes<", revoke},
		// Self-revocation is the lockout case: an admin removing their own access
		// when they are the only one leaves nobody able to restore it.
		{"yourself", AdminAccessState{IsAdmin: true, IsSelf: true}, ">yes<", ""},
		// A failed Cognito lookup must not render a control, which would offer to
		// undo whatever is actually true.
		{"lookup failed", AdminAccessState{Unknown: true}, ">unavailable<", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			if err := AdminAccess(viewed, c.state).Render(context.Background(), &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			html := out.String()

			if !strings.Contains(html, c.wantValue) {
				t.Errorf("should report %s", c.wantValue)
			}

			if c.wantAction != "" && !strings.Contains(html, c.wantAction) {
				t.Errorf("should offer %s", c.wantAction)
			}

			if c.wantAction == "" {
				if strings.Contains(html, promote) || strings.Contains(html, revoke) {
					t.Error("should offer no control")
				}
			}
		})
	}
}

func TestAdminAccessExplainsTheLoginDelay(t *testing.T) {
	viewed := members.Member{ID: uuid.New(), BCOName: "michael"}

	// Cognito stamps groups into the ID token at login, so a promotion does
	// nothing to the member's current session. Without the notice the toggle
	// looks like it failed.
	var changed strings.Builder
	if err := AdminAccess(viewed, AdminAccessState{IsAdmin: true, Changed: true}).Render(context.Background(), &changed); err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(changed.String(), "next time michael logs in") {
		t.Error("a change should say when it takes effect")
	}

	// The page load is not a change, so it should not claim one just happened.
	var loaded strings.Builder
	if err := AdminAccess(viewed, AdminAccessState{IsAdmin: true}).Render(context.Background(), &loaded); err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.Contains(loaded.String(), "logs in") {
		t.Error("an unchanged row should not carry the notice")
	}
}
