package authweb

import (
	"boardfund/service/auth"
	"boardfund/web/common"
	"github.com/a-h/templ"
	"net/http"
	"time"
)

type AuthHandler struct {
	authService *auth.AuthService

	clientID string
}

func NewAuthHandler(authService *auth.AuthService, clientID string) *AuthHandler {
	return &AuthHandler{authService: authService, clientID: clientID}
}

func (h AuthHandler) Register(r *http.ServeMux) {
	r.HandleFunc("/login", h.login)
	//r.HandleFunc("/logout", h.logout)
}

func (h AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method == http.MethodGet {
		templ.Handler(common.Home(Login(), h.clientID)).Component.Render(ctx, w)

		return
	} else if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			w.WriteHeader(http.StatusBadRequest)
			templ.Handler(common.ErrorMessage("username and password are required")).Component.Render(ctx, w)

			return
		}

		_, token, err := h.authService.Authenticate(ctx, username, password)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			templ.Handler(common.ErrorMessage("failed to authenticate")).Component.Render(ctx, w)

			return
		}

		setTokenCookie("access-token", token.TokenStr, token.Expires, w)

		http.Redirect(w, r, "/", http.StatusFound)
	}

}

func setTokenCookie(name, token string, expiration time.Time, w http.ResponseWriter) {
	cookie := new(http.Cookie)
	cookie.Name = name
	cookie.Value = token
	cookie.Expires = expiration
	cookie.Path = "/"
	cookie.HttpOnly = true

	http.SetCookie(w, cookie)
}
