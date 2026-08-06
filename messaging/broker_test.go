package messaging

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// startServer runs the embedded server exactly as the app does, JetStream on disk
// and no listening port, so these tests exercise the real configuration rather
// than a convenient one.
func startServer(t *testing.T, storeDir string) *nats.Conn {
	t.Helper()

	ns, err := server.NewServer(&server.Options{
		DontListen: true,
		JetStream:  true,
		StoreDir:   storeDir,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("server not ready")
	}

	nc, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
	})

	return nc
}

func newBroker(t *testing.T, nc *nats.Conn) *Broker {
	t.Helper()

	broker, err := NewBroker(context.Background(), nc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}

	t.Cleanup(broker.Close)

	// Real delays start at five seconds, which is right in production and useless
	// in a test. The schedule itself is asserted separately.
	broker.backOff = []time.Duration{
		20 * time.Millisecond,
		40 * time.Millisecond,
		80 * time.Millisecond,
	}

	return broker
}

// The whole point of the change: an event published while nothing is listening
// must still be delivered once a consumer appears. Core NATS dropped it on the
// floor, which is how a webhook the handler had already answered 200 to could
// vanish.
func TestEventsPublishedBeforeAnyoneIsListeningAreStillDelivered(t *testing.T) {
	broker := newBroker(t, startServer(t, t.TempDir()))

	if err := broker.Publish(PaymentCompleted, []byte(`{"id":"SALE-1"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	received := make(chan []byte, 1)
	if err := broker.Subscribe(PaymentCompleted, func(data []byte) error {
		received <- data

		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != `{"id":"SALE-1"}` {
			t.Errorf("payload = %s", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("an event published before subscribing was never delivered")
	}
}

// A handler that fails used to log and return, and the event was gone -- while
// PayPal had been told it was handled. Now the failure is a nak.
func TestAFailedHandlerGetsTheMessageAgain(t *testing.T) {
	broker := newBroker(t, startServer(t, t.TempDir()))

	var mu sync.Mutex
	attempts := 0
	succeeded := make(chan struct{})

	err := broker.Subscribe(PaymentCompleted, func([]byte) error {
		mu.Lock()
		defer mu.Unlock()

		attempts++
		if attempts == 1 {
			return errNotToday
		}

		close(succeeded)

		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err = broker.Publish(PaymentCompleted, []byte(`{"id":"SALE-2"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-succeeded:
	case <-time.After(30 * time.Second):
		mu.Lock()
		defer mu.Unlock()

		t.Fatalf("a failed handler was never retried, attempts = %d", attempts)
	}
}

// Durability across a restart is the reason for the volume. The stream lives in
// the store directory, so a new server over the same directory must still hold
// the event -- and the durable consumer must not have silently acknowledged it.
func TestUnhandledEventsSurviveARestart(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "jetstream")

	func() {
		broker := newBroker(t, startServer(t, storeDir))

		if err := broker.Publish(SubscriptionCancelled, []byte(`{"id":"SUB-1"}`)); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}()

	// A second process over the same volume, which is what a deploy produces.
	broker := newBroker(t, startServer(t, storeDir))

	received := make(chan []byte, 1)
	if err := broker.Subscribe(SubscriptionCancelled, func(data []byte) error {
		received <- data

		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != `{"id":"SUB-1"}` {
			t.Errorf("payload = %s", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("an event published before a restart did not survive it")
	}
}

// Durable names may not contain dots or wildcards, and every PayPal event type
// is dotted. A name JetStream rejects would fail at Subscribe, taking the whole
// startup with it.
func TestDurableNamesAreAcceptable(t *testing.T) {
	for _, event := range []string{
		PaymentCompleted,
		SubscriptionCancelled,
		PayoutsItemSucceeded,
		PayoutsBatchSuccess,
	} {
		name := durableName(event)

		for _, bad := range []string{".", "*", ">", " "} {
			if contains(name, bad) {
				t.Errorf("durable name %q for %q contains %q", name, event, bad)
			}
		}

		if name == "" {
			t.Errorf("durable name for %q is empty", event)
		}
	}

	// Distinct events must not collapse into one consumer, which would have two
	// event types sharing a cursor.
	if durableName(PayoutsItemSucceeded) == durableName(PayoutsItemFailed) {
		t.Error("two event types produced the same durable name")
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

type constError string

func (e constError) Error() string { return string(e) }

const errNotToday = constError("not today")

// A plain Nak asks for the message back immediately. That turns a handler failing
// because the database is down into a tight redelivery loop that exhausts the
// delivery budget in milliseconds -- retrying hardest exactly when the system can
// least afford it.
func TestRedeliveryBacksOffInsteadOfSpinning(t *testing.T) {
	broker := newBroker(t, startServer(t, t.TempDir()))
	broker.backOff = []time.Duration{
		300 * time.Millisecond,
		600 * time.Millisecond,
	}

	var mu sync.Mutex
	var times []time.Time
	done := make(chan struct{})

	err := broker.Subscribe(PaymentCompleted, func([]byte) error {
		mu.Lock()
		defer mu.Unlock()

		times = append(times, time.Now())
		if len(times) == 2 {
			close(done)
		}

		return errNotToday
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err = broker.Publish(PaymentCompleted, []byte(`{"id":"SALE-3"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("never redelivered")
	}

	mu.Lock()
	defer mu.Unlock()

	if gap := times[1].Sub(times[0]); gap < 250*time.Millisecond {
		t.Errorf("redelivered after %v, want at least the first backoff of 300ms", gap)
	}
}

// The admin page is only worth having if the numbers on it move. Redelivered is
// the one that matters: it is zero on a healthy consumer, and non-zero is the
// first sign a handler is failing.
func TestStatusReportsWhatIsStuck(t *testing.T) {
	broker := newBroker(t, startServer(t, t.TempDir()))

	ctx := context.Background()

	failing := make(chan struct{})
	var once sync.Once

	err := broker.Subscribe(PaymentCompleted, func([]byte) error {
		once.Do(func() { close(failing) })

		return errNotToday
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err = broker.Publish(PaymentCompleted, []byte(`{"id":"SALE-4"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	<-failing

	// Give the nak time to land before asking.
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, errStatus := broker.Status(ctx)
		if errStatus != nil {
			t.Fatalf("status: %v", errStatus)
		}

		if status.Stream.Messages != 1 {
			t.Fatalf("stream should hold the message, got %d", status.Stream.Messages)
		}

		if len(status.Consumers) != 1 {
			t.Fatalf("expected one consumer, got %d", len(status.Consumers))
		}

		consumer := status.Consumers[0]

		// The durable name is unreadable; the page shows the event type.
		if consumer.Subject != PaymentCompleted {
			t.Errorf("consumer subject = %q, want %q", consumer.Subject, PaymentCompleted)
		}

		if consumer.Redelivered > 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("redelivered stayed at 0 while the handler kept failing: %+v", consumer)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// A healthy stream reports nothing alarming, so the page is readable at a glance
// rather than always faintly red.
func TestStatusIsQuietWhenNothingIsWrong(t *testing.T) {
	broker := newBroker(t, startServer(t, t.TempDir()))

	handled := make(chan struct{})
	if err := broker.Subscribe(SubscriptionCancelled, func([]byte) error {
		close(handled)

		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := broker.Publish(SubscriptionCancelled, []byte(`{"id":"SUB-2"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	<-handled

	deadline := time.Now().Add(10 * time.Second)
	for {
		status, err := broker.Status(context.Background())
		if err != nil {
			t.Fatalf("status: %v", err)
		}

		if len(status.Consumers) == 1 && status.Consumers[0].Pending == 0 &&
			status.Consumers[0].AckPending == 0 && status.Consumers[0].Redelivered == 0 {
			if len(status.Exhausted) != 0 {
				t.Errorf("nothing should have been given up on, got %d", len(status.Exhausted))
			}

			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("a handled message left the consumer busy: %+v", status.Consumers)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// Running out of deliveries is where an event finally stops. Without the
// advisory it is one log line at the moment it happens and nothing afterwards.
func TestExhaustedMessagesAreRecorded(t *testing.T) {
	broker := newBroker(t, startServer(t, t.TempDir()))
	broker.backOff = []time.Duration{10 * time.Millisecond}

	if err := broker.Subscribe(PayoutsItemFailed, func([]byte) error {
		return errNotToday
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := broker.Publish(PayoutsItemFailed, []byte(`{"payout_item_id":"PI-1"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		status, err := broker.Status(context.Background())
		if err != nil {
			t.Fatalf("status: %v", err)
		}

		if len(status.Exhausted) > 0 {
			if got := status.Exhausted[0].Deliveries; got < 2 {
				t.Errorf("deliveries = %d, want at least 2", got)
			}

			return
		}

		if time.Now().After(deadline) {
			t.Fatal("a message that used up every delivery was never recorded")
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// The advisory subscription is not one of the JetStream consumers, so stopping
// those does not stop it. Left running it can fire after Close and append to
// state nothing owns any more.
func TestCloseStopsTheAdvisorySubscription(t *testing.T) {
	broker, err := NewBroker(context.Background(), startServer(t, t.TempDir()),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}

	if broker.advisories == nil {
		t.Fatal("the broker should be watching for exhausted messages")
	}

	if !broker.advisories.IsValid() {
		t.Fatal("the advisory subscription should be live before Close")
	}

	broker.Close()

	if broker.advisories.IsValid() {
		t.Error("the advisory subscription outlived Close")
	}
}
