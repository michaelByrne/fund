package adminweb

import (
	"boardfund/messaging"
	"boardfund/service/adminevents"
	"boardfund/service/auth"
	"boardfund/service/donations"
	"boardfund/service/enrollments"
	"boardfund/service/finance"
	"boardfund/service/fundevents"
	"boardfund/service/members"
	"boardfund/service/payouts"
	"boardfund/web/common"
	"boardfund/web/mux"
	"context"
	"errors"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"net/http"
	"strconv"
	"time"
)

// webhookBus is the durable bus, read-only. Narrow on purpose: the admin page
// inspects delivery, it does not drive it.
type webhookBus interface {
	Status(ctx context.Context) (messaging.Status, error)
}

type AdminHandlers struct {
	withAdmin         func(next http.HandlerFunc) http.HandlerFunc
	memberService     *members.MemberService
	donationService   *donations.DonationService
	enrollmentService *enrollments.EnrollmentsService
	authService       *auth.AuthService
	financeService    *finance.FinanceService
	payoutService     *payouts.PayoutService
	fundEventsService *fundevents.Service
	adminEvents       *adminevents.Service
	sessionManager    *scs.SessionManager
	webhookBus        webhookBus
	clientID          string
}

func NewAdminHandlers(
	withAdmin func(next http.HandlerFunc) http.HandlerFunc,
	memberService *members.MemberService,
	donationService *donations.DonationService,
	authService *auth.AuthService,
	financeService *finance.FinanceService,
	enrollmentsService *enrollments.EnrollmentsService,
	payoutService *payouts.PayoutService,
	fundEventsService *fundevents.Service,
	adminEvents *adminevents.Service,
	sessionManager *scs.SessionManager,
	webhookBus webhookBus,
	clientID string,
) *AdminHandlers {
	return &AdminHandlers{
		withAdmin:         withAdmin,
		memberService:     memberService,
		donationService:   donationService,
		authService:       authService,
		financeService:    financeService,
		enrollmentService: enrollmentsService,
		payoutService:     payoutService,
		fundEventsService: fundEventsService,
		adminEvents:       adminEvents,
		sessionManager:    sessionManager,
		webhookBus:        webhookBus,
		clientID:          clientID,
	}
}

func (h *AdminHandlers) Register(r *mux.Router) {
	r.HandleFunc("/admin", h.withAdmin(h.adminPage))
	r.HandleFunc("GET /admin/funds", h.withAdmin(h.fundsPage))
	r.HandleFunc("POST /admin/fund", h.withAdmin(h.createFund))
	r.HandleFunc("POST /admin/fund/deactivate/{id}", h.withAdmin(h.deactivateFund))
	r.HandleFunc("POST /admin/member/deactivate/{id}", h.withAdmin(h.deactivateMember))
	r.HandleFunc("GET /admin/member/{id}", h.withAdmin(h.memberPage))
	// verb/{id}, like deactivate above, and not /member/{id}/admin: a wildcard in
	// the third segment overlaps every literal one, and ServeMux rejects the
	// ambiguity by panicking as it registers.
	r.HandleFunc("POST /admin/member/promote/{id}", h.withAdmin(h.grantAdmin))
	r.HandleFunc("POST /admin/member/demote/{id}", h.withAdmin(h.revokeAdmin))
	r.HandleFunc("GET /admin/fund/audit", h.withAdmin(h.fundAudit))
	r.HandleFunc("GET /admin/fund", h.withAdmin(h.fundPage))
	r.HandleFunc("POST /admin/note/remove/{id}", h.withAdmin(h.removeFundNote))
	// Verb-first, like deactivate/{id} beside it. "/admin/fund/{id}/image" reads
	// better and cannot be registered: it collides with "/admin/fund/deactivate/{id}",
	// because {id} matches "deactivate" just as happily as a uuid. The route
	// conflict test caught it, which is what that test exists for.
	r.HandleFunc("POST /admin/fund/details/{id}", h.withAdmin(h.saveFundDetails))
	r.HandleFunc("POST /admin/fund/image/{id}", h.withAdmin(h.setFundImage))
	r.HandleFunc("POST /admin/fund/image/remove/{id}", h.withAdmin(h.removeFundImage))
	r.HandleFunc("GET /admin/members/search", h.withAdmin(h.searchMembers))
	r.HandleFunc("POST /admin/enrollment", h.withAdmin(h.createEnrollment))
	r.HandleFunc("GET /admin/enrollment/confirm", h.withAdmin(h.confirmEnrollment))
	r.HandleFunc("POST /admin/enrollment/cancel/{id}", h.withAdmin(h.deactivateEnrollment))
	r.HandleFunc("GET /admin/payouts", h.withAdmin(h.payoutsPage))
	r.HandleFunc("GET /admin/webhooks", h.withAdmin(h.webhooksPage))
	r.HandleFunc("GET /admin/audit", h.withAdmin(h.auditPage))
	r.HandleFunc("GET /admin/payout/{id}", h.withAdmin(h.payoutPage))
	r.HandleFunc("POST /admin/payout/approve/{id}", h.withAdmin(h.approvePayout))
	r.HandleFunc("POST /admin/payout/reject/{id}", h.withAdmin(h.rejectPayout))
	r.HandleFunc("DELETE /admin/approved/{email}", h.withAdmin(h.deleteApprovedEmail))
	r.HandleFunc("POST /admin/approved", h.withAdmin(h.addApprovedEmail))
}

func (h *AdminHandlers) deactivateEnrollment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.badRequest(w, r, "")

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	_, err = h.enrollmentService.DeactivateEnrollment(ctx, idUUID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	w.Header().Set("HX-Trigger", "enrollmentDeactivated")
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandlers) addApprovedEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	// Multipart now, because the form carries a picture. The body is bounded
	// before it is read: a create form is not a reason to accept an unbounded
	// upload.
	r.Body = http.MaxBytesReader(w, r.Body, donations.MaxImageBytes+8192)

	err := r.ParseMultipartForm(donations.MaxImageBytes + 8192)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	email := r.FormValue("email")
	if email == "" {
		h.badRequest(w, r, "")

		return
	}

	_, err = h.authService.InsertApprovedEmail(ctx, email)
	if err != nil {
		if errors.Is(err, auth.ErrEmailAlreadyApproved) {
			h.badRequest(w, r, "that email is already approved.")

			return
		}

		h.internalError(w, r)

		return
	}

	approvedEmails, err := h.authService.GetApprovedEmails(ctx)
	if err != nil {
		h.internalError(w, r)

		return
	}

	ApprovedEmails(approvedEmails).Render(ctx, w)
}

func (h *AdminHandlers) deleteApprovedEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	email := r.PathValue("email")
	if email == "" {
		h.badRequest(w, r, "")

		return
	}

	_, err := h.authService.DeleteApprovedEmail(ctx, email)
	if err != nil {
		h.internalError(w, r)

		return
	}

	approvedEmails, err := h.authService.GetApprovedEmails(ctx)
	if err != nil {
		h.internalError(w, r)

		return
	}

	EmailList(approvedEmails).Render(ctx, w)
}

func (h *AdminHandlers) confirmEnrollment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	memberIDStr := r.URL.Query().Get("member")
	if memberIDStr == "" {
		h.badRequest(w, r, "")

		return
	}

	memberUUID, err := uuid.Parse(memberIDStr)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	fundIDStr := r.URL.Query().Get("fund")
	if fundIDStr == "" {
		h.badRequest(w, r, "")

		return
	}

	fundUUID, err := uuid.Parse(fundIDStr)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	member, err := h.memberService.GetMemberByID(ctx, memberUUID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	enrollment, err := h.enrollmentService.FundEnrollmentExists(ctx, fundUUID, memberUUID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	if enrollment {
		EnrollmentExistsErr(*member, *fund).Render(ctx, w)

		return
	}

	ConfirmEnrollment(*fund, *member).Render(ctx, w)
}

func (h *AdminHandlers) createEnrollment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	// Multipart now, because the form carries a picture. The body is bounded
	// before it is read: a create form is not a reason to accept an unbounded
	// upload.
	r.Body = http.MaxBytesReader(w, r.Body, donations.MaxImageBytes+8192)

	err := r.ParseMultipartForm(donations.MaxImageBytes + 8192)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	fundIDStr := r.FormValue("fund")
	if fundIDStr == "" {
		h.badRequest(w, r, "")

		return
	}

	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	memberIDStr := r.FormValue("member")
	if memberIDStr == "" {
		h.badRequest(w, r, "")

		return
	}

	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	paypalEmail := r.FormValue("paypal")
	if paypalEmail == "" {
		h.badRequest(w, r, "")

		return
	}

	username := r.FormValue("username")
	if username == "" {
		h.badRequest(w, r, "")

		return
	}

	createEnrollment := enrollments.CreateEnrollment{
		FundID:        fundID,
		MemberID:      memberID,
		PaypalEmail:   paypalEmail,
		MemberBCOName: username,
	}

	enrollment, err := h.enrollmentService.CreateEnrollment(ctx, createEnrollment)
	if err != nil {
		// A malformed address is something the admin can fix, and the only person
		// who can. Reported as internal error it reads as a fault here, and the
		// obvious response is to try the same thing again.
		if errors.Is(err, enrollments.ErrInvalidPaypalEmail) {
			h.badRequest(w, r, err.Error())

			return
		}

		h.internalError(w, r)

		return
	}

	member, err := h.memberService.GetMemberByID(ctx, memberID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	EnrollmentSuccess(*enrollment, *member).Render(ctx, w)
}

func (h *AdminHandlers) searchMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query().Get("member_search")

	membersByUsername, err := h.memberService.SearchMembersByUsername(ctx, query)
	if err != nil {
		h.internalError(w, r)

		return
	}

	MemberSearchResults(membersByUsername).Render(ctx, w)
}

func (h *AdminHandlers) fundPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(r.Context(), "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	fundIDStr := r.URL.Query().Get("fund")
	if fundIDStr == "" {
		h.badRequest(w, r, "")

		return
	}

	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	activeEnrollments, err := h.enrollmentService.GetActiveEnrollmentsForFund(ctx, fundID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	// A failed history read must not take the page down with it: the enrollments
	// and fund status above are the reason someone opened this.
	events, err := h.fundEventsService.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
	if err != nil {
		events = nil
	}

	// Not fatal either: the picture is the least of what this page is for.
	image, err := h.donationService.GetFundImage(ctx, fundID)
	if err != nil {
		image = nil
	}

	w.Header().Add("HX-Redirect", r.URL.String())
	Enrollments(*fund, activeEnrollments, events, image, &member, r.URL.Path).Render(ctx, w)
}

// removeFundNote takes a donor's note down.
//
// The first thing this application publishes that a member wrote, so somebody has
// to be able to remove one. Soft: the row keeps who wrote it, who removed it and
// when, which after taking something down is exactly what you want to still have.
func (h *AdminHandlers) removeFundNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/")

		return
	}

	noteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	if err = h.donationService.RemoveFundNote(ctx, noteID, member.ID); err != nil {
		h.internalError(w, r)

		return
	}

	// The row is gone from the list, so nothing replaces it.
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandlers) fundAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	// One live view per fund, so there is no date or report type to choose. There
	// used to be: the page read a CSV written by a past reconciliation run, so it
	// could only show a fund on the days a run had left a file.
	fundID := r.URL.Query().Get("fund")
	if fundID == "" {
		h.badRequest(w, r, "")

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	req := finance.GetAuditRequest{FundID: fundUUID}

	audit, err := h.financeService.GetAudit(ctx, req)
	if err != nil {
		h.internalError(w, r)

		return
	}

	w.Header().Set("HX-Redirect", r.URL.String())
	FundPaymentsAudit(*audit, &member, r.URL.Path).Render(ctx, w)
}

// AdminAccessState is what the member page knows about a member's admin rights.
// Unknown is separate from IsAdmin because the answer lives in Cognito: when that
// lookup fails, "not an admin" is a guess, and one that offers a button undoing
// whatever is actually true.
type AdminAccessState struct {
	IsAdmin bool
	IsSelf  bool
	Changed bool
	Unknown bool
}

// adminAccess reads the viewed member's rights from Cognito. A failure is
// reported as unknown rather than returned: the rest of the member page --
// donations, contributions, dates -- does not depend on Cognito, and taking the
// whole page down when the provider is slow is how a blip becomes an outage.
func (h *AdminHandlers) adminAccess(ctx context.Context, viewed, actor members.Member, changed bool) AdminAccessState {
	state := AdminAccessState{
		IsSelf:  viewed.ID == actor.ID,
		Changed: changed,
	}

	isAdmin, err := h.authService.IsAdmin(ctx, viewed.BCOName)
	if err != nil {
		state.Unknown = true

		return state
	}

	state.IsAdmin = isAdmin

	return state
}

// setAdmin backs both the promote and the revoke route. It re-reads the rights
// from Cognito afterwards rather than assuming the write took, so the control
// that comes back reflects the provider rather than the request.
func (h *AdminHandlers) setAdmin(w http.ResponseWriter, r *http.Request, grant bool) {
	ctx := r.Context()

	actor, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		// Only ever reached by hx-post/hx-delete from the toggle, so a bare 302
		// would be followed by the XHR and put the home page inside the admin
		// access row. A session can expire while the token behind withAdmin is
		// still valid, which is exactly when this fires.
		common.Redirect(w, r, "/")

		return
	}

	idUUID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	viewed, err := h.memberService.GetMemberByID(ctx, idUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&actor, "failed to get member", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	// One admin revoking their own access locks everyone out, and the admin
	// section is the only place this can be undone. The button is hidden for the
	// same case, but the route is reachable without it.
	if !grant && viewed.ID == actor.ID {
		w.WriteHeader(http.StatusConflict)
		common.ErrorMessage(&actor, "you cannot revoke your own admin access", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	if grant {
		err = h.authService.GrantAdmin(ctx, actor, *viewed)
	} else {
		err = h.authService.RevokeAdmin(ctx, actor, *viewed)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&actor, "failed to change admin access", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	AdminAccess(*viewed, h.adminAccess(ctx, *viewed, actor, true)).Render(ctx, w)
}

func (h *AdminHandlers) grantAdmin(w http.ResponseWriter, r *http.Request) {
	h.setAdmin(w, r, true)
}

func (h *AdminHandlers) revokeAdmin(w http.ResponseWriter, r *http.Request) {
	h.setAdmin(w, r, false)
}

// webhooksPage is the only window onto the durable bus. The embedded NATS server
// opens no client port, so nothing outside this process can query it.
func (h *AdminHandlers) webhooksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/")

		return
	}

	status, err := h.webhookBus.Status(ctx)
	if err != nil {
		h.internalError(w, r)

		return
	}

	Webhooks(status, &member, r.URL.Path).Render(ctx, w)
}

// auditPage shows every recorded change to admin access.
//
// Guarded by withAdmin like the rest of the section. That is the right level:
// the log names who holds the keys and who handed them over, which is a
// governance question for the people who already have them, not a public one.
func (h *AdminHandlers) auditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/")

		return
	}

	events, err := h.adminEvents.GetAdminEvents(ctx, adminevents.DefaultLimit)
	if err != nil {
		h.internalError(w, r)

		return
	}

	AdminAudit(events, &member, r.URL.Path).Render(ctx, w)
}

func (h *AdminHandlers) memberPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.badRequest(w, r, "")

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	memberDetails, err := h.memberService.GetMemberWithDonations(ctx, idUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, "failed to get member", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	w.Header().Set("HX-Redirect", r.URL.Path)
	Member(*memberDetails, &member, r.URL.Path, h.adminAccess(ctx, *memberDetails, member, false)).Render(ctx, w)
}

func (h *AdminHandlers) deactivateFund(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	id := r.PathValue("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(nil, "invalid fund id", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(nil, "invalid fund id", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	err = h.donationService.DeactivateFund(ctx, idUUID, &member.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(nil, "failed to deactivate fund", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	w.Header().Set("HX-Trigger", "fundDeactivated")
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandlers) createFund(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	// Multipart now, because the form carries a picture. The body is bounded
	// before it is read: a create form is not a reason to accept an unbounded
	// upload.
	r.Body = http.MaxBytesReader(w, r.Body, donations.MaxImageBytes+8192)

	err := r.ParseMultipartForm(donations.MaxImageBytes + 8192)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	frequency := r.FormValue("frequency")
	goalStr := r.FormValue("goal")
	var goalCents int32
	if goalStr != "" {
		goalCents, err = dollarStringToCents(goalStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			common.ErrorMessage(nil, "invalid goal amount", r.URL.Path, r.URL.Path).Render(ctx, w)

			return
		}
	}

	endDateStr := r.FormValue("date")
	var endDate *time.Time
	if endDateStr != "" {
		endDateVal, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			common.ErrorMessage(nil, "invalid end date", r.URL.Path, r.URL.Path).Render(ctx, w)

			return
		}

		endDate = &endDateVal
	}

	createFund := donations.Fund{
		Name:            name,
		Description:     description,
		PayoutFrequency: donations.PayoutFrequency(frequency),
		GoalCents:       goalCents,
		Expires:         endDate,
	}

	newFund, err := h.donationService.CreateFund(ctx, createFund)
	if err != nil {
		h.internalError(w, r)

		return
	}

	// The fund exists from here on. A picture that will not upload must not undo
	// that: creating a fund also creates the PayPal product and plan, and there is
	// no unwinding those because somebody chose the wrong file.
	//
	// So the row goes back either way, and a failed picture is reported beside the
	// form rather than in place of the fund.
	if failure := h.attachFundPicture(ctx, r, newFund.ID); failure != "" {
		FundCreatedWithoutPicture(*newFund, failure).Render(ctx, w)

		return
	}

	FundRow(*newFund).Render(ctx, w)
}

// attachFundPicture stores the picture the create form carried, if it carried
// one, and returns what to tell the admin when it could not.
//
// An empty string means there is nothing to say: either it worked, or no file was
// chosen, which is the ordinary case and not a failure.
func (h *AdminHandlers) attachFundPicture(ctx context.Context, r *http.Request, fundID uuid.UUID) string {
	upload, _, err := r.FormFile("image")
	if err != nil {
		return ""
	}

	defer upload.Close()

	if _, err = h.donationService.SaveFundImage(ctx, fundID, upload); err != nil {
		_, message := imageFailure(err)
		if message == "" {
			message = "we could not save that picture."
		}

		return message
	}

	return ""
}

func (h *AdminHandlers) fundsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	// Everything, not just what is open. A fund vanished from this tab the moment
	// it expired, taking its payout history and enrolled payees out of reach on
	// the very day a treasurer would go looking for them.
	funds, err := h.donationService.ListAllFunds(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, "failed to get funds", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	Funds(funds, &member, r.URL.Path).Render(ctx, w)
}

func (h *AdminHandlers) deactivateMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.badRequest(w, r, "")

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	_, err = h.memberService.DeactivateMember(ctx, idUUID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	w.Header().Set("HX-Trigger", "memberDeactivated")
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandlers) adminPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	currentMembers, err := h.memberService.ListActiveMembers(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, "failed to get members", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	emails, err := h.authService.GetApprovedEmails(ctx)
	if err != nil {
		h.internalError(w, r)

		return
	}

	Members(currentMembers, emails, &member, r.URL.Path).Render(ctx, w)
}

func dollarStringToCents(dollars string) (int32, error) {
	amount, err := strconv.ParseFloat(dollars, 64)
	if err != nil {
		return 0, err
	}

	return int32(amount * 100), nil
}

type ShowMessage struct {
	Target string `json:"target"`
}

// setFundImage takes an upload from an admin and replaces the fund's picture.
//
// Not part of creating a fund. Creating one also creates the PayPal product and
// plan, and that must not fail because somebody chose the wrong file -- so this
// is its own action against a fund that already exists, which also lets images be
// added to the funds there already are.
func (h *AdminHandlers) setFundImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if _, ok := h.sessionManager.Get(ctx, "member").(members.Member); !ok {
		common.Redirect(w, r, "/")

		return
	}

	fundID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badImage(ctx, w, http.StatusBadRequest, fundID, nil, "that is not a fund")

		return
	}

	// The first of two limits, and the one that bounds what reaches this process
	// at all. It refuses at the socket rather than after reading the whole thing.
	r.Body = http.MaxBytesReader(w, r.Body, donations.MaxImageBytes+1024)

	// Buffered to the same bound rather than to a temporary file: this never wants
	// a hostile upload on disk, and the limit is small enough to hold.
	if err = r.ParseMultipartForm(donations.MaxImageBytes + 1024); err != nil {
		// Every failure here used to be reported as too large, which was right for
		// one of them. A malformed body or a missing boundary is not a size problem,
		// and telling somebody their file is too big when it is not sends them off
		// to shrink a picture that was never the trouble.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.badImage(ctx, w, http.StatusRequestEntityTooLarge, fundID, nil,
				donations.ErrImageTooLarge.Error()+".")

			return
		}

		h.badImage(ctx, w, http.StatusBadRequest, fundID, nil,
			"we could not read that upload. please try again.")

		return
	}

	upload, _, err := r.FormFile("image")
	if err != nil {
		h.badImage(ctx, w, http.StatusBadRequest, fundID, nil, "choose an image first.")

		return
	}

	defer upload.Close()

	// The filename and the browser's content type are not consulted anywhere. What
	// the file is, is whatever decoding it says it is.
	image, err := h.donationService.SaveFundImage(ctx, fundID, upload)
	if err != nil {
		// The service has already logged anything unexpected; what is left here is
		// deciding what the admin is told.
		status, message := imageFailure(err)
		if message == "" {
			message = "we could not save that image. please try again."
		}

		w.WriteHeader(status)
		FundImageControl(fundID, nil, message).Render(ctx, w)

		return
	}

	FundImageControl(fundID, image, "").Render(ctx, w)
}

// removeFundImage takes a fund's picture down.
//
// A hard delete, unlike a note: it is the fund's own picture, and there is no
// decision by one person about another person's words to keep a record of.
func (h *AdminHandlers) removeFundImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if _, ok := h.sessionManager.Get(ctx, "member").(members.Member); !ok {
		common.Redirect(w, r, "/")

		return
	}

	fundID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badImage(ctx, w, http.StatusBadRequest, fundID, nil, "that is not a fund")

		return
	}

	if err = h.donationService.RemoveFundImage(ctx, fundID); err != nil {
		status, message := imageFailure(err)
		if message == "" {
			status = http.StatusInternalServerError
			message = "we could not remove that image. please try again."
		}

		h.badImage(ctx, w, status, fundID, nil, message)

		return
	}

	FundImageControl(fundID, nil, "").Render(ctx, w)
}

// imageFailure maps a refusal onto what the admin should be told. An empty
// message means it is not one of ours and they get the generic apology.
func imageFailure(err error) (int, string) {
	switch {
	case errors.Is(err, donations.ErrImageTooLarge):
		return http.StatusRequestEntityTooLarge, donations.ErrImageTooLarge.Error() + "."
	case errors.Is(err, donations.ErrImageTooManyPixels):
		return http.StatusBadRequest, donations.ErrImageTooManyPixels.Error() + "."
	case errors.Is(err, donations.ErrImageUnreadable):
		return http.StatusBadRequest, donations.ErrImageUnreadable.Error() + "."
	case errors.Is(err, donations.ErrFundClosed):
		return http.StatusConflict, "this fund is closed. its picture cannot be changed."
	default:
		return http.StatusInternalServerError, ""
	}
}

// badImage redraws the control with what went wrong. A fragment, always: this is
// swapped into the control by htmx, and a whole page put there is not something a
// browser can make sense of.
func (h *AdminHandlers) badImage(ctx context.Context, w http.ResponseWriter, status int,
	fundID uuid.UUID, image *donations.FundImage, message string) {
	w.WriteHeader(status)
	FundImageControl(fundID, image, message).Render(ctx, w)
}

// saveFundDetails changes the things about a fund that are safe to change.
//
// The fund is read first and only description, goal and end date are taken from
// the form. Name, frequency and active come back off the stored fund untouched --
// UpdateFund writes whatever it is handed, so anything not deliberately carried
// over would be blanked, and the name and frequency are held by PayPal as well as
// by us. A caller posting either gets them ignored rather than obeyed.
func (h *AdminHandlers) saveFundDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		common.Redirect(w, r, "/")

		return
	}

	fundID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	if err = r.ParseForm(); err != nil {
		h.badDetails(w, r, fundID, "we could not read that.")

		return
	}

	updated := *fund
	updated.Description = r.FormValue("description")

	if goal := r.FormValue("goal"); goal != "" {
		goalCents, errGoal := dollarStringToCents(goal)
		if errGoal != nil {
			h.badDetails(w, r, fundID, "that is not an amount.")

			return
		}

		updated.GoalCents = goalCents
	} else {
		// Cleared on purpose. A fund is allowed to stop having a target.
		updated.GoalCents = 0
	}

	if date := r.FormValue("date"); date != "" {
		expires, errDate := time.Parse("2006-01-02", date)
		if errDate != nil {
			h.badDetails(w, r, fundID, "that is not a date.")

			return
		}

		updated.Expires = &expires
	} else {
		updated.Expires = nil
	}

	saved, err := h.donationService.UpdateFund(ctx, updated, &actor.ID)
	if err != nil {
		if errors.Is(err, donations.ErrFundClosed) {
			h.badDetails(w, r, fundID, "this fund is closed. nothing about it can be changed.")

			return
		}

		h.badDetails(w, r, fundID, "we could not save that. please try again.")

		return
	}

	// Re-read rather than reused, so what comes back is what is stored.
	image, err := h.donationService.GetFundImage(ctx, fundID)
	if err != nil {
		image = nil
	}

	FundDetails(*saved, image, "", "saved.").Render(ctx, w)
}

// badDetails redraws the card with what went wrong and the fund as it still is,
// so a refusal does not leave the boxes showing values that were never stored.
//
// It takes an id and reads the fund itself rather than being handed one. The
// caller's copy was read before the save was attempted, and the save can be
// refused precisely because the fund changed in between -- it expired, or another
// admin closed it. Redrawing from the stale copy then produced an editable card
// carrying the message "this fund is closed", which is the page arguing with
// itself.
//
// Passing no fund at all is what makes that unrepresentable, rather than
// remembering to refresh it on the one path where it currently matters.
func (h *AdminHandlers) badDetails(w http.ResponseWriter, r *http.Request,
	fundID uuid.UUID, message string) {
	ctx := r.Context()

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		// No fund, no card to draw it in.
		h.internalError(w, r)

		return
	}

	image, err := h.donationService.GetFundImage(ctx, fundID)
	if err != nil {
		image = nil
	}

	w.WriteHeader(http.StatusBadRequest)
	FundDetails(*fund, image, message, "").Render(ctx, w)
}
