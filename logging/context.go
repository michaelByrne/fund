package logging

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

type requestIDKey struct{}

// NewRequestID mints an id for one request.
//
// Always ours, never the caller's. An inbound X-Request-Id would be more useful
// for tracing across Cloudflare and is also a string an anonymous client
// chooses, which then appears verbatim in a log line -- unbounded in length and
// able to carry whatever a JSON string can carry. Correlating across the edge is
// worth having and is worth doing deliberately, with validation, rather than by
// trusting the header.
func NewRequestID() string {
	return uuid.NewString()
}

// WithRequestID puts the id where the handler below can find it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom reads it back, empty when there is none: a cron job, a webhook
// consumer, a test.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)

	return id
}

// WithContext wraps a handler so every record carries the request id from the
// context it was logged with.
//
// The point is the lines nobody has to change. There are well over two hundred
// existing logger.Error calls, and each one becomes traceable to the request
// that caused it without its call site being touched -- provided the call passes
// a context, which is what ErrorContext and friends are for.
//
// A call that does not pass a context still works and simply carries no id.
// That is the honest outcome: slog's context-free methods use
// context.Background(), so there is nothing to read.
func WithContext(handler slog.Handler) slog.Handler {
	return contextHandler{Handler: handler}
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if id := RequestIDFrom(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}

	return h.Handler.Handle(ctx, record)
}

// WithAttrs and WithGroup rewrap.
//
// Without them the embedded handler's versions are promoted, and they return the
// inner handler -- so logger.With("fund_id", id) would quietly hand back a
// logger that had lost the request id, on exactly the derived loggers most
// worth tracing.
func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup nests the request id inside the group, which is a wart worth knowing
// about rather than discovering.
//
// Handle adds the id as a record attribute, and a handler with an open group
// puts record attributes inside it -- so a logger built with WithGroup("payout")
// writes payout.request_id rather than a top-level one, and a search across
// services stops matching. There is no fix from inside a wrapper: placing an
// attribute outside every group means adding it before the group was opened,
// and the id is not known until the request arrives.
//
// Nothing in this codebase opens a group. The behaviour is pinned by a test so
// that the first thing that does will find it stated rather than have to work it
// out from a log line that nearly matches.
func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}
