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
	r.HandleFunc("GET /auth/error", h.errorPage)
}

func (h AuthHandler) errorPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	errStr := r.URL.Query().Get("msg")
	link := r.URL.Query().Get("link")

	common.ErrorMessage(nil, errStr, link, r.URL.Path).Render(ctx, w)
}

func (h AuthHandler) passwordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	Password().Render(ctx, w)
}

func (h AuthHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	password := r.FormValue("old")
	newPassword := r.FormValue("new")
	confirmNew := r.FormValue("confirm")

	if newPassword != confirmNew {
		errRedirect(w, r, "passwords do not match", "/password")

		return
	}

	member, authResp, err := h.authService.ResetPassword(ctx, username, password, newPassword)
	if err != nil {
		errRedirect(w, r, err.Error(), "/password")

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

	Login().Render(ctx, w)
}

func (h AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	password := r.FormValue("password")

	member, authResp, err := h.authService.Authenticate(ctx, username, password)
	if err != nil {
		errRedirect(w, r, err.Error(), "/login")

		return
	}

	if authResp.ResetPassword {
		http.Redirect(w, r, "/password", http.StatusFound)

		return
	}

	if !member.Active {
		http.Redirect(w, r, "/", http.StatusFound)

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

func errRedirect(w http.ResponseWriter, r *http.Request, msg, link string) {
	http.Redirect(w, r, "/auth/error?msg="+msg+"&link="+link, http.StatusFound)
}

func isHx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
