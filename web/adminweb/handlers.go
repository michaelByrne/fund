package adminweb

import (
	"boardfund/service/auth"
	"boardfund/service/donations"
	"boardfund/service/finance"
	"boardfund/service/members"
	"boardfund/web/common"
	"boardfund/web/mux"
	"encoding/json"
	"errors"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"net/http"
	"net/mail"
	"strconv"
	"time"
)

type AdminHandlers struct {
	withAdmin       func(next http.HandlerFunc) http.HandlerFunc
	memberService   *members.MemberService
	donationService *donations.DonationService
	financeServe    *finance.FinanceService
	sessionManager  *scs.SessionManager
	clientID        string
}

func NewAdminHandlers(
	withAdmin func(next http.HandlerFunc) http.HandlerFunc,
	memberService *members.MemberService,
	donationService *donations.DonationService,
	financeService *finance.FinanceService,
	sessionManager *scs.SessionManager,
	clientID string,
) *AdminHandlers {
	return &AdminHandlers{
		withAdmin:       withAdmin,
		memberService:   memberService,
		donationService: donationService,
		financeServe:    financeService,
		sessionManager:  sessionManager,
		clientID:        clientID,
	}
}

func (h *AdminHandlers) Register(r *mux.Router) {
	r.HandleFunc("/admin", h.withAdmin(h.admin))
	r.HandleFunc("GET /admin/funds", h.withAdmin(h.funds))
	r.HandleFunc("POST /admin/member", h.withAdmin(h.createMember))
	r.HandleFunc("POST /admin/fund", h.withAdmin(h.createFund))
	r.HandleFunc("POST /admin/fund/deactivate/{id}", h.withAdmin(h.deactivateFund))
	r.HandleFunc("POST /admin/member/deactivate/{id}", h.withAdmin(h.deactivateMember))
	r.HandleFunc("GET /admin/member/{id}", h.withAdmin(h.member))
	r.HandleFunc("GET /admin/fund/audit/{id}", h.withAdmin(h.availableAudits))
	r.HandleFunc("POST /admin/fund/audit/{id}", h.withAdmin(h.fundAudit))
}

func (h *AdminHandlers) fundAudit(w http.ResponseWriter, r *http.Request) {
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

	frequency := r.FormValue("frequency")
	if frequency == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	var dateStr string
	if frequency == "monthly" {
		dateStr = r.FormValue("period")
	} else {
		dateStr = time.Now().Format("01-02-2006")
	}

	id := r.PathValue("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	_, err = uuid.Parse(id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	FundAuditResult(dateStr).Render(ctx, w)
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

	availableDates, err := h.financeServe.GetAvailableReportDates(ctx, "payments", idUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	FundAudit(availableDates).Render(ctx, w)
}

func (h *AdminHandlers) member(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandlers) funds(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandlers) createMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	var fieldErrs []fieldError

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "failed to parse form", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	email := r.FormValue("email")
	_, err = mail.ParseAddress(email)
	if err != nil {
		fieldErrs = append(fieldErrs, fieldError{"email", "invalid email address"})
	}

	user := r.FormValue("username")
	first := r.FormValue("first")
	last := r.FormValue("last")

	if len(fieldErrs) > 0 {
		sendFormValidationErrJSON(w, r, fieldErrs)

		return
	}

	createMember := members.CreateMember{
		Email:     email,
		BCOName:   user,
		FirstName: first,
		LastName:  last,
	}

	newMember, err := h.memberService.CreateMember(ctx, createMember)
	if err != nil {
		if errors.Is(err, auth.ErrUsernameExists) {
			fieldErrs = append(fieldErrs, fieldError{"username", "username already exists"})
			sendFormValidationErrJSON(w, r, fieldErrs)

			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, "failed to create member", r.URL.Path, r.URL.Path).Render(ctx, w)

		return
	}

	MemberRow(*newMember).Render(ctx, w)
}

func (h *AdminHandlers) admin(w http.ResponseWriter, r *http.Request) {
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

	Members(currentMembers, &member, r.URL.Path).Render(ctx, w)
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
