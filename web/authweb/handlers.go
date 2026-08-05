package authweb

import (
	"boardfund/service/auth"
	"boardfund/service/members"
	"boardfund/web/common"
	"boardfund/web/mux"
	"errors"
	"github.com/alexedwards/scs/v2"
	"net/http"
	"net/url"
	"time"
)

type AuthHandlers struct {
	authService    *auth.AuthService
	memberService  *members.MemberService
	sessionManager *scs.SessionManager

	clientID string
}

func NewAuthHandlers(authService *auth.AuthService, memberService *members.MemberService, sessionManager *scs.SessionManager, clientID string) *AuthHandlers {

	return &AuthHandlers{
		authService:    authService,
		memberService:  memberService,
		sessionManager: sessionManager,
		clientID:       clientID,
	}
}

func (h AuthHandlers) Register(r *mux.Router) {
	r.HandleFunc("GET /login", h.loginPage)
	r.HandleFunc("POST /login", h.login)
	r.HandleFunc("GET /register", h.passwordRegistrationPage)
	r.HandleFunc("POST /register", h.register)
	r.HandleFunc("/logout", h.logout)
	r.HandleFunc("GET /password", h.passwordPage)
	r.HandleFunc("GET /auth/error", h.errorPage)
	r.HandleFunc("POST /password", h.resetPassword)
}

func (h AuthHandlers) register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	email := r.FormValue("email")

	approvedEmail, err := h.authService.GetApprovedEmail(ctx, email)
	if err != nil {
		// Being turned away is not a server fault. Answering 500 here made a routine
		// rejection look like an outage, both to the user and in the logs.
		if errors.Is(err, auth.ErrEmailNotApproved) {
			errRedirect(w, r, "that email has not been approved for registration. ask an admin to add it.", "/register")

			return
		}

		errRedirect(w, r, "could not check that email. please try again.", "/register")

		return
	}

	if approvedEmail.Used {
		errRedirect(w, r, "an account has already been registered with that email.", "/register")

		return
	}

	_, err = h.authService.Register(ctx, username, email)
	if err != nil {
		errRedirect(w, r, err.Error(), "/register")

		return
	}

	_, err = h.authService.MarkEmailAsUsed(ctx, email)
	if err != nil {
		errRedirect(w, r, err.Error(), "/register")

		return
	}

	RegistrationSuccess().Render(ctx, w)
}

func (h AuthHandlers) passwordRegistrationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	PasswordRegistration().Render(ctx, w)
}

func (h AuthHandlers) resetPassword(w http.ResponseWriter, r *http.Request) {
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

func (h AuthHandlers) login(w http.ResponseWriter, r *http.Request) {
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

func (h AuthHandlers) errorPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	errStr := r.URL.Query().Get("msg")
	link := r.URL.Query().Get("link")

	common.ErrorMessage(nil, errStr, link, r.URL.Path).Render(ctx, w)
}

func (h AuthHandlers) passwordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	Password().Render(ctx, w)
}

func (h AuthHandlers) logout(w http.ResponseWriter, r *http.Request) {
	setTokenCookie("access-token", "", time.Now(), w)

	http.Redirect(w, r, "/login", http.StatusFound)
}

func (h AuthHandlers) loginPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if isHx(r) {
		w.Header().Set("HX-Redirect", "/login")
		Login().Render(ctx, w)

		return
	}

	Login().Render(ctx, w)
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

// errRedirect sends the user to the error page with a message to display.
//
// Both values are escaped: they were concatenated raw, so any message containing
// an ampersand truncated itself into a second query parameter, and one containing
// a space produced a malformed URL.
func errRedirect(w http.ResponseWriter, r *http.Request, msg, link string) {
	target := url.URL{Path: "/auth/error"}

	query := target.Query()
	query.Set("msg", msg)
	query.Set("link", link)
	target.RawQuery = query.Encode()

	http.Redirect(w, r, target.String(), http.StatusFound)
}

func isHx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
