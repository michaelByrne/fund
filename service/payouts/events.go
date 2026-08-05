package payouts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"boardfund/events"

	"github.com/hashicorp/go-multierror"
)

type subscriber interface {
	Subscribe(event string, cb func(data []byte)) error
}

// PayoutItemEvent is the resource carried by PAYMENT.PAYOUTS-ITEM.* webhooks.
type PayoutItemEvent struct {
	PayoutItemID      string       `json:"payout_item_id"`
	TransactionStatus string       `json:"transaction_status"`
	PayoutBatchID     string       `json:"payout_batch_id"`
	PayoutItemFee     EventAmount  `json:"payout_item_fee"`
	Errors            *EventErrors `json:"errors,omitempty"`
	PayoutItem        struct {
		SenderItemID string `json:"sender_item_id"`
	} `json:"payout_item"`
}

// PayoutBatchEvent is the resource carried by PAYMENT.PAYOUTSBATCH.* webhooks.
type PayoutBatchEvent struct {
	BatchHeader struct {
		PayoutBatchID     string `json:"payout_batch_id"`
		BatchStatus       string `json:"batch_status"`
		SenderBatchHeader struct {
			SenderBatchID string `json:"sender_batch_id"`
		} `json:"sender_batch_header"`
	} `json:"batch_header"`
}

type EventAmount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

type EventErrors struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type Handlers struct {
	payoutStore payoutStore

	logger *slog.Logger
}

func NewHandlers(payoutStore payoutStore, logger *slog.Logger) *Handlers {
	return &Handlers{
		payoutStore: payoutStore,
		logger:      logger,
	}
}

func (h *Handlers) Subscribe(sub subscriber) error {
	var errResult error

	itemEvents := []string{
		events.PayoutsItemSucceeded,
		events.PayoutsItemFailed,
		events.PayoutsItemBlocked,
		events.PayoutsItemCanceled,
		events.PayoutsItemDenied,
		events.PayoutsItemHeld,
		events.PayoutsItemRefunded,
		events.PayoutsItemReturned,
		events.PayoutsItemUnclaimed,
	}

	for _, event := range itemEvents {
		if err := sub.Subscribe(event, h.payoutItemUpdated); err != nil {
			errResult = multierror.Append(errResult, fmt.Errorf("failed to subscribe to %s: %w", event, err))
		}
	}

	batchEvents := []string{
		events.PayoutsBatchSuccess,
		events.PayoutsBatchDenied,
		events.PayoutsBatchProcessing,
	}

	for _, event := range batchEvents {
		if err := sub.Subscribe(event, h.payoutBatchUpdated); err != nil {
			errResult = multierror.Append(errResult, fmt.Errorf("failed to subscribe to %s: %w", event, err))
		}
	}

	return errResult
}

func (h *Handlers) payoutItemUpdated(data []byte) {
	var event PayoutItemEvent
	if err := json.Unmarshal(data, &event); err != nil {
		h.logger.Error("failed to unmarshal payout item event", slog.String("error", err.Error()))

		return
	}

	if event.PayoutItemID == "" {
		h.logger.Error("payout item event has no payout_item_id")

		return
	}

	var failureReason string
	if event.Errors != nil {
		failureReason = event.Errors.Message
	}

	_, err := h.payoutStore.SetPayoutStatusByProviderItemID(context.Background(), SetPayoutStatusByItem{
		ProviderPayoutItemID: event.PayoutItemID,
		Status:               ProviderStatusToStatus(event.TransactionStatus),
		FailureReason:        failureReason,
		ProviderFeeCents:     dollarStringToCents(event.PayoutItemFee.Value),
	})
	if err != nil {
		// Not fatal: the reconciler polls the provider and will settle this item
		// regardless. Logged so a systematically failing webhook is visible.
		h.logger.Error("failed to apply payout item event",
			slog.String("error", err.Error()),
			slog.String("payout_item_id", event.PayoutItemID),
			slog.String("transaction_status", event.TransactionStatus),
		)

		return
	}

	h.logger.Info("applied payout item event",
		slog.String("payout_item_id", event.PayoutItemID),
		slog.String("transaction_status", event.TransactionStatus),
	)
}

func (h *Handlers) payoutBatchUpdated(data []byte) {
	var event PayoutBatchEvent
	if err := json.Unmarshal(data, &event); err != nil {
		h.logger.Error("failed to unmarshal payout batch event", slog.String("error", err.Error()))

		return
	}

	senderBatchID := event.BatchHeader.SenderBatchHeader.SenderBatchID
	if senderBatchID == "" {
		h.logger.Error("payout batch event has no sender_batch_id")

		return
	}

	batchID, err := uuidFromString(senderBatchID)
	if err != nil {
		// Not one of ours: the same PayPal account may be used for payouts we did
		// not originate.
		h.logger.Warn("payout batch event sender_batch_id is not a known batch",
			slog.String("sender_batch_id", senderBatchID),
		)

		return
	}

	batch, err := h.payoutStore.GetBatchBySenderBatchID(context.Background(), batchID)
	if err != nil {
		h.logger.Error("failed to look up batch by sender batch id",
			slog.String("error", err.Error()),
			slog.String("sender_batch_id", senderBatchID),
		)

		return
	}

	_, err = h.payoutStore.SetBatchStatus(context.Background(), SetBatchStatus{
		BatchID: batch.ID,
		Status:  ProviderStatusToStatus(event.BatchHeader.BatchStatus),
	})
	if err != nil {
		h.logger.Error("failed to apply payout batch event",
			slog.String("error", err.Error()),
			slog.String("batch_id", batch.ID.String()),
		)

		return
	}

	h.logger.Info("applied payout batch event",
		slog.String("batch_id", batch.ID.String()),
		slog.String("batch_status", event.BatchHeader.BatchStatus),
	)
}
