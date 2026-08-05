package authweb

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The message and link were concatenated into the query string unescaped, so an
// ampersand in a message split it into a bogus third parameter and spaces produced
// a malformed URL. Both now round-trip.
func TestErrRedirectEscapesItsParameters(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		link string
	}{
		{"plain", "something went wrong", "/login"},
		{"with spaces and punctuation", "that email has not been approved. ask an admin.", "/register"},
		{"with an ampersand", "tea & biscuits failed", "/login"},
		{"with a question mark", "who? that account", "/login"},
		{"with an equals sign", "a=b was rejected", "/password"},
		{"empty link", "no link here", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/register", nil)

			errRedirect(w, r, c.msg, c.link)

			if w.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
			}

			location := w.Header().Get("Location")

			parsed, err := url.Parse(location)
			if err != nil {
				t.Fatalf("Location %q does not parse: %v", location, err)
			}

			if parsed.Path != "/auth/error" {
				t.Errorf("path = %q, want /auth/error", parsed.Path)
			}

			if got := parsed.Query().Get("msg"); got != c.msg {
				t.Errorf("msg round-tripped as %q, want %q", got, c.msg)
			}

			if got := parsed.Query().Get("link"); got != c.link {
				t.Errorf("link round-tripped as %q, want %q", got, c.link)
			}

			// A raw space in a Location header is malformed regardless of what the
			// parser tolerates.
			if strings.Contains(location, " ") {
				t.Errorf("Location contains an unescaped space: %q", location)
			}
		})
	}
}
