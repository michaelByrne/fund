package payouts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"boardfund/messaging"

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
		messaging.PayoutsItemSucceeded,
		messaging.PayoutsItemFailed,
		messaging.PayoutsItemBlocked,
		messaging.PayoutsItemCanceled,
		messaging.PayoutsItemDenied,
		messaging.PayoutsItemHeld,
		messaging.PayoutsItemRefunded,
		messaging.PayoutsItemReturned,
		messaging.PayoutsItemUnclaimed,
	}

	for _, event := range itemEvents {
		if err := sub.Subscribe(event, h.payoutItemUpdated); err != nil {
			errResult = multierror.Append(errResult, fmt.Errorf("failed to subscribe to %s: %w", event, err))
		}
	}

	batchEvents := []string{
		messaging.PayoutsBatchSuccess,
		messaging.PayoutsBatchDenied,
		messaging.PayoutsBatchProcessing,
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

	status := ProviderStatusToStatus(event.TransactionStatus)
	feeCents := dollarStringToCents(event.PayoutItemFee.Value)

	// Match on sender_item_id first. It carries our own payout ID and is populated
	// from the moment the batch is submitted, whereas provider_payout_item_id is
	// only written once ReconcileBatch has run. Keying on the provider's ID would
	// therefore miss every webhook that arrives before the first reconcile -- which
	// in practice is most of them, since PayPal reports item outcomes within
	// seconds of accepting a batch.
	//
	// Recording the provider's item ID in the same statement also means a later
	// reconcile, and any subsequent webhook, both have it available.
	if payoutID, errParse := uuidFromString(event.PayoutItem.SenderItemID); errParse == nil {
		_, err := h.payoutStore.SetPayoutResult(context.Background(), SetPayoutResult{
			PayoutID:             payoutID,
			ProviderPayoutItemID: event.PayoutItemID,
			Status:               status,
			FailureReason:        failureReason,
			ProviderFeeCents:     feeCents,
		})
		if err != nil {
			h.logger.Error("failed to apply payout item event",
				slog.String("error", err.Error()),
				slog.String("payout_id", payoutID.String()),
				slog.String("payout_item_id", event.PayoutItemID),
				slog.String("transaction_status", event.TransactionStatus),
			)

			return
		}

		h.logger.Info("applied payout item event",
			slog.String("payout_id", payoutID.String()),
			slog.String("transaction_status", event.TransactionStatus),
		)

		return
	}

	// No usable sender_item_id. Fall back to the provider's own ID, which resolves
	// once the batch has been reconciled at least once.
	_, err := h.payoutStore.SetPayoutStatusByProviderItemID(context.Background(), SetPayoutStatusByItem{
		ProviderPayoutItemID: event.PayoutItemID,
		Status:               status,
		FailureReason:        failureReason,
		ProviderFeeCents:     feeCents,
	})
	if err != nil {
		// Not fatal: the reconciler polls the provider and will settle this item
		// regardless. Logged so a systematically failing webhook is visible.
		h.logger.Error("failed to apply payout item event by provider item id",
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

	senderBatchUUID, err := uuidFromString(senderBatchID)
	if err != nil {
		// Not one of ours: the same PayPal account may be used for payouts we did
		// not originate.
		h.logger.Warn("payout batch event sender_batch_id is not a known batch",
			slog.String("sender_batch_id", senderBatchID),
		)

		return
	}

	batch, err := h.payoutStore.GetBatchBySenderBatchID(context.Background(), senderBatchUUID)
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
