package adminweb

import (
	"boardfund/service/auth"
	"boardfund/service/donations"
	"boardfund/service/enrollments"
	"boardfund/service/finance"
	"boardfund/service/members"
	"boardfund/web/common"
	"boardfund/web/mux"
	"encoding/json"
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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	_, err = h.enrollmentService.DeactivateEnrollment(ctx, idUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	email := r.FormValue("email")
	if email == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	_, err = h.authService.InsertApprovedEmail(ctx, email)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	approvedEmails, err := h.authService.GetApprovedEmails(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	_, err := h.authService.DeleteApprovedEmail(ctx, email)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	approvedEmails, err := h.authService.GetApprovedEmails(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	memberUUID, err := uuid.Parse(memberIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fundIDStr := r.URL.Query().Get("fund")
	if fundIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fundUUID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	member, err := h.memberService.GetMemberByID(ctx, memberUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	enrollment, err := h.enrollmentService.FundEnrollmentExists(ctx, fundUUID, memberUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fundIDStr := r.FormValue("fund")
	if fundIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	memberIDStr := r.FormValue("member")
	if memberIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	paypalEmail := r.FormValue("paypal")
	if paypalEmail == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	username := r.FormValue("username")
	if username == "" {
		w.WriteHeader(http.StatusBadRequest)

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
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	member, err := h.memberService.GetMemberByID(ctx, memberID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	EnrollmentSuccess(*enrollment, *member).Render(ctx, w)
}

func (h *AdminHandlers) searchMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query().Get("member_search")

	membersByUsername, err := h.memberService.SearchMembersByUsername(ctx, query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	activeEnrollments, err := h.enrollmentService.GetActiveEnrollmentsForFund(ctx, fundID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Add("HX-Redirect", r.URL.String())
	Enrollments(*fund, activeEnrollments, &member, r.URL.Path).Render(ctx, w)
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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	date, err := time.Parse("01-02-2006", dateStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	req := finance.GetAuditRequest{
		FundID: fundUUID,
		Type:   reportType,
		Date:   date,
	}

	audit, err := h.financeService.GetAudit(ctx, req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	availAudits, err := h.financeService.GetAvailableAudits(ctx, idUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

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

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
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

	err = h.donationService.DeactivateFund(ctx, idUUID)
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
		w.WriteHeader(http.StatusBadRequest)

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
		w.WriteHeader(http.StatusInternalServerError)

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

	funds, err := h.donationService.ListActiveFunds(ctx)
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
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	idUUID, err := uuid.Parse(id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	_, err = h.memberService.DeactivateMember(ctx, idUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

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
		w.WriteHeader(http.StatusInternalServerError)

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

func sendFormValidationErrJSON(w http.ResponseWriter, r *http.Request, fieldErrs []fieldError) {
	errs := make(map[string]string)
	for _, err := range fieldErrs {
		errs[err.Field] = err.Error
	}

	targetID := r.Header.Get("HX-Trigger")
	if targetID == "" {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	validationPayload := map[string]map[string]string{
		targetID: errs,
	}

	payloadBytes, err := json.Marshal(validationPayload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	w.Write(payloadBytes)
}

type fieldError struct {
	Field string
	Error string
}

type menuTarget struct {
	ShowMessage ShowMessage `json:"showMenu"`
}
type ShowMessage struct {
	Target string `json:"target"`
}
