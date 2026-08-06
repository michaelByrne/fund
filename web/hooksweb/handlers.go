package hooksweb

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/web/mux"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type publisher interface {
	Publish(event string, data []byte) error
}

// deliveries records which transmissions have already been accepted, so a replay
// of a genuinely signed request is not handled twice.
type deliveries interface {
	RecordDelivery(ctx context.Context, transmissionID, eventType string) (bool, error)
}

type WebhooksHandlers struct {
	donationService *donations.DonationService
	memberService   *members.MemberService
	publisher       publisher
	deliveries      deliveries

	logger *slog.Logger

	webhookID string
}

func NewWebhooksHandlers(donationService *donations.DonationService, memberService *members.MemberService, publisher publisher, deliveries deliveries, logger *slog.Logger, webhoodID string) *WebhooksHandlers {
	return &WebhooksHandlers{
		donationService: donationService,
		memberService:   memberService,
		publisher:       publisher,
		deliveries:      deliveries,
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
		// Being unable to check is not the same as checking and finding it invalid.
		// A bad signature is settled and a retry would fail identically, so it is
		// dropped. A certificate we could not fetch or trust is our fault or
		// PayPal's, and answering 200 to that would tell PayPal the event was
		// delivered -- discarding a real event permanently, on the strength of a
		// fault at our end.
		if errors.Is(err, errUnverifiable) {
			h.logger.Error("could not verify signature, asking for redelivery",
				slog.String("error", err.Error()),
			)

			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		h.logger.Error("rejected webhook with an invalid signature", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusOK)

		return
	}

	h.accept(r.Context(), w, r.Header.Get("paypal-transmission-id"), bodyBytes)
}

// accept handles a request whose signature has already been checked.
//
// Split from the verification above so it can be tested: passing verification in
// a test would mean producing a signature that chains to a public root, which is
// not something a test can do, and the decisions below -- replay, publish, what
// to tell PayPal -- are the ones worth checking.
func (h WebhooksHandlers) accept(ctx context.Context, w http.ResponseWriter, transmissionID string, body []byte) {
	var event webhookEvent

	err := json.Unmarshal(body, &event)
	if err != nil {
		h.logger.Error("failed to unmarshal webhook event", slog.String("error", err.Error()))

		w.WriteHeader(http.StatusOK)

		return
	}

	// A valid signature says PayPal sent this. It does not say we have not
	// already handled it: a captured request replays for as long as its
	// certificate verifies, and PayPal itself redelivers anything answered with a
	// 5xx. Recorded before publishing, so a replay never reaches the stream or the
	// handlers behind it.
	//
	// A failure here is not a reason to drop the event. It is a reason to be asked
	// again, when the database is back.
	fresh, err := h.deliveries.RecordDelivery(ctx, transmissionID, event.EventType)
	if err != nil {
		h.logger.Error("failed to record webhook delivery, asking for redelivery",
			slog.String("error", err.Error()),
			slog.String("event_type", event.EventType),
		)

		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	if !fresh {
		h.logger.Info("ignoring a webhook we have already accepted",
			slog.String("event_type", event.EventType),
			slog.String("transmission_id", transmissionID),
		)

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
