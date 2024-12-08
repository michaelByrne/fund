package authweb

import (
	"boardfund/service/auth"
	"boardfund/web/common"
	"boardfund/web/mux"
	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"net/http"
	"time"
)

type AuthHandler struct {
	authService    *auth.AuthService
	sessionManager *scs.SessionManager

	clientID string
}

func NewAuthHandler(authService *auth.AuthService, sessionManager *scs.SessionManager, clientID string) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		sessionManager: sessionManager,
		clientID:       clientID,
	}
}

func (h AuthHandler) Register(r *mux.Router) {
	r.HandleFunc("GET /login", h.loginPage)
	r.HandleFunc("POST /login", h.login)
	r.HandleFunc("/logout", h.logout)
	r.HandleFunc("GET /password", h.passwordPage)
	r.HandleFunc("POST /password", h.resetPassword)
}

func (h AuthHandler) passwordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	templ.Handler(common.Home(Password(), common.Links(nil), h.clientID)).Component.Render(ctx, w)
}

func (h AuthHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	password := r.FormValue("old")
	newPassword := r.FormValue("new")
	confirmNew := r.FormValue("confirm")

	if username == "" || password == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("username and password are required")).Component.Render(ctx, w)

		return
	}

	if newPassword == "" || confirmNew == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("new password and confirm new password are required")).Component.Render(ctx, w)

		return
	}

	if newPassword != confirmNew {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("new password and confirm new password do not match")).Component.Render(ctx, w)

		return
	}

	member, authResp, err := h.authService.ResetPassword(ctx, username, password, newPassword)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage("failed to reset password")).Component.Render(ctx, w)

		return
	}

	if authResp.ResetPassword {
		http.Redirect(w, r, "/password", http.StatusFound)

		return
	}

	setTokenCookie("access-token", authResp.Token.IDTokenStr, authResp.Token.Expires, w)
	h.sessionManager.Put(ctx, "member", member)

	http.Redirect(w, r, "/", http.StatusFound)
}

func (h AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	setTokenCookie("access-token", "", time.Now(), w)

	http.Redirect(w, r, "/login", http.StatusFound)
}

func (h AuthHandler) loginPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if isHx(r) {
		templ.Handler(Login()).Component.Render(ctx, w)

		return
	}

	templ.Handler(common.Home(Login(), common.Links(nil), h.clientID)).Component.Render(ctx, w)
}

func (h AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("username and password are required")).Component.Render(ctx, w)

		return
	}

	member, authResp, err := h.authService.Authenticate(ctx, username, password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage("failed to authenticate")).Component.Render(ctx, w)

		return
	}

	if authResp.ResetPassword {
		http.Redirect(w, r, "/password", http.StatusFound)

		return
	}

	setTokenCookie("access-token", authResp.Token.IDTokenStr, authResp.Token.Expires, w)
	h.sessionManager.Put(ctx, "member", member)

	http.Redirect(w, r, "/", http.StatusFound)
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

func isHx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
