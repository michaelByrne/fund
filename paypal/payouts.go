package paypal

import (
	"context"
	"encoding/json"
	"net/url"

	"boardfund/service/payouts"

	"github.com/google/uuid"
)

// SubmitBatch creates a payout batch.
//
// senderBatchID is PayPal's idempotency key: a second call carrying a
// sender_batch_id PayPal has already seen is rejected rather than paid again. It is
// persisted before this call is made, so a request that times out can be retried
// without risking a duplicate payout.
//
// The response carries only the batch header -- no per-item IDs. Those are learned
// from GetBatchStatus, matched back via sender_item_id.
func (p Paypal) SubmitBatch(ctx context.Context, senderBatchID uuid.UUID, note string, items []payouts.ProviderPayoutItem) (*payouts.ProviderBatchResult, error) {
	if note == "" {
		note = "BCO mutual aid payout"
	}

	payoutItems := make([]PayoutItem, 0, len(items))
	for _, item := range items {
		itemNote := item.Note
		if itemNote == "" {
			itemNote = note
		}

		payoutItems = append(payoutItems, PayoutItem{
			RecipientType: "EMAIL",
			Receiver:      item.ReceiverEmail,
			Note:          itemNote,
			// Our payout ID, echoed back by PayPal so reconciliation can attribute
			// each item to the row that created it.
			SenderItemID: item.PayoutID.String(),
			Amount: PayoutAmount{
				Value:        centsToDecimalString(item.AmountCents),
				CurrencyCode: "USD",
			},
		})
	}

	request := CreatePayoutRequest{
		SenderBatchHeader: SenderBatchHeader{
			SenderBatchID: senderBatchID.String(),
			EmailSubject:  "You have a payment from the BCO mutual aid fund",
			EmailMessage:  note,
		},
		Items: payoutItems,
	}

	responseBytes, err := p.client.postWithResponse(ctx, "/v1/payments/payouts", request)
	if err != nil {
		return nil, err
	}

	var response CreatePayoutResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return nil, err
	}

	return &payouts.ProviderBatchResult{
		ProviderBatchID: response.BatchHeader.PayoutBatchID,
		Status:          response.BatchHeader.BatchStatus,
	}, nil
}

// GetBatchStatus reads a submitted batch back, including per-item status. This is
// the authoritative view: webhooks may be dropped, this cannot be.
func (p Paypal) GetBatchStatus(ctx context.Context, providerBatchID string) (*payouts.ProviderBatchResult, error) {
	path := "/v1/payments/payouts/" + url.PathEscape(providerBatchID) + "?page_size=1000&total_required=true"

	responseBytes, err := p.client.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var response PayoutBatchDetails
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return nil, err
	}

	items := make([]payouts.ProviderItemResult, 0, len(response.Items))
	for _, item := range response.Items {
		// sender_item_id is the payout ID we sent. If it does not parse, the item did
		// not originate here and must not be attributed to one of our rows -- the
		// service treats uuid.Nil as unmatched and refuses to write it.
		payoutID, errParse := uuid.Parse(item.SenderItemID)
		if errParse != nil {
			payoutID = uuid.Nil
		}

		items = append(items, payouts.ProviderItemResult{
			PayoutID:             payoutID,
			ProviderPayoutItemID: item.PayoutItemID,
			Status:               item.TransactionStatus,
			FeeCents:             decimalDollarStringToCents(item.PayoutItemFee.Value),
		})
	}

	return &payouts.ProviderBatchResult{
		ProviderBatchID: response.BatchHeader.PayoutBatchID,
		Status:          response.BatchHeader.BatchStatus,
		Items:           items,
	}, nil
}
