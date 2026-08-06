package common

import (
	"context"
	"strings"
	"testing"

	"boardfund/service/members"

	"github.com/google/uuid"
)

// The nav decides logged-in purely by member != nil, so a pointer to a zero-value
// member is indistinguishable from a real session. Handlers reached this state by
// writing `&member` after a failed type assertion, which rendered logout and hid
// login on the one page a signed-out visitor was most likely to land on.
func TestNavLinksByAuthState(t *testing.T) {
	ctx := context.Background()

	admin := members.Member{ID: uuid.New(), Roles: []members.MemberRole{members.AdminRole}}
	plain := members.Member{ID: uuid.New()}

	cases := []struct {
		name       string
		member     *members.Member
		wantLogin  bool
		wantLogout bool
		wantAdmin  bool
	}{
		{"signed out", nil, true, false, false},
		{"signed in", &plain, false, true, false},
		{"admin", &admin, false, true, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			if err := Links(c.member, "/").Render(ctx, &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			html := out.String()

			if got := strings.Contains(html, `href="/login"`); got != c.wantLogin {
				t.Errorf("login link present=%v, want %v", got, c.wantLogin)
			}

			if got := strings.Contains(html, `href="/logout"`); got != c.wantLogout {
				t.Errorf("logout link present=%v, want %v", got, c.wantLogout)
			}

			if got := strings.Contains(html, `href="/admin"`); got != c.wantAdmin {
				t.Errorf("admin link present=%v, want %v", got, c.wantAdmin)
			}
		})
	}
}

// The unauthenticated error page is the specific case that was wrong. Asserting
// on it directly means the next handler to copy the pattern fails here rather
// than shipping.
func TestUnauthenticatedErrorPageOffersLogin(t *testing.T) {
	var out strings.Builder
	if err := ErrorMessage(nil, "unauthorized", "/", "/").Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	html := out.String()

	if !strings.Contains(html, `href="/login"`) {
		t.Error("an unauthorized page must offer a way to log in")
	}

	if strings.Contains(html, `href="/logout"`) {
		t.Error("an unauthorized visitor has no session to log out of")
	}
}
