package hooksweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/mux"
	"encoding/json"
	"log/slog"
	"net/http"
)

type publisher interface {
	Publish(event string, data []byte) error
}

type WebhooksHandlers struct {
	donationService *donations.DonationService
	memberService   *members.MemberService
	publisher       publisher

	logger *slog.Logger

	webhookID string
}

func NewWebhooksHandlers(donationService *donations.DonationService, memberService *members.MemberService, publisher publisher, logger *slog.Logger, webhoodID string) *WebhooksHandlers {
	return &WebhooksHandlers{
		donationService: donationService,
		memberService:   memberService,
		publisher:       publisher,
		logger:          logger,
		webhookID:       webhoodID,
	}
}

func (h WebhooksHandlers) Register(r *mux.Router) {
	r.HandleFunc("POST /webhooks", h.webhooks)
}

func (h WebhooksHandlers) webhooks(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := verifySignature(r, h.webhookID)
	if err != nil {
		h.logger.Error("failed to verify signature", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusOK)

		return
	}

	var event webhookEvent
	err = json.Unmarshal(bodyBytes, &event)
	if err != nil {
		h.logger.Error("failed to unmarshal webhook event", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusOK)

		return
	}

	err = h.publisher.Publish(event.EventType, []byte(event.Resource))
	if err != nil {
		// Fail loudly so the provider redelivers. Answering 200 here drops the event
		// permanently, and nothing replays it.
		h.logger.Error("failed to publish event",
			slog.String("error", err.Error()),
			slog.String("event_type", event.EventType),
		)

		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	h.logger.Info("received webhook event", slog.String("event_type", event.EventType))

	w.WriteHeader(http.StatusOK)
}

type webhookEvent struct {
	EventType string          `json:"event_type"`
	Resource  json.RawMessage `json:"resource"`
}
