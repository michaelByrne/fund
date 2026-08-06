package donations

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"boardfund/messaging"
	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// fakeDonationStore embeds the interface so the methods a handler has no business
// calling are absent rather than stubbed -- calling one is a nil panic, which is
// the assertion.
type fakeDonationStore struct {
	donationStore

	donation *Donation
	getErr   error

	inserted   *DonationPayment
	insertErr  error
	deactivate *Donation
	deactErr   error

	resumed    *Donation
	resumeErr  error
	resumeCall int

	refunded    *RefundedPayment
	refundErr   error
	refundCents int32
	refundCalls int

	calls int
}

func (f *fakeDonationStore) ReactivateSuspendedDonation(context.Context, string) (*Donation, error) {
	f.resumeCall++

	return f.resumed, f.resumeErr
}

func (f *fakeDonationStore) SetDonationPaymentRefunded(_ context.Context, _ string, cents int32) (*RefundedPayment, error) {
	f.refundCalls++
	f.refundCents = cents

	return f.refunded, f.refundErr
}

func (f *fakeDonationStore) GetDonationByProviderSubscriptionID(context.Context, string) (*Donation, error) {
	return f.donation, f.getErr
}

func (f *fakeDonationStore) InsertDonationPayment(context.Context, InsertDonationPayment) (*DonationPayment, error) {
	f.calls++

	return f.inserted, f.insertErr
}

func (f *fakeDonationStore) SetDonationToInactiveBySubscriptionID(context.Context, DeactivateDonationBySubscription) (*Donation, error) {
	return f.deactivate, f.deactErr
}

type recordedEvents struct {
	records []fundevents.Record
}

func (r *recordedEvents) Record(_ context.Context, record fundevents.Record) {
	r.records = append(r.records, record)
}

func newHandlers(store donationStore, events eventRecorder) *Handlers {
	return NewHandlers(store, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// A returned error naks the message and JetStream hands it back; nil acknowledges
// it and it is gone. Which branch a failure takes is the difference between an
// event that arrives late and one that never arrives, and nothing checked it.
func TestPaymentSaleCompletedDecidesWhatIsWorthRetrying(t *testing.T) {
	fund, donor := uuid.New(), uuid.New()
	donation := &Donation{ID: uuid.New(), FundID: fund, DonorID: donor}

	valid := []byte(`{"id":"SALE-1","billing_agreement_id":"SUB-1",
		"amount":{"total":"10.00"},"transaction_fee":{"value":"0.50"}}`)

	cases := []struct {
		name     string
		store    *fakeDonationStore
		payload  []byte
		wantErr  bool
		because  string
		wantCall int
	}{
		{
			name:     "unparseable payload is discarded",
			store:    &fakeDonationStore{},
			payload:  []byte(`{ not json`),
			wantErr:  false,
			because:  "it will not parse on the fourth delivery either",
			wantCall: 0,
		},
		{
			name:    "a database failure is retried",
			store:   &fakeDonationStore{getErr: errors.New("connection refused")},
			payload: valid,
			wantErr: true,
			because: "the payment is real and the database will come back",
		},
		{
			name:    "a subscription we have not recorded yet is retried",
			store:   &fakeDonationStore{donation: nil},
			payload: valid,
			wantErr: true,
			because: "PayPal can report the first payment before we finish recording the subscription",
		},
		{
			name:     "an unreadable amount is discarded",
			store:    &fakeDonationStore{donation: donation},
			payload:  []byte(`{"id":"S","billing_agreement_id":"SUB-1","amount":{"total":"ten dollars"}}`),
			wantErr:  false,
			because:  "no retry makes 'ten dollars' a number",
			wantCall: 0,
		},
		{
			name:     "a failed insert is retried",
			store:    &fakeDonationStore{donation: donation, insertErr: errors.New("deadlock")},
			payload:  valid,
			wantErr:  true,
			because:  "losing this is losing the money",
			wantCall: 1,
		},
		{
			name:     "an already-recorded payment succeeds",
			store:    &fakeDonationStore{donation: donation, inserted: nil},
			payload:  valid,
			wantErr:  false,
			because:  "a redelivery of something already banked is not a failure",
			wantCall: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := newHandlers(c.store, &recordedEvents{}).paymentSaleCompleted(c.payload)

			if c.wantErr && err == nil {
				t.Errorf("should have been retried: %s", c.because)
			}

			if !c.wantErr && err != nil {
				t.Errorf("should have been acknowledged (%s), got: %v", c.because, err)
			}

			if c.store.calls != c.wantCall {
				t.Errorf("insert called %d times, want %d", c.store.calls, c.wantCall)
			}
		})
	}
}

// The bus is at-least-once, so a payment already on record arrives again as a
// matter of course. Writing the activity entry anyway would show the donor
// paying twice.
func TestARedeliveredPaymentAddsNothingToTheFeed(t *testing.T) {
	donation := &Donation{ID: uuid.New(), FundID: uuid.New(), DonorID: uuid.New()}
	events := &recordedEvents{}

	payload := []byte(`{"id":"SALE-1","billing_agreement_id":"SUB-1",
		"amount":{"total":"10.00"},"transaction_fee":{"value":"0.50"}}`)

	// inserted nil is what the store returns when the provider payment is already
	// on record.
	handlers := newHandlers(&fakeDonationStore{donation: donation, inserted: nil}, events)

	if err := handlers.paymentSaleCompleted(payload); err != nil {
		t.Fatalf("redelivery should succeed: %v", err)
	}

	if len(events.records) != 0 {
		t.Errorf("a redelivery recorded %d events, want 0", len(events.records))
	}
}

// Suspended means the subscription has stopped paying, so it belongs with expired
// and cancelled rather than being ignored, which is what it was.
func TestSuspendedSubscriptionsDeactivateTheDonation(t *testing.T) {
	donation := &Donation{ID: uuid.New(), FundID: uuid.New(), DonorID: uuid.New()}
	store := &fakeDonationStore{deactivate: donation}
	events := &recordedEvents{}

	payload := []byte(`{"id":"SUB-1","status":"SUSPENDED"}`)

	if err := newHandlers(store, events).subscriptionEnded(payload); err != nil {
		t.Fatalf("subscriptionEnded: %v", err)
	}

	if len(events.records) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events.records))
	}

	if got := events.records[0].Kind; got != fundevents.KindDonationCancelled {
		t.Errorf("kind = %s, want %s", got, fundevents.KindDonationCancelled)
	}
}

// A failed payment is not an ended subscription. PayPal retries and most recover,
// so deactivating here would cancel donations it is about to collect.
func TestAFailedPaymentIsRecordedButChangesNothing(t *testing.T) {
	donation := &Donation{ID: uuid.New(), FundID: uuid.New(), DonorID: uuid.New()}

	// deactivate is nil: if the handler tried to deactivate, the assertion below
	// on the recorded kind would catch it.
	store := &fakeDonationStore{donation: donation}
	events := &recordedEvents{}

	payload := []byte(`{"id":"SUB-1","status":"ACTIVE"}`)

	if err := newHandlers(store, events).subscriptionPaymentFailed(payload); err != nil {
		t.Fatalf("subscriptionPaymentFailed: %v", err)
	}

	if len(events.records) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events.records))
	}

	record := events.records[0]

	if record.Kind != fundevents.KindPaymentFailed {
		t.Errorf("kind = %s, want %s", record.Kind, fundevents.KindPaymentFailed)
	}

	if record.Kind == fundevents.KindDonationCancelled {
		t.Error("a failed payment must not read as a cancellation")
	}

	if record.AmountCents != nil {
		t.Error("no money moved, so no amount should be recorded")
	}
}

// Every event type in keys.go that this service is meant to act on has to be
// registered, or it lands in the stream and stays there. Two of these were
// declared and never subscribed for exactly that reason.
func TestSubscribesToEveryDonationEvent(t *testing.T) {
	registered := map[string]bool{}

	err := newHandlers(&fakeDonationStore{}, &recordedEvents{}).Subscribe(subscriberFunc(
		func(event string, _ func([]byte) error) error {
			registered[event] = true

			return nil
		}))
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	for _, event := range []string{
		messaging.PaymentCompleted,
		messaging.SubscriptionExpired,
		messaging.SubscriptionCancelled,
		messaging.SubscriptionSuspended,
		messaging.SubscriptionPaymentFailed,
		messaging.PaymentRefunded,
		messaging.PaymentReversed,
	} {
		if !registered[event] {
			t.Errorf("%s is never subscribed, so it would accumulate in the stream unread", event)
		}
	}
}

type subscriberFunc func(event string, cb func([]byte) error) error

func (f subscriberFunc) Subscribe(event string, cb func(data []byte) error) error {
	return f(event, cb)
}

// PayPal sends no "unsuspended" event, so a payment against a suspended
// subscription is the only evidence it resumed. Without acting on it, suspending
// a donation was permanent: the money kept arriving and being recorded while the
// donation stayed inactive and the donor stopped counting as one.
func TestAPaymentResumesASuspendedDonation(t *testing.T) {
	fund, donor := uuid.New(), uuid.New()
	suspended := &Donation{ID: uuid.New(), FundID: fund, DonorID: donor, Active: false}

	payload := []byte(`{"id":"SALE-9","billing_agreement_id":"SUB-9",
		"amount":{"total":"10.00"},"transaction_fee":{"value":"0.50"}}`)

	t.Run("reactivates and says so in the feed", func(t *testing.T) {
		store := &fakeDonationStore{
			donation: suspended,
			resumed:  &Donation{ID: suspended.ID, FundID: fund, DonorID: donor, Active: true},
			inserted: &DonationPayment{},
		}
		events := &recordedEvents{}

		if err := newHandlers(store, events).paymentSaleCompleted(payload); err != nil {
			t.Fatalf("paymentSaleCompleted: %v", err)
		}

		var resumedRecords int
		for _, record := range events.records {
			if record.Kind == fundevents.KindDonationResumed {
				resumedRecords++
			}
		}

		if resumedRecords != 1 {
			t.Errorf("recorded %d resumed events, want 1", resumedRecords)
		}

		// The payment is still recorded: the money arrived either way.
		if store.calls != 1 {
			t.Errorf("payment insert called %d times, want 1", store.calls)
		}
	})

	t.Run("an active donation is left alone", func(t *testing.T) {
		active := &Donation{ID: uuid.New(), FundID: fund, DonorID: donor, Active: true}
		store := &fakeDonationStore{donation: active, inserted: &DonationPayment{}}

		if err := newHandlers(store, &recordedEvents{}).paymentSaleCompleted(payload); err != nil {
			t.Fatalf("paymentSaleCompleted: %v", err)
		}

		if store.resumeCall != 0 {
			t.Error("an active donation should not be reactivated")
		}
	})

	t.Run("a cancellation the payment must not overturn stays closed", func(t *testing.T) {
		// nil resumed is what the store returns when the donation was cancelled by
		// the member, or closed with its fund: the query matches only suspensions.
		store := &fakeDonationStore{donation: suspended, resumed: nil, inserted: &DonationPayment{}}
		events := &recordedEvents{}

		if err := newHandlers(store, events).paymentSaleCompleted(payload); err != nil {
			t.Fatalf("paymentSaleCompleted: %v", err)
		}

		for _, record := range events.records {
			if record.Kind == fundevents.KindDonationResumed {
				t.Error("a donation nobody wants running was reported as resumed")
			}
		}

		// Still banked. The money arrived whatever the donation's state.
		if store.calls != 1 {
			t.Errorf("payment insert called %d times, want 1", store.calls)
		}
	})

	t.Run("a failed reactivation is retried", func(t *testing.T) {
		store := &fakeDonationStore{donation: suspended, resumeErr: errors.New("deadlock")}

		if err := newHandlers(store, &recordedEvents{}).paymentSaleCompleted(payload); err == nil {
			t.Error("a failed reactivation should come back rather than be acknowledged")
		}

		// And the payment is not recorded on that pass, so the retry does both.
		if store.calls != 0 {
			t.Errorf("payment insert called %d times before the reactivation succeeded, want 0", store.calls)
		}
	})
}

// The mechanism is only worth having if the handlers that need it use it. These
// two record unconditionally -- nothing in either reports "already done" the way
// the payment and reactivation paths do -- so a redelivery duplicates the feed
// entry unless a key stops it.
func TestTheUnguardedHandlersSupplyADedupeKey(t *testing.T) {
	donation := &Donation{ID: uuid.New(), FundID: uuid.New(), DonorID: uuid.New()}

	t.Run("a cancellation is keyed on the subscription and its new status", func(t *testing.T) {
		events := &recordedEvents{}
		store := &fakeDonationStore{deactivate: donation}

		err := newHandlers(store, events).subscriptionEnded([]byte(`{"id":"SUB-1","status":"CANCELLED"}`))
		require(t, err)

		key := events.records[0].DedupeKey
		if key == "" {
			t.Fatal("a redelivery would record a second cancellation")
		}

		// Suspension and cancellation of the same subscription are different
		// events and must not collapse into one.
		suspended := &recordedEvents{}
		err = newHandlers(&fakeDonationStore{deactivate: donation}, suspended).
			subscriptionEnded([]byte(`{"id":"SUB-1","status":"SUSPENDED"}`))
		require(t, err)

		if suspended.records[0].DedupeKey == key {
			t.Error("two different subscription outcomes produced the same key")
		}
	})

	t.Run("a failed payment is keyed on when it failed", func(t *testing.T) {
		first := &recordedEvents{}
		err := newHandlers(&fakeDonationStore{donation: donation}, first).
			subscriptionPaymentFailed([]byte(`{"id":"SUB-1","status_update_time":"2026-08-06T10:00:00Z"}`))
		require(t, err)

		if first.records[0].DedupeKey == "" {
			t.Fatal("a redelivery would record a second failure")
		}

		// A second genuine failure a week later is a second event. Keying on the
		// subscription alone would keep the first and swallow every one after it.
		second := &recordedEvents{}
		err = newHandlers(&fakeDonationStore{donation: donation}, second).
			subscriptionPaymentFailed([]byte(`{"id":"SUB-1","status_update_time":"2026-08-13T10:00:00Z"}`))
		require(t, err)

		if second.records[0].DedupeKey == first.records[0].DedupeKey {
			t.Error("two separate failures share a key, so only the first would ever be recorded")
		}
	})
}

func require(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("handler: %v", err)
	}
}

// Nothing was subscribed to refunds or reversals, so money returned to a donor
// went on counting toward the balance the payout planner divides up.
func TestRefundsAreRecordedAgainstTheRightPayment(t *testing.T) {
	refunded := &RefundedPayment{
		PaymentID: uuid.New(), DonationID: uuid.New(),
		FundID: uuid.New(), DonorID: uuid.New(),
		AmountCents: 5000, RefundedCents: 5000,
	}

	t.Run("prefers the running total over this refund's own amount", func(t *testing.T) {
		store := &fakeDonationStore{refunded: refunded}

		// A second partial refund: 10.00 now, 30.00 in all. Recording 10 would
		// leave the fund believing it still holds money it has returned.
		payload := []byte(`{"id":"REF-2","sale_id":"SALE-1",
			"amount":{"total":"10.00"},"total_refunded_amount":{"value":"30.00"}}`)

		if err := newHandlers(store, &recordedEvents{}).paymentRefunded(payload); err != nil {
			t.Fatalf("paymentRefunded: %v", err)
		}

		if store.refundCents != 3000 {
			t.Errorf("recorded %d cents refunded, want 3000", store.refundCents)
		}
	})

	t.Run("a reversal reports the sale in id rather than sale_id", func(t *testing.T) {
		store := &fakeDonationStore{refunded: refunded}

		payload := []byte(`{"id":"SALE-1","amount":{"total":"50.00"}}`)

		if err := newHandlers(store, &recordedEvents{}).paymentRefunded(payload); err != nil {
			t.Fatalf("paymentRefunded: %v", err)
		}

		if store.refundCalls != 1 {
			t.Error("a reversal must still find its payment")
		}
	})

	t.Run("records the money as leaving", func(t *testing.T) {
		events := &recordedEvents{}

		payload := []byte(`{"id":"REF-1","sale_id":"SALE-1","amount":{"total":"50.00"}}`)

		if err := newHandlers(&fakeDonationStore{refunded: refunded}, events).paymentRefunded(payload); err != nil {
			t.Fatalf("paymentRefunded: %v", err)
		}

		if len(events.records) != 1 {
			t.Fatalf("recorded %d events, want 1", len(events.records))
		}

		record := events.records[0]

		if record.Kind != fundevents.KindPaymentRefunded {
			t.Errorf("kind = %s, want %s", record.Kind, fundevents.KindPaymentRefunded)
		}

		// The feed reads as money moving, and this moved out.
		if record.AmountCents == nil || *record.AmountCents >= 0 {
			t.Errorf("amount = %v, want a negative figure", record.AmountCents)
		}
	})

	t.Run("a payment we do not know records nothing", func(t *testing.T) {
		events := &recordedEvents{}
		store := &fakeDonationStore{refunded: nil}

		payload := []byte(`{"id":"REF-1","sale_id":"SOMEBODY-ELSES","amount":{"total":"50.00"}}`)

		if err := newHandlers(store, events).paymentRefunded(payload); err != nil {
			t.Fatalf("an unknown sale is ordinary on a shared account: %v", err)
		}

		if len(events.records) != 0 {
			t.Error("a refund that changed nothing should not appear in the feed")
		}
	})

	t.Run("a database failure is retried", func(t *testing.T) {
		store := &fakeDonationStore{refundErr: errors.New("deadlock")}

		payload := []byte(`{"id":"REF-1","sale_id":"SALE-1","amount":{"total":"50.00"}}`)

		if err := newHandlers(store, &recordedEvents{}).paymentRefunded(payload); err == nil {
			t.Error("losing a refund leaves the fund paying out money it returned")
		}
	})

	t.Run("an unreadable amount is discarded", func(t *testing.T) {
		store := &fakeDonationStore{refunded: refunded}

		payload := []byte(`{"id":"REF-1","sale_id":"SALE-1","amount":{"total":"loads"}}`)

		if err := newHandlers(store, &recordedEvents{}).paymentRefunded(payload); err != nil {
			t.Errorf("no retry makes 'loads' a number: %v", err)
		}

		if store.refundCalls != 0 {
			t.Error("nothing should have been written")
		}
	})
}
