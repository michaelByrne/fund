package authweb

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func issued(t *testing.T, secure bool) *http.Cookie {
	t.Helper()

	recorder := httptest.NewRecorder()

	AuthHandlers{secureCookies: secure}.setTokenCookie(
		"access-token", "a-token", time.Now().Add(time.Hour), recorder,
	)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set %d cookies, want 1", len(cookies))
	}

	return cookies[0]
}

// This cookie is the credential. middlewares.Verify reads it, and the admin
// group claim inside the token it carries is the whole of what grants admin
// access, so each flag here is load-bearing.
func TestTheTokenCookieIsProtected(t *testing.T) {
	cookie := issued(t, true)

	if !cookie.HttpOnly {
		t.Error("a script that can read the token can act as the member")
	}

	// An HTTP-to-HTTPS redirect at the edge does not help: the browser has
	// already put the cookie on the wire by the time the redirect arrives.
	if !cookie.Secure {
		t.Error("the token must not be sent over plain http")
	}

	// The only thing standing in for a CSRF token. Lax is also the browser
	// default when unset, which is exactly why it is set explicitly -- a default
	// nobody wrote down is one that vanishes the first time someone needs
	// SameSite=None for an embed.
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("same-site is %v, want Lax", cookie.SameSite)
	}
}

// Local development is served over plain HTTP, where a Secure cookie is never
// sent back and login silently fails. The other two flags cost nothing there and
// stay on.
func TestSecureIsTheOnlyFlagThatVariesByEnvironment(t *testing.T) {
	cookie := issued(t, false)

	if cookie.Secure {
		t.Error("a secure cookie over plain http is never returned, and nobody can log in")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Error("only Secure depends on the environment")
	}
}
