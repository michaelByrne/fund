package homeweb

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectNavigatesRatherThanSwapping(t *testing.T) {
	// The archive and active-fund rows are clicked through hx-get. An XHR follows
	// a 303 transparently, so htmx never learns a redirect happened and swaps the
	// destination page into the row that was clicked. Only HX-Redirect navigates.
	req := httptest.NewRequest(http.MethodGet, "/fund/abc/summary", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	redirect(rec, req, "/donate/abc")

	if got := rec.Header().Get("HX-Redirect"); got != "/donate/abc" {
		t.Errorf("HX-Redirect = %q, want /donate/abc", got)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: htmx ignores the header on a 3xx it never sees", rec.Code)
	}

	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("Location = %q, want none for an htmx request", loc)
	}
}

func TestRedirectStillWorksForPlainNavigation(t *testing.T) {
	// Pasting the URL into the address bar sends no HX-Request header, and a
	// browser needs a real redirect.
	req := httptest.NewRequest(http.MethodGet, "/fund/abc/summary", nil)

	rec := httptest.NewRecorder()
	redirect(rec, req, "/donate/abc")

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rec.Code)
	}

	if got := rec.Header().Get("Location"); got != "/donate/abc" {
		t.Errorf("Location = %q, want /donate/abc", got)
	}

	if got := rec.Header().Get("HX-Redirect"); got != "" {
		t.Errorf("HX-Redirect = %q, want none for a plain navigation", got)
	}
}
