package adminweb

import (
	"boardfund/service/auth"
	"boardfund/service/donations"
	"boardfund/service/enrollments"
	"boardfund/service/finance"
	"boardfund/service/fundevents"
	"boardfund/service/members"
	"boardfund/service/payouts"
	"boardfund/web/common"
	"boardfund/web/mux"
	"errors"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"net/http"
	"strconv"
	"time"
)

type AdminHandlers struct {
	withAdmin         func(next http.HandlerFunc) http.HandlerFunc
	memberService     *members.MemberService
	donationService   *donations.DonationService
	enrollmentService *enrollments.EnrollmentsService
	authService       *auth.AuthService
	financeService    *finance.FinanceService
	payoutService     *payouts.PayoutService
	fundEventsService *fundevents.Service
	sessionManager    *scs.SessionManager
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
	sessionManager *scs.SessionManager,
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
		sessionManager:    sessionManager,
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
	r.HandleFunc("GET /admin/fund/audits/{id}", h.withAdmin(h.availableAudits))
	r.HandleFunc("GET /admin/fund/audit", h.withAdmin(h.fundAudit))
	r.HandleFunc("GET /admin/fund", h.withAdmin(h.fundPage))
	r.HandleFunc("GET /admin/members/search", h.withAdmin(h.searchMembers))
	r.HandleFunc("POST /admin/enrollment", h.withAdmin(h.createEnrollment))
	r.HandleFunc("GET /admin/enrollment/confirm", h.withAdmin(h.confirmEnrollment))
	r.HandleFunc("POST /admin/enrollment/cancel/{id}", h.withAdmin(h.deactivateEnrollment))
	r.HandleFunc("GET /admin/payouts", h.withAdmin(h.payoutsPage))
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

	err := r.ParseForm()
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

	err := r.ParseForm()
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

	w.Header().Add("HX-Redirect", r.URL.String())
	Enrollments(*fund, activeEnrollments, events, &member, r.URL.Path).Render(ctx, w)
}

func (h *AdminHandlers) fundAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	fundID := r.URL.Query().Get("fund")
	dateStr := r.URL.Query().Get("date")
	reportType := r.URL.Query().Get("type")

	if fundID == "" || dateStr == "" || reportType == "" {
		h.badRequest(w, r, "")

		return
	}

	date, err := time.Parse("01-02-2006", dateStr)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.badRequest(w, r, "")

		return
	}

	req := finance.GetAuditRequest{
		FundID: fundUUID,
		Type:   reportType,
		Date:   date,
	}

	audit, err := h.financeService.GetAudit(ctx, req)
	if err != nil {
		h.internalError(w, r)

		return
	}

	w.Header().Set("HX-Redirect", r.URL.String())
	FundPaymentsAudit(*audit, &member, r.URL.Path).Render(ctx, w)
}

func (h *AdminHandlers) availableAudits(w http.ResponseWriter, r *http.Request) {
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

	availAudits, err := h.financeService.GetAvailableAudits(ctx, idUUID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	FundAudits(availAudits).Render(ctx, w)
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
	Member(*memberDetails, &member, r.URL.Path).Render(ctx, w)
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

	err := r.ParseForm()
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

	FundRow(*newFund).Render(ctx, w)
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
