package homeweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/service/stats"
	"boardfund/web/common"
	"boardfund/web/mux"
	"encoding/json"
	"fmt"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const internalErrMessage = "internal error"

type FundHandlers struct {
	donationService *donations.DonationService
	statsService    *stats.StatsService
	sessionManager  *scs.SessionManager
	withAuth        func(http.HandlerFunc) http.HandlerFunc
	logger          *slog.Logger
	productID       string
	clientID        string
}

func NewFundHandlers(
	donationService *donations.DonationService,
	statsService *stats.StatsService,
	sessionManager *scs.SessionManager,
	withAuth func(http.HandlerFunc) http.HandlerFunc,
	logger *slog.Logger,
	productID, clientID string,
) *FundHandlers {
	return &FundHandlers{
		donationService: donationService,
		statsService:    statsService,
		sessionManager:  sessionManager,
		withAuth:        withAuth,
		logger:          logger,
		productID:       productID,
		clientID:        clientID,
	}
}

func (h *FundHandlers) Register(r *mux.Router) {
	r.HandleFunc("/fund", h.fund)
	r.HandleFunc("/donation/plan", h.withAuth(h.createDonationPlan))
	r.HandleFunc("/donation/once", h.withAuth(h.createOneTimeDonation))
	r.HandleFunc("/donation/plan/complete", h.withAuth(h.completeRecurringDonation))
	r.HandleFunc("/donation/once/complete", h.withAuth(h.completeOneTimeDonation))
	r.HandleFunc("/donation/once/initiate", h.withAuth(h.initiateOneTimeDonation))
	r.HandleFunc("/donation/success", h.withAuth(h.donationSuccess))
	r.HandleFunc("/donate/{fundId}", h.withAuth(h.donate))
	r.HandleFunc("/error", h.error)
	r.HandleFunc("/ping", h.ping)
	r.HandleFunc("/about", h.about)
	r.HandleFunc("/", h.withAuth(h.home))
}

func (h *FundHandlers) error(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(nil, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	errorText := r.FormValue("error")

	common.ErrorMessage(nil, errorText, "/", r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) about(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		About(nil, r.URL.Path).Render(ctx, w)

		return
	}

	About(&member, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) initiateOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount_cents")
	amountCents, err := strconv.Atoi(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund_id")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	providerPaymentID, err := h.donationService.InitiateDonation(ctx, fundUUID, int32(amountCents))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	sendJSON(w, http.StatusOK, initDonationResponse{ProviderOrderID: providerPaymentID})
}

type initDonationResponse struct {
	ProviderOrderID string `json:"orderId"`
}

func (h *FundHandlers) createOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	Paypal(*fund, amountCents, h.clientID).Render(ctx, w)
}

func (h *FundHandlers) donate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(&member, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundIDStr := r.PathValue("fundId")
	fundID, err := uuid.Parse(fundIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundStats, err := h.statsService.GetFundStats(ctx, fundID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	w.Header().Set("HX-Redirect", r.URL.Path)
	Fund(*fund, *fundStats, &member, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func (h *FundHandlers) donationSuccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(&member, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	ThankYou(member, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) completeOneTimeDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(&member, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund_id")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	orderID := r.FormValue("order_id")
	if orderID == "" {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	paymentID := r.FormValue("payment_id")
	if paymentID == "" {
		h.logger.Error("missing payment id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

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
		AmountCents:       amountCents,
		FundID:            fundUUID,
		ProviderOrderID:   orderID,
		ProviderPaymentID: paymentID,
	}

	err = h.donationService.CompleteDonation(ctx, member.ID, completion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *FundHandlers) completeRecurringDonation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(&member, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	planIDStr := r.FormValue("plan_id")
	if planIDStr == "" {
		h.logger.Error("missing plan id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	planUUID, err := uuid.Parse(planIDStr)
	if err != nil {
		h.logger.Error("unable to parse plan id", slog.String("plan_id", planIDStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundIDStr := r.FormValue("fund_id")
	if fundIDStr == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundIDStr)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundIDStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amountStr), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	providerSubscriptionID := r.FormValue("subscription_id")
	if providerSubscriptionID == "" {
		h.logger.Error("missing subscription id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	orderID := r.FormValue("order_id")
	if orderID == "" {
		h.logger.Error("missing order_id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	completion := donations.RecurringCompletion{
		PlanID: uuid.NullUUID{
			UUID:  planUUID,
			Valid: true,
		},
		AmountCents:            amountCents,
		FundID:                 fundUUID,
		ProviderOrderID:        orderID,
		ProviderSubscriptionID: providerSubscriptionID,
	}

	err = h.donationService.CompleteRecurringDonation(ctx, member.ID, completion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	ThankYou(member, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) createDonationPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(nil, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	err := r.ParseForm()
	if err != nil {
		h.logger.Error("unable to parse form", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	interval := r.FormValue("interval")
	if interval == "" {
		h.logger.Error("missing interval")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "interval is required", "/", r.URL.Path).Render(ctx, w)

		return
	}
	amount := r.FormValue("amount")
	if amount == "" {
		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, "amount is required", "/", r.URL.Path).Render(ctx, w)

		return
	}

	amountInt, err := strconv.Atoi(amount)
	if err != nil {
		h.logger.Error("unable to parse amount", slog.String("amount", amount), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundID := r.FormValue("fund")
	if fundID == "" {
		h.logger.Error("missing fund id")

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fundUUID, err := uuid.Parse(fundID)
	if err != nil {
		h.logger.Error("unable to parse fund id", slog.String("fund_id", fundID), slog.String("error", err.Error()))

		w.WriteHeader(http.StatusBadRequest)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

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
		fmt.Printf("error creating plan: %s\n", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	fund, err := h.donationService.GetFundByID(ctx, fundUUID)
	if err != nil {
		fmt.Printf("error getting fund: %s\n", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	PaypalSubscription(*newPlan, h.clientID, fund.Name).Render(ctx, w)
}

func (h *FundHandlers) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		common.ErrorMessage(&member, "unauthorized", "/", r.URL.Path).Render(ctx, w)

		return
	}

	funds, err := h.donationService.ListActiveFunds(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		common.ErrorMessage(&member, internalErrMessage, "/", r.URL.Path).Render(ctx, w)

		return
	}

	Funds(funds, &member, r.URL.Path).Render(ctx, w)
}

func (h *FundHandlers) fund(w http.ResponseWriter, r *http.Request) {
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
