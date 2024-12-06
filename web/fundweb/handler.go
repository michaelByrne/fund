package fundweb

import (
	"boardfund/service/donations"
	"boardfund/web/common"
	"encoding/json"
	"fmt"
	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type FundHandler struct {
	donationService *donations.DonationService
	sessionManager  *scs.SessionManager
	withAuth        func(http.HandlerFunc) http.HandlerFunc
	productID       string
	clientID        string
}

func NewFundHandler(
	donationService *donations.DonationService,
	sessionManager *scs.SessionManager,
	withAuth func(http.HandlerFunc) http.HandlerFunc,
	productID, clientID string,
) *FundHandler {
	return &FundHandler{
		donationService: donationService,
		sessionManager:  sessionManager,
		withAuth:        withAuth,
		productID:       productID,
		clientID:        clientID,
	}
}

func (h *FundHandler) Register(r *http.ServeMux) {
	r.HandleFunc("/fund", h.fund)
	r.HandleFunc("/donation/plan", h.createDonationPlan)
	r.HandleFunc("/donation/once", h.createOneTimeDonation)
	r.HandleFunc("/donation/plan/complete", h.completeRecurringDonation)
	r.HandleFunc("/donation/once/complete", h.completeOneTimeDonation)
	r.HandleFunc("/donation/once/initiate", h.initiateOneTimeDonation)
	r.HandleFunc("/donation/success", h.donationSuccess)
	r.HandleFunc("/donate/{fundId}", h.withAuth(h.donate))
	r.HandleFunc("/error", h.error)
	r.HandleFunc("/ping", h.ping)
	r.HandleFunc("/", h.withAuth(h.home))
}

func (h *FundHandler) error(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	errorText := r.FormValue("error")

	templ.Handler(common.ErrorMessage(errorText)).Component.Render(ctx, w)
}

func (h *FundHandler) initiateOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount_cents")
	amountCents, err := strconv.Atoi(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund_id")
	if fundID == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("fund_id is required")).Component.Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	providerPaymentID, err := h.donationService.InitiateDonation(ctx, fundUUID, int32(amountCents))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	sendJSON(w, http.StatusOK, initDonationResponse{ProviderOrderID: providerPaymentID})
}

type initDonationResponse struct {
	ProviderOrderID string `json:"orderId"`
}

func (h *FundHandler) createOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	bcoName := r.FormValue("bconame")
	fundID := r.FormValue("fund")
	if fundID == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("fund is required")).Component.Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	if isHx(r) {
		templ.Handler(Paypal(*fund, amountCents, bcoName)).Component.Render(ctx, w)

		return
	}

	templ.Handler(common.Home(Paypal(*fund, amountCents, bcoName), h.clientID)).Component.Render(ctx, w)
}

func (h *FundHandler) donate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fundIDStr := r.PathValue("fundId")
	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	if isHx(r) {
		templ.Handler(Fund(*fund)).Component.Render(ctx, w)

		return
	}

	templ.Handler(common.Home(Fund(*fund), h.clientID)).Component.Render(ctx, w)
}

func (h *FundHandler) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func (h *FundHandler) donationSuccess(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("name is required")).Component.Render(r.Context(), w)

		return
	}

	templ.Handler(common.Home(ThankYou(name), h.clientID)).Component.Render(r.Context(), w)
}

func (h *FundHandler) completeOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	bcoName := r.FormValue("bco_name")
	fundID := r.FormValue("fund_id")
	if fundID == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("fund_id is required"))

		return
	}

	firstName := r.FormValue("first_name")
	lastName := r.FormValue("last_name")
	payerID := r.FormValue("payer_id")
	payerEmail := r.FormValue("payer_email")
	if payerEmail == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("payer_email is required"))

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))

		return
	}

	ipAddress := r.RemoteAddr
	if strings.Contains(ipAddress, "[::1]") {
		ipAddress = "127.0.0.1"
	}

	splitIP := strings.Split(ipAddress, ":")
	if len(splitIP) > 1 {
		ipAddress = splitIP[0]
	}

	completion := donations.OneTimeCompletion{
		AmountCents:    amountCents,
		FundID:         fundUUID,
		IPAddress:      ipAddress,
		BCOName:        bcoName,
		PayerID:        payerID,
		PayerEmail:     payerEmail,
		PayerFirstName: firstName,
		PayerLastName:  lastName,
	}

	err = h.donationService.CompleteDonation(ctx, completion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *FundHandler) completeRecurringDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	orderID := r.FormValue("order_id")
	planIDStr := r.FormValue("plan_id")
	if planIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("plan_id is required")).Component.Render(ctx, w)

		return
	}

	planUUID, err := uuid.Parse(planIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	fundIDStr := r.FormValue("fund_id")
	if fundIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("fund_id is required")).Component.Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	providerPlanID := r.FormValue("provider_plan_id")
	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	email := r.FormValue("email")
	payerID := r.FormValue("payer_id")
	firstName := r.FormValue("first_name")
	lastName := r.FormValue("last_name")
	providerDonationID := r.FormValue("provider_donation_id")
	bcoName := r.FormValue("bco_name")

	ipAddress := r.RemoteAddr
	if strings.Contains(ipAddress, "[::1]") {
		ipAddress = "127.0.0.1"
	}

	splitIP := strings.Split(ipAddress, ":")
	if len(splitIP) > 1 {
		ipAddress = splitIP[0]
	}

	completion := donations.RecurringCompletion{
		ProviderOrderID: orderID,
		PlanID: uuid.NullUUID{
			UUID:  planUUID,
			Valid: true,
		},
		ProviderPlanID:     providerPlanID,
		ProviderDonationID: providerDonationID,
		AmountCents:        amountCents,
		PayerEmail:         email,
		PayerID:            payerID,
		PayerFirstName:     firstName,
		PayerLastName:      lastName,
		IPAddress:          ipAddress,
		BCOName:            bcoName,
		FundID:             fundUUID,
	}

	err = h.donationService.CompleteRecurringDonation(ctx, completion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	templ.Handler(ThankYou(firstName)).Component.Render(ctx, w)
}

func (h *FundHandler) createDonationPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	interval := r.FormValue("interval")
	if interval == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("interval is required")).Component.Render(ctx, w)

		return
	}
	amount := r.FormValue("amount")
	if amount == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("amount is required")).Component.Render(ctx, w)

		return
	}

	amountInt, err := strconv.Atoi(amount)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	bcoName := r.FormValue("bconame")
	fundID := r.FormValue("fund")
	if fundID == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage("fund is required")).Component.Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	plan := donations.CreatePlan{
		FundID:       fundUUID,
		Name:         fmt.Sprintf("%d-%s", amountInt, interval),
		AmountCents:  int32(amountInt * 100),
		IntervalUnit: donations.IntervalUnit(interval),
	}

	newPlan, err := h.donationService.CreateDonationPlan(ctx, plan)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	if isHx(r) {
		templ.Handler(PaypalSubscription(*newPlan, *fund, bcoName)).Component.Render(ctx, w)

		return
	}

	templ.Handler(common.Home(PaypalSubscription(*newPlan, *fund, bcoName), h.clientID)).Component.Render(ctx, w)
}

func (h *FundHandler) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	funds, err := h.donationService.ListFunds(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(common.ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	templ.Handler(common.Home(Funds(funds), h.clientID)).Component.Render(ctx, w)
}

func (h *FundHandler) fund(w http.ResponseWriter, r *http.Request) {
	body := r.Body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	var req donations.Fund
	err = json.Unmarshal(bodyBytes, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	var fund *donations.Fund

	if r.Method == http.MethodPost {
		fund, err = h.donationService.CreateFund(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fund.ProviderID))
	} else if r.Method == http.MethodPut {
		fund, err = h.donationService.UpdateFund(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fund.ProviderID))
	}
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func dollarStringToCents(dollars string) (int32, error) {
	dollars = strings.TrimSpace(dollars)

	decimalIndex := strings.Index(dollars, ".")

	if decimalIndex == -1 {
		dollars = dollars + ".00"
		decimalIndex = len(dollars) - 3
	}

	integerPart := dollars[:decimalIndex]
	fractionalPart := dollars[decimalIndex+1:]

	if len(fractionalPart) == 1 {
		fractionalPart += "0"
	} else if len(fractionalPart) > 2 {
		fractionalPart = fractionalPart[:2]
	}

	combinedAmount := integerPart + fractionalPart
	amountInCents, err := strconv.Atoi(combinedAmount)
	if err != nil {
		return 0, fmt.Errorf("invalid dollar amount format: %s", dollars)
	}

	return int32(amountInCents), nil
}

func isHx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
