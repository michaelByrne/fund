package common

import "net/http"

// IsHTMX reports whether the request came from htmx rather than a plain browser
// navigation.
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// Redirect sends a browser to target from a handler that may have been reached
// either by htmx or by a plain navigation.
//
// A bare 3xx is wrong for the htmx case, and wrong in a way that looks like a
// feature: the XHR follows the redirect transparently, so htmx never learns one
// happened and swaps the destination page into whatever element was clicked. A
// session that expired mid-click renders the home page inside a table row.
// HX-Redirect is the header htmx acts on, and it needs a 2xx to be seen at all.
func Redirect(w http.ResponseWriter, r *http.Request, target string) {
	if IsHTMX(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)

		return
	}

	http.Redirect(w, r, target, http.StatusSeeOther)
}
