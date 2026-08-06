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
	Subscribe(event string, cb func(data []byte) error) error
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

func (h *Handlers) payoutItemUpdated(data []byte) error {
	var event PayoutItemEvent
	if err := json.Unmarshal(data, &event); err != nil {
		// Unparseable now is unparseable on every redelivery, so it is
		// acknowledged rather than retried to exhaustion.
		h.logger.Error("discarding unparseable payout item event", slog.String("error", err.Error()))

		return nil
	}

	if event.PayoutItemID == "" {
		h.logger.Error("discarding payout item event with no payout_item_id")

		return nil
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
			return fmt.Errorf("failed to apply payout item event for payout %s: %w", payoutID, err)
		}

		h.logger.Info("applied payout item event",
			slog.String("payout_id", payoutID.String()),
			slog.String("transaction_status", event.TransactionStatus),
		)

		return nil
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
		// Returned rather than swallowed so the message comes back. The reconciler
		// is still the backstop, but it polls on a schedule and a redelivery is
		// both sooner and cheaper than a full reconcile.
		return fmt.Errorf("failed to apply payout item event %s: %w", event.PayoutItemID, err)
	}

	h.logger.Info("applied payout item event",
		slog.String("payout_item_id", event.PayoutItemID),
		slog.String("transaction_status", event.TransactionStatus),
	)

	return nil
}

func (h *Handlers) payoutBatchUpdated(data []byte) error {
	var event PayoutBatchEvent
	if err := json.Unmarshal(data, &event); err != nil {
		h.logger.Error("discarding unparseable payout batch event", slog.String("error", err.Error()))

		return nil
	}

	senderBatchID := event.BatchHeader.SenderBatchHeader.SenderBatchID
	if senderBatchID == "" {
		h.logger.Error("discarding payout batch event with no sender_batch_id")

		return nil
	}

	senderBatchUUID, err := uuidFromString(senderBatchID)
	if err != nil {
		// Not one of ours: the same PayPal account may be used for payouts we did
		// not originate. Acknowledged rather than retried -- a batch id that is not
		// a uuid will not become one, and redelivering it four times before parking
		// it would turn somebody else's payouts into our error log.
		h.logger.Warn("payout batch event sender_batch_id is not a known batch",
			slog.String("sender_batch_id", senderBatchID),
		)

		//nolint:nilerr // the error is the answer: this batch is not ours to handle.
		return nil
	}

	batch, err := h.payoutStore.GetBatchBySenderBatchID(context.Background(), senderBatchUUID)
	if err != nil {
		return fmt.Errorf("failed to look up batch by sender batch id %s: %w", senderBatchID, err)
	}

	_, err = h.payoutStore.SetBatchStatus(context.Background(), SetBatchStatus{
		BatchID: batch.ID,
		Status:  ProviderStatusToStatus(event.BatchHeader.BatchStatus),
	})
	if err != nil {
		return fmt.Errorf("failed to apply payout batch event for batch %s: %w", batch.ID, err)
	}

	h.logger.Info("applied payout batch event",
		slog.String("batch_id", batch.ID.String()),
		slog.String("batch_status", event.BatchHeader.BatchStatus),
	)

	return nil
}
