package adminweb

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"boardfund/service/members"
)

// Messages shown to operators. Deliberately not the underlying error: an admin can
// act on "that fund no longer exists", not on a pgx wrapping chain, and the real
// error is already in the logs.
const (
	msgInternal    = "something went wrong. try again, and check the logs if it persists."
	msgBadRequest  = "that request was not valid."
	msgNotFound    = "not found. it may have been removed since this page loaded."
	msgUnavailable = "could not load this page."
)

// maxFormBytes bounds a urlencoded admin form.
//
// Generous for a handful of text fields and nowhere near the eight megabytes
// these were reading when they were mistaken for uploads. The forms that really
// do carry a picture bound themselves against donations.MaxImageBytes.
const maxFormBytes = 64 << 10

func isHx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// renderError reports a failed admin action.
//
// htmx discards 4xx/5xx bodies by default, so the admin layout carries an
// inheritable hx-target-error pointing at #admin-error; this writes the fragment
// that lands there. Returning a bare status instead -- which is what every admin
// handler used to do -- meant a failed action looked exactly like a successful one.
func (h *AdminHandlers) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	ctx := r.Context()

	h.logRefusal(r, status, message)

	if isHx(r) {
		w.WriteHeader(status)
		AdminError(message).Render(ctx, w)

		return
	}

	// A full page load has no swap target, so render the whole shell around it.
	member, _ := h.sessionManager.Get(ctx, "member").(members.Member)

	w.WriteHeader(status)
	AdminErrorPage(message, &member, r.URL.Path).Render(ctx, w)
}

// logRefusal says why a request was turned down, and where.
//
// The request line records that a POST to /admin/enrollment answered 400. It
// cannot say which of that handler's seven checks fired, and the operator
// message is the same generic sentence for all of them -- so a refusal in
// production was a dead end. Enrollment creation had been broken for every
// attempt and the logs showed a tidy, unremarkable 400.
//
// The source position is what makes this worth having. It names the line that
// decided, for all twenty-eight refusal sites at once, without a reason having
// to be threaded through each of them and kept accurate.
func (h *AdminHandlers) logRefusal(r *http.Request, status int, message string) {
	if h.logger == nil {
		return
	}

	attrs := []any{
		slog.Int("status", status),
		slog.String("reason", message),
	}

	if at := refusedAt(); at != "" {
		attrs = append(attrs, slog.String("refused_at", at))
	}

	// A 4xx is a decision, not a fault: the handler looked at the request and
	// said no, which is the system working. A 5xx is the other thing.
	if status >= http.StatusInternalServerError {
		h.logger.ErrorContext(r.Context(), "request could not be served", attrs...)

		return
	}

	h.logger.InfoContext(r.Context(), "request refused", attrs...)
}

// refusedAt is the handler line that decided, skipping the error helpers
// themselves.
//
// Walked rather than counted. A fixed skip depth is right for badRequest and
// wrong for a handler that calls renderError directly, and payouts.go does
// exactly that -- so the frame to report is the first one that is not part of
// this file's plumbing.
func refusedAt() string {
	var pcs [8]uintptr

	// 2 skips runtime.Callers and refusedAt itself.
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs[:])])

	for {
		frame, more := frames.Next()

		if !isErrorPlumbing(frame.Function) {
			return fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
		}

		if !more {
			return ""
		}
	}
}

func isErrorPlumbing(function string) bool {
	for _, helper := range []string{
		".logRefusal", ".renderError", ".badRequest", ".internalError", ".notFound",
	} {
		if strings.HasSuffix(function, helper) {
			return true
		}
	}

	return false
}

func (h *AdminHandlers) internalError(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusInternalServerError, msgInternal)
}

func (h *AdminHandlers) badRequest(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = msgBadRequest
	}

	h.renderError(w, r, http.StatusBadRequest, message)
}

func (h *AdminHandlers) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusNotFound, msgNotFound)
}
