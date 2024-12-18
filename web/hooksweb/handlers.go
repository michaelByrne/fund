package hooksweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/mux"
	"log/slog"
	"net/http"
)

type WebhooksHandlers struct {
	donationService *donations.DonationService
	memberService   *members.MemberService

	logger *slog.Logger

	webhookID string
}

func NewWebhooksHandlers(donationService *donations.DonationService, memberService *members.MemberService, logger *slog.Logger, webhoodID string) *WebhooksHandlers {
	return &WebhooksHandlers{
		donationService: donationService,
		memberService:   memberService,
		logger:          logger,
		webhookID:       webhoodID,
	}
}

func (h WebhooksHandlers) Register(r *mux.Router) {
	r.HandleFunc("POST /webhooks", h.webhooks)
}

func (h WebhooksHandlers) webhooks(w http.ResponseWriter, r *http.Request) {
	err := verifySignature(r, h.webhookID)
	if err != nil {
		h.logger.Error("failed to verify signature", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusOK)

		return
	}

	w.WriteHeader(http.StatusOK)
}
