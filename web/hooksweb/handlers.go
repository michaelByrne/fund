package hooksweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/mux"
	"fmt"
	"log/slog"
	"net/http"
)

type WebhooksHandlers struct {
	donationService *donations.DonationService
	memberService   *members.MemberService

	logger *slog.Logger
}

func NewWebhooksHandlers(donationService *donations.DonationService, memberService *members.MemberService, logger *slog.Logger) *WebhooksHandlers {
	return &WebhooksHandlers{
		donationService: donationService,
		memberService:   memberService,
		logger:          logger,
	}
}

func (h WebhooksHandlers) Register(r *mux.Router) {
	r.HandleFunc("POST /webhooks/payment", h.payment)
}

func (h WebhooksHandlers) payment(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%+v\n", r)

	err := verifySignature(r, "1VT58541NE9487622")
	if err != nil {
		h.logger.Error("failed to verify signature", slog.String("error", err.Error()))

		return
	}

	w.WriteHeader(http.StatusOK)
}
