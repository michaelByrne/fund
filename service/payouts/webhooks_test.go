package payouts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"boardfund/messaging"

	"github.com/google/uuid"
)

// fakePayoutStore embeds the interface so methods a handler has no business
// calling are absent rather than stubbed -- calling one is a nil panic, which is
// the assertion.
type fakePayoutStore struct {
	payoutStore

	resultErr error
	byItemErr error
	batch     *Batch
	batchErr  error
	statusErr error

	resultCalls int
	byItemCalls int
}

func (f *fakePayoutStore) SetPayoutResult(context.Context, SetPayoutResult) (*Payout, error) {
	f.resultCalls++

	return &Payout{}, f.resultErr
}

func (f *fakePayoutStore) SetPayoutStatusByProviderItemID(context.Context, SetPayoutStatusByItem) (*Payout, error) {
	f.byItemCalls++

	return &Payout{}, f.byItemErr
}

func (f *fakePayoutStore) GetBatchBySenderBatchID(context.Context, uuid.UUID) (*Batch, error) {
	return f.batch, f.batchErr
}

func (f *fakePayoutStore) SetBatchStatus(context.Context, SetBatchStatus) (*Batch, error) {
	return &Batch{}, f.statusErr
}

func newPayoutHandlers(store payoutStore) *Handlers {
	return NewHandlers(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// A returned error naks the message and JetStream hands it back; nil acknowledges
// it and it is gone for good. These payouts are money that has already left, so
// which branch a failure takes decides whether its outcome is ever recorded.
func TestPayoutItemUpdatedDecidesWhatIsWorthRetrying(t *testing.T) {
	ours := uuid.New()

	withSender := []byte(`{"payout_item_id":"PI-1","transaction_status":"SUCCESS",
		"payout_item_fee":{"value":"0.25"},"payout_item":{"sender_item_id":"` + ours.String() + `"}}`)

	// No sender_item_id, which is how an older batch arrives before it has been
	// reconciled at least once.
	withoutSender := []byte(`{"payout_item_id":"PI-2","transaction_status":"FAILED",
		"payout_item_fee":{"value":"0.00"}}`)

	cases := []struct {
		name       string
		store      *fakePayoutStore
		payload    []byte
		wantErr    bool
		because    string
		wantResult int
		wantByItem int
	}{
		{
			name:    "unparseable payload is discarded",
			store:   &fakePayoutStore{},
			payload: []byte(`{{{`),
			because: "it will not parse on the fourth delivery either",
		},
		{
			name:    "an event with no item id is discarded",
			store:   &fakePayoutStore{},
			payload: []byte(`{"transaction_status":"SUCCESS"}`),
			because: "there is nothing to match it against, now or later",
		},
		{
			name:       "matched on sender item id",
			store:      &fakePayoutStore{},
			payload:    withSender,
			because:    "our own id is present from submission, unlike the provider's",
			wantResult: 1,
		},
		{
			name:       "a failed write against our id is retried",
			store:      &fakePayoutStore{resultErr: errors.New("deadlock")},
			payload:    withSender,
			wantErr:    true,
			because:    "the payout has already left; losing its outcome loses the reconciliation",
			wantResult: 1,
		},
		{
			name:       "falls back to the provider id",
			store:      &fakePayoutStore{},
			payload:    withoutSender,
			because:    "resolvable once the batch has been reconciled once",
			wantByItem: 1,
		},
		{
			name:       "a failed fallback write is retried",
			store:      &fakePayoutStore{byItemErr: errors.New("timeout")},
			payload:    withoutSender,
			wantErr:    true,
			because:    "the reconciler is a backstop, not a reason to drop this",
			wantByItem: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := newPayoutHandlers(c.store).payoutItemUpdated(c.payload)

			if c.wantErr && err == nil {
				t.Errorf("should have been retried: %s", c.because)
			}

			if !c.wantErr && err != nil {
				t.Errorf("should have been acknowledged (%s), got: %v", c.because, err)
			}

			if c.store.resultCalls != c.wantResult {
				t.Errorf("SetPayoutResult called %d times, want %d", c.store.resultCalls, c.wantResult)
			}

			if c.store.byItemCalls != c.wantByItem {
				t.Errorf("SetPayoutStatusByProviderItemID called %d times, want %d", c.store.byItemCalls, c.wantByItem)
			}
		})
	}
}

func TestPayoutBatchUpdatedDecidesWhatIsWorthRetrying(t *testing.T) {
	ours := uuid.New()

	oursPayload := []byte(`{"batch_header":{"payout_batch_id":"PB-1","batch_status":"SUCCESS",
		"sender_batch_header":{"sender_batch_id":"` + ours.String() + `"}}}`)

	cases := []struct {
		name    string
		store   *fakePayoutStore
		payload []byte
		wantErr bool
		because string
	}{
		{
			name:    "unparseable payload is discarded",
			store:   &fakePayoutStore{},
			payload: []byte(`nope`),
			because: "it will not parse later either",
		},
		{
			name:    "a batch with no sender id is discarded",
			store:   &fakePayoutStore{},
			payload: []byte(`{"batch_header":{"batch_status":"SUCCESS"}}`),
			because: "nothing to match it against",
		},
		{
			name:    "somebody else's batch is discarded",
			store:   &fakePayoutStore{},
			payload: []byte(`{"batch_header":{"sender_batch_header":{"sender_batch_id":"not-a-uuid"}}}`),
			because: "the same PayPal account may send payouts we did not originate",
		},
		{
			name:    "a failed lookup is retried",
			store:   &fakePayoutStore{batchErr: errors.New("connection reset")},
			payload: oursPayload,
			wantErr: true,
			because: "the batch is ours and the database will come back",
		},
		{
			name:    "a failed status write is retried",
			store:   &fakePayoutStore{batch: &Batch{ID: uuid.New()}, statusErr: errors.New("deadlock")},
			payload: oursPayload,
			wantErr: true,
			because: "a batch stuck in the wrong status is a batch somebody chases",
		},
		{
			name:    "a batch we know about succeeds",
			store:   &fakePayoutStore{batch: &Batch{ID: uuid.New()}},
			payload: oursPayload,
			because: "the ordinary path",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := newPayoutHandlers(c.store).payoutBatchUpdated(c.payload)

			if c.wantErr && err == nil {
				t.Errorf("should have been retried: %s", c.because)
			}

			if !c.wantErr && err != nil {
				t.Errorf("should have been acknowledged (%s), got: %v", c.because, err)
			}
		})
	}
}

// Every payout event PayPal sends has to be registered, or it lands in the stream
// and stays there while a payout's outcome goes unrecorded.
func TestSubscribesToEveryPayoutEvent(t *testing.T) {
	registered := map[string]bool{}

	err := newPayoutHandlers(&fakePayoutStore{}).Subscribe(payoutSubscriberFunc(
		func(event string, _ func([]byte) error) error {
			registered[event] = true

			return nil
		}))
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	for _, event := range []string{
		messaging.PayoutsItemSucceeded, messaging.PayoutsItemFailed, messaging.PayoutsItemBlocked,
		messaging.PayoutsItemCanceled, messaging.PayoutsItemDenied, messaging.PayoutsItemHeld,
		messaging.PayoutsItemRefunded, messaging.PayoutsItemReturned, messaging.PayoutsItemUnclaimed,
		messaging.PayoutsBatchSuccess, messaging.PayoutsBatchDenied, messaging.PayoutsBatchProcessing,
	} {
		if !registered[event] {
			t.Errorf("%s is never subscribed, so a payout outcome would go unrecorded", event)
		}
	}
}

type payoutSubscriberFunc func(event string, cb func([]byte) error) error

func (f payoutSubscriberFunc) Subscribe(event string, cb func(data []byte) error) error {
	return f(event, cb)
}
