package authweb

import (
	"boardfund/service/auth"
	"boardfund/service/members"
	"boardfund/web/common"
	"boardfund/web/mux"
	"encoding/gob"
	"encoding/json"
	"github.com/alexedwards/scs/v2"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"net/http"
	"time"
)

type AuthHandlers struct {
	authService    *auth.AuthService
	memberService  *members.MemberService
	sessionManager *scs.SessionManager

	webAuthn *webauthn.WebAuthn

	clientID string
}

func NewAuthHandlers(authService *auth.AuthService, memberService *members.MemberService, webAuthn *webauthn.WebAuthn, sessionManager *scs.SessionManager, clientID string) *AuthHandlers {
	gob.Register(webauthn.SessionData{})

	return &AuthHandlers{
		authService:    authService,
		memberService:  memberService,
		webAuthn:       webAuthn,
		sessionManager: sessionManager,
		clientID:       clientID,
	}
}

func (h AuthHandlers) Register(r *mux.Router) {
	r.HandleFunc("GET /login", h.loginPage)
	r.HandleFunc("/logout", h.logout)
	r.HandleFunc("GET /password", h.passwordPage)
	r.HandleFunc("GET /auth/error", h.errorPage)
	r.HandleFunc("POST /auth/register", h.startRegistration)
	r.HandleFunc("PUT /auth/register", h.finishRegistration)
	r.HandleFunc("GET /auth/register", h.registrationPage)
	r.HandleFunc("GET /auth/login", h.passkeyLoginPage)
	r.HandleFunc("POST /auth/login", h.startLogin)
	r.HandleFunc("PUT /auth/login", h.finishLogin)
}

func (h AuthHandlers) passkeyLoginPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	PasskeyLogin().Render(ctx, w)
}

func (h AuthHandlers) startLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	if username == "" {
		errRedirect(w, r, "username is required", "/login")

		return
	}

	passkeyUser, err := h.authService.GetOrCreatePasskeyUser(ctx, username)
	if err != nil {
		errRedirect(w, r, err.Error(), "/auth/login")

		return
	}

	options, session, err := h.webAuthn.BeginLogin(passkeyUser)
	if err != nil {
		errRedirect(w, r, err.Error(), "/auth/login")

		return
	}

	h.sessionManager.Put(ctx, "webauthn-session", session)

	jsonResponse(w, http.StatusOK, options)
}

func (h AuthHandlers) finishLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := h.sessionManager.Get(ctx, "webauthn-session")
	if session == nil {
		http.Error(w, "session not found", http.StatusUnauthorized)

		return
	}

	sessionData, ok := session.(webauthn.SessionData)
	if !ok {
		http.Error(w, "session not found", http.StatusUnauthorized)

		return
	}

	userIDStr := string(sessionData.UserID)
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusUnauthorized)

		return
	}

	user, err := h.authService.GetPasskeyUserByID(ctx, userUUID)
	if err != nil {
		http.Error(w, "user not found", http.StatusUnauthorized)

		return
	}

	credential, err := h.webAuthn.FinishLogin(user, sessionData, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)

		return
	}

	if credential.Authenticator.CloneWarning {
		http.Error(w, "cloned authenticator", http.StatusUnauthorized)

		return
	}

	credentialBytes, err := json.Marshal(credential)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	_, err = h.authService.UpdatePasskeyUserCredentials(ctx, user.BCOName, credentialBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	member, err := h.memberService.GetMemberByUsername(ctx, user.BCOName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	h.sessionManager.Remove(ctx, "webauthn-session")
	h.sessionManager.Put(ctx, "authenticated", true)
	h.sessionManager.Put(ctx, "member", member)
}

func (h AuthHandlers) finishRegistration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := h.sessionManager.Get(ctx, "webauthn-session")
	if session == nil {
		http.Error(w, "session not found", http.StatusUnauthorized)

		return
	}

	sessionData, ok := session.(webauthn.SessionData)
	if !ok {
		http.Error(w, "session not found", http.StatusUnauthorized)

		return
	}

	userIDStr := string(sessionData.UserID)
	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusUnauthorized)

		return
	}

	user, err := h.authService.GetPasskeyUserByID(ctx, userUUID)
	if err != nil {
		http.Error(w, "user not found", http.StatusUnauthorized)

		return
	}

	credential, err := h.webAuthn.FinishRegistration(user, sessionData, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)

		return
	}

	credentialBytes, err := json.Marshal(credential)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	_, err = h.authService.UpdatePasskeyUserCredentials(ctx, user.BCOName, credentialBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	_, err = h.authService.MarkEmailAsUsed(ctx, user.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	createMember := members.CreateMember{
		BCOName: user.BCOName,
		Email:   user.Email,
	}
	_, err = h.memberService.CreateMember(ctx, createMember)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
}

func (h AuthHandlers) registrationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	Registration().Render(ctx, w)
}

func (h AuthHandlers) startRegistration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	username := r.FormValue("username")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)

		return
	}

	email := r.FormValue("email")
	if email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)

		return
	}

	err := h.authService.ValidateNewPasskeyUser(ctx, username, email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	approvedEmail, err := h.authService.GetApprovedEmail(ctx, email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	if approvedEmail.Used {
		http.Error(w, "email is already in use", http.StatusBadRequest)

		return
	}

	passkeyUser, err := h.authService.CreatePasskeyUser(ctx, username, email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	options, session, err := h.webAuthn.BeginRegistration(passkeyUser)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	h.sessionManager.Put(ctx, "webauthn-session", session)

	jsonResponse(w, http.StatusOK, options)
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

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func errRedirect(w http.ResponseWriter, r *http.Request, msg, link string) {
	http.Redirect(w, r, "/auth/error?msg="+msg+"&link="+link, http.StatusFound)
}

func isHx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
