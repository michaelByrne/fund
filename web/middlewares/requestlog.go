package middlewares

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"boardfund/logging"
	"boardfund/service/members"

	"github.com/alexedwards/scs/v2"
)

// RequestHeader is where the id is echoed back, so a member reporting a problem
// can be matched to the request that caused it.
const RequestHeader = "X-Request-Id"

// RequestLog records one line per request: what was asked for, what came back,
// and how long it took.
//
// Nothing did this. The web package logged only on failure, so a quiet morning
// and an outage produced the same output -- nothing -- and there was no way to
// ask whether a request had even arrived. That is the shape of failure this app
// actually has: not a crash, but a thing that silently stops happening.
//
// It also mints the request id. Every log line written under this request
// carries it, including the two hundred-odd existing error lines, which is what
// turns them from separate events into an account of one request.
//
// Must be registered inside scs's LoadAndSave, which is where the session it
// reads the member from is put into the context. sessions may be nil, which
// simply leaves the member off.
func RequestLog(logger *slog.Logger, sessions *scs.SessionManager) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Assets are a request per page and say nothing: they are served by a
			// file server that does not touch the database or the provider, and
			// they would be most of this log by volume.
			if strings.HasPrefix(r.URL.Path, "/static/") {
				next(w, r)

				return
			}

			id := logging.NewRequestID()
			r = r.WithContext(logging.WithRequestID(r.Context(), id))

			w.Header().Set(RequestHeader, id)

			recorder := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
			started := time.Now()

			// Deferred, so a handler that panics still leaves a line saying which
			// request it was. The panic carries on to the server's own recovery.
			defer func() {
				logRequest(logger, sessions, r, recorder, started)
			}()

			next(recorder, r)
		}
	}
}

func logRequest(
	logger *slog.Logger,
	sessions *scs.SessionManager,
	r *http.Request,
	recorder *recordingWriter,
	started time.Time,
) {
	attrs := []any{
		slog.String("method", r.Method),
		// Path only. The query string carries the message shown on the auth error
		// page, which is the one place a failure reason is put in a URL.
		slog.String("path", r.URL.Path),
		slog.Int("status", recorder.status),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		slog.Int("bytes", recorder.bytes),
	}

	// Read after the handler, not before: a login populates the session on the
	// way out, and attributing that request to nobody would lose the one line
	// that says who arrived.
	if member, ok := sessionMember(logger, sessions, r); ok {
		// The id, not the name or the email. A log is a place names end up
		// duplicated into somewhere with different retention than the database.
		attrs = append(attrs, slog.String("member_id", member.ID.String()))
	}

	// A 5xx is logged at error as well as being counted, so that turning the
	// level down to warn still surfaces them. The handler's own error line says
	// why; this one says which request, and how long it took to get there.
	if recorder.status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", attrs...)

		return
	}

	logger.InfoContext(r.Context(), "request", attrs...)
}

// sessionMember reads who the request turned out to belong to.
//
// The assertion is the two-value form, so an anonymous request -- where Get
// returns nil -- yields the zero member and false. That is the ordinary path and
// it does not go anywhere near the recover below.
//
// The recover is for the one thing that does panic: scs refuses to read session
// data from a context that never went through LoadAndSave. That is a wiring
// mistake, and this runs in a deferred call while the response is being
// finished, where an unrecovered panic would bury whatever real error was being
// reported under an obscure one from the logger.
//
// Caught and then said out loud. Swallowing it would leave every request
// attributed to nobody, on a page full of signed-in members, with nothing
// anywhere explaining why -- which is the same shape of silent wrong this
// middleware exists to remove.
func sessionMember(
	logger *slog.Logger,
	sessions *scs.SessionManager,
	r *http.Request,
) (member members.Member, found bool) {
	if sessions == nil {
		return members.Member{}, false
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			logger.ErrorContext(r.Context(), "request logging is wired outside the session middleware",
				slog.Any("panic", recovered),
			)

			member, found = members.Member{}, false
		}
	}()

	member, found = sessions.Get(r.Context(), "member").(members.Member)

	return member, found
}

// recordingWriter remembers what the handler sent.
//
// status is seeded with 200 because that is what net/http sends for a handler
// that writes a body without calling WriteHeader, and for one that writes
// nothing at all. A zero here would be reported as a status no server ever
// returns.
type recordingWriter struct {
	http.ResponseWriter

	status int
	bytes  int
}

func (w *recordingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n

	return n, err
}

// Unwrap lets http.ResponseController reach the real writer, so wrapping does
// not quietly remove flushing or hijacking from anything downstream.
func (w *recordingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
