package donations

import (
	"boardfund/events"
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
	fmt.Printf("Payment sale completed: %s\n", data)
}
