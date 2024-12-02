package web

import (
	"boardfund/service/donations"
	"context"
	"encoding/json"
	"fmt"
	"github.com/a-h/templ"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type donationService interface {
	CreateDonationPlan(ctx context.Context, plan donations.CreatePlan) (*donations.DonationPlan, error)
	CreateProduct(ctx context.Context, name, description string) (string, error)
	CaptureDonationOrder(ctx context.Context, createCapture donations.CreateCapture) error
}

type DonationHandler struct {
	donationService donationService
	productID       string
	clientID        string
}

func NewDonationHandler(donationService donationService, productID, clientID string) *DonationHandler {
	return &DonationHandler{
		donationService: donationService,
		productID:       productID,
		clientID:        clientID,
	}
}

func (h *DonationHandler) Register(r *http.ServeMux) {
	r.HandleFunc("/plan", h.createDonationPlan)
	r.HandleFunc("/product", h.createProduct)
	r.HandleFunc("/subscription/capture", h.captureDonationOrder)
	r.HandleFunc("/subscription/success", h.subscriptionSuccess)
	r.HandleFunc("/ping", h.ping)
	r.HandleFunc("/", h.home)
}

func (h *DonationHandler) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func (h *DonationHandler) subscriptionSuccess(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage("name is required")).Component.Render(r.Context(), w)

		return
	}

	templ.Handler(Home(ThankYou(name), h.clientID)).Component.Render(r.Context(), w)
}

func (h *DonationHandler) captureDonationOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	orderID := r.FormValue("order_id")
	planIDStr := r.FormValue("plan_id")
	planID, err := strconv.Atoi(planIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	providerPlanID := r.FormValue("provider_plan_id")
	amountStr := r.FormValue("amount")
	amountCents, err := dollarStringToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

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

	capture := donations.CreateCapture{
		ProviderOrderID:    orderID,
		PlanID:             int32(planID),
		ProviderPlanID:     providerPlanID,
		ProviderDonationID: providerDonationID,
		AmountCents:        amountCents,
		PayerEmail:         email,
		PayerID:            payerID,
		PayerFirstName:     firstName,
		PayerLastName:      lastName,
		IPAddress:          ipAddress,
		BCOName:            bcoName,
	}

	err = h.donationService.CaptureDonationOrder(ctx, capture)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	templ.Handler(ThankYou(firstName)).Component.Render(ctx, w)
}

func (h *DonationHandler) createDonationPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	interval := r.FormValue("interval")
	if interval == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage("interval is required")).Component.Render(ctx, w)

		return
	}
	amount := r.FormValue("amount")
	if amount == "" {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage("amount is required")).Component.Render(ctx, w)

		return
	}

	amountInt, err := strconv.Atoi(amount)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	bcoName := r.FormValue("bconame")

	plan := donations.CreatePlan{
		Name:         fmt.Sprintf("%d-%s", amountInt, interval),
		AmountCents:  int32(amountInt * 100),
		IntervalUnit: donations.IntervalUnit(interval),
	}

	newPlan, err := h.donationService.CreateDonationPlan(ctx, plan)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templ.Handler(ErrorMessage(err.Error())).Component.Render(ctx, w)

		return
	}

	if isHx(r) {
		templ.Handler(Paypal(newPlan.ID, newPlan.ProviderPlanID, amount, interval, bcoName)).Component.Render(ctx, w)

		return
	}

	templ.Handler(Home(Paypal(newPlan.ID, newPlan.ProviderPlanID, amount, interval, bcoName), h.clientID)).Component.Render(ctx, w)
}

func (h *DonationHandler) home(w http.ResponseWriter, r *http.Request) {
	templ.Handler(Home(DonationForm(h.productID), h.clientID)).Component.Render(r.Context(), w)
}

func (h *DonationHandler) createProduct(w http.ResponseWriter, r *http.Request) {
	body := r.Body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	var req productRequest
	err = json.Unmarshal(bodyBytes, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	productID, err := h.donationService.CreateProduct(r.Context(), req.Name, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(productID))
}

type productRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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
