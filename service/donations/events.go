package donations

import (
	"boardfund/events"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"log/slog"
)

type Handlers struct {
	donationStore donationStore

	logger *slog.Logger
}

func NewHandlers(donationStore donationStore, logger *slog.Logger) *Handlers {
	return &Handlers{
		donationStore: donationStore,
		logger:        logger,
	}
}

func (h *Handlers) Subscribe(subscribe subscriber) error {
	var errResult error
	if err := subscribe.Subscribe(events.SubscriptionPaymentCompleted, h.paymentSaleCompleted); err != nil {
		errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", events.SubscriptionPaymentCompleted, err))
	}

	return errResult
}

func (h *Handlers) paymentSaleCompleted(data []byte) {
	var paymentSale PaymentSaleEvent
	if err := json.Unmarshal(data, &paymentSale); err != nil {
		h.logger.Error("failed to unmarshal payment sale event", slog.String("error", err.Error()))

		return
	}

	fmt.Printf("Payment sale completed: %+v\n", paymentSale)
}
