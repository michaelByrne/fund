package adminweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/common"
	"boardfund/web/mux"
	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"net/http"
)

type AdminHandler struct {
	withAdmin       func(next http.HandlerFunc) http.HandlerFunc
	memberService   *members.MemberService
	donationService *donations.DonationService
	sessionManager  *scs.SessionManager
	clientID        string
}

func NewAdminHandler(
	withAdmin func(next http.HandlerFunc) http.HandlerFunc,
	memberService *members.MemberService,
	donationService *donations.DonationService,
	sessionManager *scs.SessionManager,
	clientID string,
) *AdminHandler {
	return &AdminHandler{
		withAdmin:       withAdmin,
		memberService:   memberService,
		donationService: donationService,
		sessionManager:  sessionManager,
		clientID:        clientID,
	}
}

func (h *AdminHandler) Register(r *mux.Router) {
	r.HandleFunc("/admin", h.withAdmin(h.admin))
}

func (h *AdminHandler) admin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		templ.Handler(common.ErrorMessage("unauthorized")).Component.Render(ctx, w)

		return
	}

	templ.Handler(common.Home(Admin(), common.Links(&member), h.clientID)).Component.Render(ctx, w)
}
