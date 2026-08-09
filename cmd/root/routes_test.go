package root

import (
	"net/http"
	"testing"

	"boardfund/web/adminweb"
	"boardfund/web/authweb"
	"boardfund/web/homeweb"
	"boardfund/web/hooksweb"
	"boardfund/web/mux"
)

// Every handler group registers into one router in run(), so a pattern in
// adminweb can conflict with one in homeweb and no per-package test would see
// it. This registers all four against a real ServeMux, which is the same check
// the process performs at boot -- only somewhere that failing costs a test run
// rather than the site.
//
// It exists because a conflicting admin pattern shipped, panicked on startup and
// took production down: registration happens inside run(), so nothing had ever
// called it outside of a live boot.
func TestAllRoutesRegisterWithoutConflict(t *testing.T) {
	// Register only takes method values off each handler, so the services behind
	// them are never dereferenced here and can stay nil. The middleware arguments
	// are the exception -- they are called during registration, not per request.
	passthrough := func(next http.HandlerFunc) http.HandlerFunc { return next }

	authHandlers := authweb.NewAuthHandlers(nil, nil, nil, "", true)
	donationHandlers := homeweb.NewFundHandlers(nil, nil, passthrough, nil, "", "")
	adminHandlers := adminweb.NewAdminHandlers(
		passthrough, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "",
	)
	webhooksHandlers := hooksweb.NewWebhooksHandlers(nil, nil, nil, nil, nil, "")

	router := mux.NewRouter(http.NewServeMux())

	// Mirrors run(): the static handler is registered before the groups.
	router.HandleFunc("/static/", func(http.ResponseWriter, *http.Request) {})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("routes conflict: %v", r)
		}
	}()

	authHandlers.Register(router)
	donationHandlers.Register(router)
	adminHandlers.Register(router)
	webhooksHandlers.Register(router)
}
