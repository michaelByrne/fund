package donations

import (
	"boardfund/events"
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

	insertPayment := InsertDonationPayment{
		ID:                uuid.New(),
		DonationID:        parentDonation.ID,
		ProviderPaymentID: paymentSale.ID,
		AmountCents:       amountCents,
	}

	_, err = h.donationStore.InsertDonationPayment(context.Background(), insertPayment)
	if err != nil {
		h.logger.Error("failed to insert donation payment", slog.String("error", err.Error()))
	}
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
