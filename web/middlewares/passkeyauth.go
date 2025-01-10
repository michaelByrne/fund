package middlewares

import (
	"boardfund/service/members"
	"github.com/alexedwards/scs/v2"
	"net/http"
)

func PasskeyVerify(sessionManager *scs.SessionManager) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		hfn := func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/static" {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			authenticated := sessionManager.GetBool(ctx, "authenticated")
			if !authenticated {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return hfn
	}
}

func PasskeyVerifyAdmin(sessionManager *scs.SessionManager) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		hfn := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			rawMember := sessionManager.Get(ctx, "member")
			member, ok := rawMember.(members.Member)
			if !ok {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			if !member.IsAdmin() {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return hfn
	}
}
