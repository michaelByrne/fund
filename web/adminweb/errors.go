package adminweb

import (
	"net/http"

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
