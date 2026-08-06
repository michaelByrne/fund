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

	calls int
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
