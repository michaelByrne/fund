package middlewares

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boardfund/logging"
	"boardfund/service/members"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
)

// serve runs one request through the middleware, wired the way root.go wires it:
// inside scs's LoadAndSave, which is where the session comes from.
func serve(t *testing.T, r *http.Request, handler http.HandlerFunc) ([]map[string]any, *http.Response) {
	t.Helper()

	var out bytes.Buffer

	logger := slog.New(logging.WithContext(slog.NewJSONHandler(&out, nil)))
	sessions := scs.New()

	// Inside LoadAndSave, as root.go wires it: that is what puts the session data
	// in the context that the middleware reads the member from.
	recorder := httptest.NewRecorder()
	sessions.LoadAndSave(RequestLog(logger, sessions)(handler)).ServeHTTP(recorder, r)

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}

		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not json: %v\n%s", err, line)
		}

		records = append(records, record)
	}

	return records, recorder.Result()
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("hello"))
}

// The line that did not exist. A quiet morning and an outage produced the same
// output before this: nothing.
func TestEveryRequestLeavesALine(t *testing.T) {
	records, _ := serve(t, httptest.NewRequest(http.MethodGet, "/donate/abc", nil), ok)

	if len(records) != 1 {
		t.Fatalf("wrote %d lines, want 1", len(records))
	}

	record := records[0]

	for field, want := range map[string]any{
		"level":  "INFO",
		"method": "GET",
		"path":   "/donate/abc",
		"status": float64(http.StatusOK),
		"bytes":  float64(5),
	} {
		if record[field] != want {
			t.Errorf("%s = %v, want %v", field, record[field], want)
		}
	}

	if _, present := record["duration_ms"]; !present {
		t.Error("how long it took is half the reason to write the line")
	}
}

// A handler that writes a body without calling WriteHeader gets a 200 from
// net/http. Reporting the zero value would be a status no server ever returns.
func TestAnUnsetStatusIsReportedAsTheOneSent(t *testing.T) {
	records, _ := serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) {},
	)

	if records[0]["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", records[0]["status"])
	}
}

// The query string is the one place this app puts a failure reason in a URL --
// the auth error page carries its message there -- so paths are logged and
// queries are not.
func TestTheQueryStringIsNotLogged(t *testing.T) {
	records, _ := serve(t,
		httptest.NewRequest(http.MethodGet, "/auth/error?msg=that+email+has+not+been+approved", nil), ok)

	if records[0]["path"] != "/auth/error" {
		t.Errorf("path = %v, want the path alone", records[0]["path"])
	}
	if strings.Contains(records[0]["path"].(string), "msg=") {
		t.Error("the query string should not reach the log")
	}
}

// Turning the level down to warn should still surface failures, and a 5xx is
// worth finding without reading every line.
func TestAServerErrorIsLoggedAtError(t *testing.T) {
	records, _ := serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
	)

	if records[0]["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR for a 500", records[0]["level"])
	}

	// A refused upload or a bad id is the user being told no, not the server
	// failing, and logging those at error would make the level useless.
	records, _ = serve(t, httptest.NewRequest(http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) },
	)

	if records[0]["level"] != "INFO" {
		t.Errorf("level = %v, want INFO for a 400", records[0]["level"])
	}
}

// The point of the id. A service's own error line and the request line have to
// be findable as one thing, which is what makes the two hundred existing error
// calls worth more than they were.
func TestTheRequestIDTiesTheHandlersLinesToTheRequestLine(t *testing.T) {
	var out bytes.Buffer

	logger := slog.New(logging.WithContext(slog.NewJSONHandler(&out, nil)))
	sessions := scs.New()

	handler := func(w http.ResponseWriter, r *http.Request) {
		// What a service does today, unchanged except for carrying the context.
		logger.ErrorContext(r.Context(), "failed to read the fund")
		w.WriteHeader(http.StatusInternalServerError)
	}

	recorder := httptest.NewRecorder()
	sessions.LoadAndSave(RequestLog(logger, sessions)(handler)).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/fund", nil))

	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not json: %v", err)
		}

		id, _ := record["request_id"].(string)
		if id == "" {
			t.Fatalf("no request id on %v", record)
		}

		ids = append(ids, id)
	}

	if len(ids) != 2 {
		t.Fatalf("wrote %d lines, want the handler's and the request's", len(ids))
	}
	if ids[0] != ids[1] {
		t.Errorf("the two lines carry different ids, %q and %q", ids[0], ids[1])
	}

	// Echoed back, so a member reporting a problem can be matched to it.
	if got := recorder.Result().Header.Get(RequestHeader); got != ids[0] {
		t.Errorf("%s header = %q, want %q", RequestHeader, got, ids[0])
	}
}

// Read after the handler rather than before: a login populates the session on
// the way out, and reading first would lose the one line that says who arrived.
func TestTheMemberIsReadAfterTheHandlerRuns(t *testing.T) {
	id := uuid.New()

	records, _ := serve(t, httptest.NewRequest(http.MethodPost, "/login", nil), ok)

	// Nobody signed in: no member on the line at all, rather than an empty one.
	if _, present := records[0]["member_id"]; present {
		t.Error("an anonymous request should carry no member_id")
	}

	var out bytes.Buffer

	logger := slog.New(logging.WithContext(slog.NewJSONHandler(&out, nil)))
	sessions := scs.New()

	handler := func(w http.ResponseWriter, r *http.Request) {
		sessions.Put(r.Context(), "member", members.Member{ID: id})
	}

	sessions.LoadAndSave(RequestLog(logger, sessions)(handler)).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/login", nil))

	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &record); err != nil {
		t.Fatalf("log line is not json: %v", err)
	}

	if record["member_id"] != id.String() {
		t.Errorf("member_id = %v, want the member the request ended up as", record["member_id"])
	}

	// The id, not the name or the email: a log has different retention from the
	// database it would be copying them out of.
	for _, field := range []string{"email", "bco_name", "name"} {
		if _, present := record[field]; present {
			t.Errorf("the request line carries %q, which is a person's details", field)
		}
	}
}

// An anonymous request is the ordinary case, not an error one. The assertion on
// the session value is the two-value form, so a nil yields the zero member and
// false without panicking -- and the previous test could not tell that apart
// from a panic being caught, because a recovered panic also produces no
// member_id.
func TestAnAnonymousRequestDoesNotGoThroughRecover(t *testing.T) {
	records, _ := serve(t, httptest.NewRequest(http.MethodGet, "/", nil), ok)

	if len(records) != 1 {
		t.Fatalf("wrote %d lines, want only the request line", len(records))
	}

	if records[0]["level"] != "INFO" {
		t.Errorf("an anonymous request logged at %v", records[0]["level"])
	}
}

// The wiring mistake the recover exists for. Swallowed, it would attribute every
// request to nobody on a site full of signed-in members with nothing anywhere
// saying why.
func TestBeingWiredOutsideTheSessionMiddlewareSaysSo(t *testing.T) {
	var out bytes.Buffer

	logger := slog.New(logging.WithContext(slog.NewJSONHandler(&out, nil)))

	// No LoadAndSave around it, which is what makes scs refuse the read.
	RequestLog(logger, scs.New())(ok).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	body := out.String()

	if !strings.Contains(body, "wired outside the session middleware") {
		t.Errorf("a swallowed wiring mistake left nothing to find:\n%s", body)
	}

	// And the request line still goes out. The point of catching it here is that
	// a mistake in the logging must not take the response down with it.
	if !strings.Contains(body, `"msg":"request"`) {
		t.Errorf("the request line was lost:\n%s", body)
	}
}

// Assets are a request per page and say nothing -- served by a file server that
// touches neither the database nor the provider. They would be most of this log
// by volume.
func TestStaticAssetsAreNotLogged(t *testing.T) {
	records, _ := serve(t, httptest.NewRequest(http.MethodGet, "/static/styles.abc123.css", nil), ok)

	if len(records) != 0 {
		t.Errorf("wrote %d lines for a static asset, want none", len(records))
	}
}

// Deferred, so a handler that panics still says which request it was. The panic
// carries on to the server's own recovery.
func TestAPanickingHandlerStillLeavesALine(t *testing.T) {
	var out bytes.Buffer

	logger := slog.New(logging.WithContext(slog.NewJSONHandler(&out, nil)))

	defer func() {
		if recover() == nil {
			t.Fatal("the panic should carry on rather than be swallowed here")
		}

		if !strings.Contains(out.String(), "/admin/fund") {
			t.Errorf("a panicking request left no line:\n%s", out.String())
		}
	}()

	RequestLog(logger, nil)(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/fund", nil))
}
