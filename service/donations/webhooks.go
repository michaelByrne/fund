package donations

import (
	"boardfund/messaging"
	"boardfund/service/fundevents"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"log/slog"
	"strconv"
	"strings"
)

type Handlers struct {
	donationStore donationStore
	events        eventRecorder

	logger *slog.Logger
}

func NewHandlers(donationStore donationStore, events eventRecorder, logger *slog.Logger) *Handlers {
	return &Handlers{
		donationStore: donationStore,
		events:        events,
		logger:        logger,
	}
}

func (h *Handlers) Subscribe(subscriber subscriber) error {
	var errResult error

	if err := subscriber.Subscribe(messaging.PaymentCompleted, h.paymentSaleCompleted); err != nil {
		errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", messaging.PaymentCompleted, err))
	}

	if err := subscriber.Subscribe(messaging.SubscriptionExpired, h.subscriptionEnded); err != nil {
		errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", messaging.SubscriptionExpired, err))
	}

	if err := subscriber.Subscribe(messaging.SubscriptionCancelled, h.subscriptionEnded); err != nil {
		errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", messaging.SubscriptionCancelled, err))
	}

	return errResult
}

func (h *Handlers) subscriptionEnded(data []byte) {
	var subscriptionEnded SubscriptionEvent
	if err := json.Unmarshal(data, &subscriptionEnded); err != nil {
		h.logger.Error("failed to unmarshal subscription ended event", slog.String("error", err.Error()))

		return
	}

	deactivateSub := DeactivateDonationBySubscription{
		SubscriptionID: subscriptionEnded.ID,
		Reason:         subscriptionEnded.Status,
	}

	donation, err := h.donationStore.SetDonationToInactiveBySubscriptionID(context.Background(), deactivateSub)
	if err != nil {
		h.logger.Error("failed to deactivate donation by subscription id", slog.String("error", err.Error()))

		return
	}

	// No actor: the provider ended this, not a person. OccurredAt is the
	// provider's own timestamp, so the feed reads in the order things actually
	// happened rather than the order we heard about them.
	h.events.Record(context.Background(), fundevents.Record{
		FundID:          donation.FundID,
		Kind:            fundevents.KindDonationCancelled,
		OccurredAt:      subscriptionEnded.StatusUpdateTime,
		SubjectMemberID: &donation.DonorID,
		Detail:          "subscription " + strings.ToLower(subscriptionEnded.Status) + " at provider",
		ReferenceID:     &donation.ID,
	})
}

func (h *Handlers) paymentSaleCompleted(data []byte) {
	var paymentSale PaymentSaleEvent
	if err := json.Unmarshal(data, &paymentSale); err != nil {
		h.logger.Error("failed to unmarshal payment sale event", slog.String("error", err.Error()))

		return
	}

	parentDonation, err := h.donationStore.GetDonationByProviderSubscriptionID(context.Background(), paymentSale.BillingAgreementID)
	if err != nil {
		h.logger.Error("failed to get donation by provider subscription id", slog.String("error", err.Error()))

		return
	}

	if parentDonation == nil {
		h.logger.Error("failed to find donation by provider subscription id", slog.String("provider_subscription_id", paymentSale.BillingAgreementID))

		return
	}

	amountCents, err := dollarStringToCents(paymentSale.Amount.Total)
	if err != nil {
		h.logger.Error("failed to convert dollar amount to cents", slog.String("error", err.Error()))

		return
	}

	feeAmountCents, err := dollarStringToCents(paymentSale.TransactionFee.Value)
	if err != nil {
		h.logger.Error("failed to convert dollar fee amount to cents", slog.String("error", err.Error()))

		return
	}

	insertPayment := InsertDonationPayment{
		ID:                uuid.New(),
		DonationID:        parentDonation.ID,
		ProviderPaymentID: paymentSale.ID,
		AmountCents:       amountCents,
		ProviderFeeCents:  feeAmountCents,
	}

	recorded, err := h.donationStore.InsertDonationPayment(context.Background(), insertPayment)
	if err != nil {
		h.logger.Error("failed to insert donation payment", slog.String("error", err.Error()))

		return
	}

	// Already on record, so this is a redelivery. Returning here keeps the fund
	// event out of the audit trail too: the payment is counted once, and the feed
	// should not show it arriving twice.
	if recorded == nil {
		h.logger.Info("payment already recorded, ignoring redelivery",
			slog.String("provider_payment_id", paymentSale.ID),
		)

		return
	}

	h.events.Record(context.Background(), fundevents.Record{
		FundID:          parentDonation.FundID,
		Kind:            fundevents.KindPaymentReceived,
		OccurredAt:      paymentSale.CreateTime,
		SubjectMemberID: &parentDonation.DonorID,
		AmountCents:     &amountCents,
		Detail:          "recurring",
		ReferenceID:     &parentDonation.ID,
	})
}

func dollarStringToCents(dollarStr string) (int32, error) {
	dollarStr = strings.TrimSpace(dollarStr)

	if dollarStr == "" {
		return 0, fmt.Errorf("input string is empty")
	}

	parts := strings.Split(dollarStr, ".")

	cents := int32(0)

	if len(parts) > 0 {
		dollars, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid dollar amount: %s", dollarStr)
		}
		cents += int32(dollars * 100)
	}

	// Handle the cent part, if present
	if len(parts) > 1 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("invalid cent amount: %s", dollarStr)
		}
		centStr := parts[1]
		// Pad the cent part to ensure it's 2 digits
		if len(centStr) == 1 {
			centStr += "0"
		}
		centPart, err := strconv.Atoi(centStr)
		if err != nil {
			return 0, fmt.Errorf("invalid cent amount: %s", dollarStr)
		}
		cents += int32(centPart)
	}

	return cents, nil
}
