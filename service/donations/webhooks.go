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
	"time"
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

	// Suspended joins expired and cancelled. PayPal suspends a subscription that
	// has stopped paying, and a donation nobody is paying into is not an active
	// donation whatever the provider may do with it later. If it resumes, the next
	// payment webhook is the evidence for turning it back on -- which nothing does
	// yet, and is the one thing to watch for here.
	if err := subscriber.Subscribe(messaging.SubscriptionSuspended, h.subscriptionEnded); err != nil {
		errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", messaging.SubscriptionSuspended, err))
	}

	// Money coming back. Refund and reversal are the same thing to a balance --
	// one returned by us, one taken back by the donor's bank -- and neither was
	// subscribed, so a refunded donation went on being paid out.
	for _, event := range []string{messaging.PaymentRefunded, messaging.PaymentReversed} {
		if err := subscriber.Subscribe(event, h.paymentRefunded); err != nil {
			errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", event, err))
		}
	}

	// A failed payment is not an ended subscription. PayPal retries, and most
	// recover, so this records and changes nothing.
	if err := subscriber.Subscribe(messaging.SubscriptionPaymentFailed, h.subscriptionPaymentFailed); err != nil {
		errResult = multierror.Append(err, fmt.Errorf("failed to subscribe to %s: %w", messaging.SubscriptionPaymentFailed, err))
	}

	return errResult
}

// paymentRefunded records money returned to the donor or taken back by their
// bank.
//
// The fund balance is what the planner divides between enrollees, so a refund
// that is not subtracted leaves the fund paying out money it no longer holds --
// from a PayPal balance shared with every other fund. The payment row survives:
// it happened, and an audit trail that erases what it reverses is not one.
func (h *Handlers) paymentRefunded(data []byte) error {
	var event RefundEvent
	if err := json.Unmarshal(data, &event); err != nil {
		h.logger.Error("discarding unparseable refund event", slog.String("error", err.Error()))

		return nil
	}

	paymentID := event.PaymentID()
	if paymentID == "" {
		h.logger.Error("discarding refund event with no payment to attribute it to")

		return nil
	}

	refundedCents, err := dollarStringToCents(event.RefundedTotal())
	if err != nil {
		h.logger.Error("discarding refund with an unreadable amount",
			slog.String("amount", event.RefundedTotal()),
			slog.String("error", err.Error()),
		)

		return nil
	}

	refunded, err := h.donationStore.SetDonationPaymentRefunded(context.Background(), paymentID, refundedCents)
	if err != nil {
		return fmt.Errorf("failed to record refund for payment %s: %w", paymentID, err)
	}

	// Nil means the payment is unknown to us, or already carries this refunded
	// total. The first is somebody else's sale on a shared account; the second is
	// a redelivery. Neither is a failure, and neither should record an event.
	if refunded == nil {
		h.logger.Info("refund recorded nothing",
			slog.String("provider_payment_id", paymentID),
		)

		return nil
	}

	h.logger.Info("recorded a refund against a payment",
		slog.String("provider_payment_id", paymentID),
		slog.Int("refunded_cents", int(refundedCents)),
	)

	// Negative, because the feed reads as money moving and this moved out. The
	// amount is what came back in this refund, not the running total: a second
	// partial refund would otherwise report the whole refunded sum again.
	amount := -refunded.NewlyRefundedCents()

	h.events.Record(context.Background(), fundevents.Record{
		FundID:          refunded.FundID,
		Kind:            fundevents.KindPaymentRefunded,
		OccurredAt:      event.CreateTime,
		SubjectMemberID: &refunded.DonorID,
		AmountCents:     &amount,
		Detail:          "refunded at provider",
		ReferenceID:     &refunded.DonationID,
	})

	return nil
}

// subscriptionPaymentFailed records a charge that did not go through.
//
// Deliberately no state change. Deactivating on one failed payment would cancel
// donations that PayPal is about to collect successfully on its own retry, and
// the fund would stop counting money it is still receiving. What matters is that
// somebody can see a run of them against one donor, which is what the feed is
// for.
func (h *Handlers) subscriptionPaymentFailed(data []byte) error {
	var event SubscriptionEvent
	if err := json.Unmarshal(data, &event); err != nil {
		h.logger.Error("discarding unparseable payment failed event", slog.String("error", err.Error()))

		return nil
	}

	donation, err := h.donationStore.GetDonationByProviderSubscriptionID(context.Background(), event.ID)
	if err != nil {
		return fmt.Errorf("failed to get donation by provider subscription id: %w", err)
	}

	if donation == nil {
		// Not ours, or not recorded yet. Retried on the same reasoning as a
		// payment: the subscription may still be being written.
		return fmt.Errorf("no donation for provider subscription %s yet", event.ID)
	}

	// This handler changes nothing, so there is no state to consult about whether
	// it has run before. The provider's own update time separates one failure from
	// the next; keying on the subscription alone would record the first failure
	// and silently swallow every one after it.
	h.events.Record(context.Background(), fundevents.Record{
		FundID:          donation.FundID,
		Kind:            fundevents.KindPaymentFailed,
		OccurredAt:      event.StatusUpdateTime,
		SubjectMemberID: &donation.DonorID,
		Detail:          "payment failed at provider",
		ReferenceID:     &donation.ID,
		DedupeKey: "payment-failed:" + event.ID + ":" +
			event.StatusUpdateTime.UTC().Format(time.RFC3339Nano),
	})

	return nil
}

func (h *Handlers) subscriptionEnded(data []byte) error {
	var subscriptionEnded SubscriptionEvent
	if err := json.Unmarshal(data, &subscriptionEnded); err != nil {
		// Nothing will make this parse on a second attempt, so it is acknowledged
		// rather than redelivered until the consumer gives up on it.
		h.logger.Error("discarding unparseable subscription ended event", slog.String("error", err.Error()))

		return nil
	}

	deactivateSub := DeactivateDonationBySubscription{
		SubscriptionID: subscriptionEnded.ID,
		Reason:         subscriptionEnded.Status,
	}

	donation, err := h.donationStore.SetDonationToInactiveBySubscriptionID(context.Background(), deactivateSub)
	if err != nil {
		return fmt.Errorf("failed to deactivate donation by subscription id: %w", err)
	}

	// No actor: the provider ended this, not a person. OccurredAt is the
	// provider's own timestamp, so the feed reads in the order things actually
	// happened rather than the order we heard about them.
	//
	// Keyed on the subscription and the status it moved to, from the same helper
	// the local cancellation paths use. When we asked for the cancellation, this
	// webhook is the provider echoing it back and the key discards it -- the feed
	// showed one action twice, once as "cancelled by donor" and once as
	// "subscription cancelled at provider".
	h.events.Record(context.Background(), fundevents.Record{
		FundID:          donation.FundID,
		Kind:            fundevents.KindDonationCancelled,
		OccurredAt:      subscriptionEnded.StatusUpdateTime,
		SubjectMemberID: &donation.DonorID,
		Detail:          "subscription " + strings.ToLower(subscriptionEnded.Status) + " at provider",
		ReferenceID:     &donation.ID,
		DedupeKey:       subscriptionEndedKey(subscriptionEnded.ID, subscriptionEnded.Status),
	})

	return nil
}

func (h *Handlers) paymentSaleCompleted(data []byte) error {
	var paymentSale PaymentSaleEvent
	if err := json.Unmarshal(data, &paymentSale); err != nil {
		h.logger.Error("discarding unparseable payment sale event", slog.String("error", err.Error()))

		return nil
	}

	parentDonation, err := h.donationStore.GetDonationByProviderSubscriptionID(context.Background(), paymentSale.BillingAgreementID)
	if err != nil {
		return fmt.Errorf("failed to get donation by provider subscription id: %w", err)
	}

	if parentDonation == nil {
		// Retried rather than dropped: PayPal can report the first payment before
		// the browser has finished telling us the subscription exists, and the
		// money is real either way.
		return fmt.Errorf("no donation for provider subscription %s yet", paymentSale.BillingAgreementID)
	}

	// A payment against a suspended subscription is the provider telling us it
	// resumed, and it is the only such evidence we get -- PayPal sends no
	// "unsuspended" event. Without this, suspending a donation was permanent:
	// the money kept arriving and being recorded, while the donation stayed
	// inactive and the donor stopped counting as one.
	//
	// Before the payment is recorded rather than after, so that a delivery which
	// records the payment and then fails still reactivates on its retry. The
	// other order leaves the redelivery returning early on the duplicate payment,
	// having never reached this.
	if !parentDonation.Active {
		resumed, errResume := h.donationStore.ReactivateSuspendedDonation(context.Background(), paymentSale.BillingAgreementID)
		if errResume != nil {
			return fmt.Errorf("failed to reactivate suspended donation: %w", errResume)
		}

		// nil means there was nothing to bring back: cancelled by the member, or
		// closed with its fund. Those stay closed, and the payment is still
		// recorded below, because the money did arrive.
		if resumed != nil {
			h.logger.Info("reactivated a suspended donation after a payment",
				slog.String("donation_id", resumed.ID.String()),
			)

			h.events.Record(context.Background(), fundevents.Record{
				FundID:          resumed.FundID,
				Kind:            fundevents.KindDonationResumed,
				OccurredAt:      paymentSale.CreateTime,
				SubjectMemberID: &resumed.DonorID,
				Detail:          "payment received after suspension",
				ReferenceID:     &resumed.ID,
			})
		}
	}

	amountCents, err := dollarStringToCents(paymentSale.Amount.Total)
	if err != nil {
		h.logger.Error("discarding payment with an unreadable amount",
			slog.String("amount", paymentSale.Amount.Total),
			slog.String("error", err.Error()),
		)

		return nil
	}

	feeAmountCents, err := dollarStringToCents(paymentSale.TransactionFee.Value)
	if err != nil {
		h.logger.Error("discarding payment with an unreadable fee",
			slog.String("fee", paymentSale.TransactionFee.Value),
			slog.String("error", err.Error()),
		)

		return nil
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
		return fmt.Errorf("failed to insert donation payment: %w", err)
	}

	// Already on record, so this is a redelivery. Returning here keeps the fund
	// event out of the audit trail too: the payment is counted once, and the feed
	// should not show it arriving twice.
	if recorded == nil {
		h.logger.Info("payment already recorded, ignoring redelivery",
			slog.String("provider_payment_id", paymentSale.ID),
		)

		return nil
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

	return nil
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
