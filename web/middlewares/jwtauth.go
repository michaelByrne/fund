package middlewares

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwt"
)

func Verify(verifyFunc func(tokenStr string) (jwt.Token, error), findTokenFns ...func(r *http.Request) string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		hfn := func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/static" {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			_, err := VerifyRequest(r, verifyFunc, findTokenFns...)
			if err != nil {
				w.Header().Set("HX-Redirect", "/login")
				http.Redirect(w, r, "/login", http.StatusFound)

				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return hfn
	}
}

func VerifyRequest(r *http.Request, verifyFunc func(tokenStr string) (jwt.Token, error), findTokenFns ...func(r *http.Request) string) (jwt.Token, error) {
	var tokenString string

	// Extract token string from the request by calling token find functions in
	// the order they where provided. Further extraction stops if a function
	// returns a non-empty string.
	for _, fn := range findTokenFns {
		tokenString = fn(r)
		if tokenString != "" {
			break
		}
	}
	if tokenString == "" {
		return nil, fmt.Errorf("no token found")
	}

	return verifyFunc(tokenString)
}

// TokenFromCookie tries to retrieve the token string from a cookie named
// "jwt".
func TokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie("access-token")
	if err != nil {
		return ""
	}

	return cookie.Value
}

// TokenFromHeader tries to retreive the token string from the
// "Authorization" reqeust header: "Authorization: BEARER T".
func TokenFromHeader(r *http.Request) string {
	// Get token from authorization header.
	bearer := r.Header.Get("Authorization")
	if len(bearer) > 7 && strings.ToUpper(bearer[0:6]) == "BEARER" {
		return bearer[7:]
	}
	return ""
}
