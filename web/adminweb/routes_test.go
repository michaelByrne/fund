package adminweb

import (
	"net/http"
	"testing"

	"boardfund/web/mux"
)

// Routes are registered at startup, inside run(), so nothing before this test
// ever exercised them: a conflicting pattern compiled, passed every test, built
// a release, and then panicked in the container as it booted.
//
// ServeMux rejects ambiguous patterns at registration, which is the check we
// want -- it just needed to happen somewhere a test could see it. Registering
// against a real ServeMux is the whole test.
func TestAdminRoutesRegisterWithoutConflict(t *testing.T) {
	// Register only takes method values off the handler and wraps them, so the
	// services can stay nil. withAdmin cannot: it is called during registration.
	h := &AdminHandlers{
		withAdmin: func(next http.HandlerFunc) http.HandlerFunc { return next },
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("admin routes conflict: %v", r)
		}
	}()

	h.Register(mux.NewRouter(http.NewServeMux()))
}
