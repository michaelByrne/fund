package adminweb

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"boardfund/service/members"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// formRig posts a urlencoded body at one admin route, the way htmx does, and
// hands back what was logged.
//
// The services are nil: every case here is refused by the handler before it
// reaches one, which is the point being made. A test that got past them would
// panic rather than quietly pass.
func formRig(
	t *testing.T,
	pattern, path string,
	handler func(*AdminHandlers) http.HandlerFunc,
	form url.Values,
) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()

	gob.Register(members.Member{})

	var out bytes.Buffer

	sessions := scs.New()
	handlers := &AdminHandlers{
		sessionManager: sessions,
		logger:         slog.New(slog.NewJSONHandler(&out, nil)),
	}

	router := http.NewServeMux()
	router.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		sessions.Put(r.Context(), "member", members.Member{ID: uuid.New()})

		handler(handlers)(w, r)
	})

	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	// What htmx sends for a form with no hx-encoding, which is every admin form
	// that does not carry a picture.
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")

	recorder := httptest.NewRecorder()
	sessions.LoadAndSave(router).ServeHTTP(recorder, request)

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}

		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))

		records = append(records, record)
	}

	return recorder, records
}

// The outage. Both forms post urlencoded, and ParseMultipartForm answers
// ErrNotMultipart for exactly that -- so every submission was refused before a
// single field was read, and enrolling anybody was impossible.
//
// Asserted through the refusal reason rather than the status, because the bug
// and a genuinely missing field both produce 400 and the status alone cannot
// tell them apart. That is the same blindness that let this run in production.
func TestUrlencodedAdminFormsAreRead(t *testing.T) {
	// Each form deliberately omits its last field, so a handler that read the
	// form stops at that check and never reaches its nil service. The two
	// outcomes are then told apart by the reason, not the status.
	for _, tc := range []struct {
		name    string
		pattern string
		path    string
		handler func(*AdminHandlers) http.HandlerFunc
		form    url.Values
		stopsAt string
	}{
		{
			name:    "enrollment",
			pattern: "POST /admin/enrollment",
			path:    "/admin/enrollment",
			handler: func(h *AdminHandlers) http.HandlerFunc { return h.createEnrollment },
			form:    enrollmentForm(),
			stopsAt: "paypal",
		},
		{
			name:    "approved email",
			pattern: "POST /admin/approved",
			path:    "/admin/approved",
			handler: func(h *AdminHandlers) http.HandlerFunc { return h.addApprovedEmail },
			form:    url.Values{},
			stopsAt: "email address",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, records := formRig(t, tc.pattern, tc.path, tc.handler, tc.form)

			require.NotEmpty(t, records)

			reason, _ := records[0]["reason"].(string)

			if !strings.Contains(reason, tc.stopsAt) {
				t.Errorf("reason = %q, want the handler to have read the form and stopped at %q",
					reason, tc.stopsAt)
			}

			if strings.Contains(reason, "could not read that form") {
				t.Fatalf("the form was not read at all: %v", records[0])
			}
		})
	}
}

// A refusal has to say which decision was taken. The request line records that a
// POST answered 400; it cannot say which of seven checks fired, and before this
// the operator message was the same sentence for all of them.
func TestARefusalSaysWhyAndWhere(t *testing.T) {
	_, records := formRig(t, "POST /admin/enrollment", "/admin/enrollment",
		func(h *AdminHandlers) http.HandlerFunc { return h.createEnrollment }, enrollmentForm())

	require.NotEmpty(t, records, "a refusal should leave a line")

	refusal := records[0]

	if refusal["msg"] != "request refused" {
		t.Fatalf("first line = %v, want the refusal", refusal)
	}

	// A 4xx is the handler looking at a request and saying no, which is the
	// system working, not failing.
	if refusal["level"] != "INFO" {
		t.Errorf("level = %v, want INFO for a 400", refusal["level"])
	}
	if refusal["status"] != float64(http.StatusBadRequest) {
		t.Errorf("status = %v, want 400", refusal["status"])
	}

	// The paypal field is the one this form leaves out, and the message names it
	// rather than saying "that request was not valid."
	reason, _ := refusal["reason"].(string)
	if !strings.Contains(reason, "paypal") {
		t.Errorf("reason = %q, want it to name the missing field", reason)
	}

	// And where the decision was taken, which is what distinguishes seven
	// otherwise identical 400s from one another.
	at, _ := refusal["refused_at"].(string)
	if !strings.HasPrefix(at, "handlers.go:") {
		t.Errorf("refused_at = %q, want the handler line that decided", at)
	}
}

// The frame walk has to skip this file's plumbing however deep it is: badRequest
// reaches renderError through one hop, and payouts.go calls renderError
// directly.
func TestTheReportedLineIsTheHandlersNotTheHelpers(t *testing.T) {
	_, records := formRig(t, "POST /admin/enrollment", "/admin/enrollment",
		func(h *AdminHandlers) http.HandlerFunc { return h.createEnrollment }, enrollmentForm())

	at, _ := records[0]["refused_at"].(string)

	if strings.HasPrefix(at, "errors.go:") {
		t.Errorf("refused_at = %q, which is the helper rather than the caller", at)
	}
}

// enrollmentForm is what the confirm-enrollment form posts, minus the paypal
// address the admin types in.
func enrollmentForm() url.Values {
	form := url.Values{}
	form.Set("fund", uuid.NewString())
	form.Set("member", uuid.NewString())
	form.Set("username", "michael")

	return form
}
