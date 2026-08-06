package finance

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// fakeStore embeds the interface so a method the backfill has no business calling
// is absent rather than stubbed, and calling one panics.
type fakeStore struct {
	donationStore

	// asked records the frequencies the reconciler looked for funds under.
	asked []string

	inserted  []donations.InsertDonationPayment
	insertErr error
	// conflict makes the insert report "already recorded", which is what the
	// unique index on the provider payment id does for a payment a webhook
	// delivered while this run was in flight.
	conflict bool
}

func (f *fakeStore) InsertDonationPayment(_ context.Context, payment donations.InsertDonationPayment) (*donations.DonationPayment, error) {
	if f.insertErr != nil {
		return nil, f.insertErr
	}

	f.inserted = append(f.inserted, payment)

	if f.conflict {
		return nil, nil
	}

	return &donations.DonationPayment{ID: payment.ID}, nil
}

func (f *fakeStore) GetActiveFunds(_ context.Context, frequency string) ([]donations.Fund, error) {
	f.asked = append(f.asked, frequency)

	return nil, nil
}

type fakeProvider struct {
	paymentsProvider

	transactions []ProviderTransaction
	err          error
}

func (f *fakeProvider) GetTransactionsForDonationSubscription(context.Context, string) ([]ProviderTransaction, error) {
	return f.transactions, f.err
}

type capturedEvents struct {
	records []fundevents.Record
}

func (c *capturedEvents) Record(_ context.Context, record fundevents.Record) {
	c.records = append(c.records, record)
}

func newService(store donationStore, provider paymentsProvider, events eventRecorder) FinanceService {
	return FinanceService{
		donationStore:    store,
		paymentsProvider: provider,
		events:           events,
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// The reconciler walked the payments we already held and asked the provider about
// each, so it could report a payment that looked wrong and could not notice one
// that never arrived -- which is exactly what a lost webhook leaves behind.
func TestBackfillRecordsPaymentsTheProviderHasAndWeDoNot(t *testing.T) {
	donation := donations.Donation{
		ID: uuid.New(), FundID: uuid.New(), DonorID: uuid.New(),
		ProviderSubscriptionID: "SUB-1",
	}

	when := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)

	t.Run("records only what is missing", func(t *testing.T) {
		provider := &fakeProvider{transactions: []ProviderTransaction{
			{ProviderPaymentID: "SALE-1", Status: "COMPLETED", AmountCents: 1000, FeeCents: 50, Date: when},
			{ProviderPaymentID: "SALE-2", Status: "COMPLETED", AmountCents: 2000, FeeCents: 60, Date: when},
		}}
		store := &fakeStore{}
		events := &capturedEvents{}

		// SALE-1 is already ours; only SALE-2 went missing.
		known := []donations.DonationPayment{{ProviderPaymentID: "SALE-1"}}

		recovered := newService(store, provider, events).
			backfillMissingPayments(context.Background(), donation, known)

		if recovered != 1 {
			t.Fatalf("recovered %d, want 1", recovered)
		}

		if len(store.inserted) != 1 || store.inserted[0].ProviderPaymentID != "SALE-2" {
			t.Fatalf("inserted %+v, want only SALE-2", store.inserted)
		}

		// The provider's figures, not a guess.
		if got := store.inserted[0].AmountCents; got != 2000 {
			t.Errorf("amount = %d, want 2000", got)
		}

		if got := store.inserted[0].ProviderFeeCents; got != 60 {
			t.Errorf("fee = %d, want 60", got)
		}

		if len(events.records) != 1 {
			t.Fatalf("recorded %d events, want 1", len(events.records))
		}

		// Keyed, so a second reconciliation run does not write a second entry for
		// money that was recovered once.
		if events.records[0].DedupeKey == "" {
			t.Error("a recovered payment needs a dedupe key or every run re-reports it")
		}

		if events.records[0].OccurredAt != when {
			t.Error("the feed should read in the order things happened at the provider")
		}
	})

	t.Run("ignores money that has not settled", func(t *testing.T) {
		provider := &fakeProvider{transactions: []ProviderTransaction{
			{ProviderPaymentID: "SALE-3", Status: "PENDING", AmountCents: 1000},
			{ProviderPaymentID: "SALE-4", Status: "DENIED", AmountCents: 1000},
			{ProviderPaymentID: "SALE-5", Status: "", AmountCents: 1000},
		}}
		store := &fakeStore{}

		// A pending or failed transaction is not money the fund can pay out, and
		// recording it would inflate the balance the planner divides up.
		if recovered := newService(store, provider, &capturedEvents{}).
			backfillMissingPayments(context.Background(), donation, nil); recovered != 0 {
			t.Errorf("recovered %d unsettled transactions, want 0", recovered)
		}

		if len(store.inserted) != 0 {
			t.Errorf("inserted %+v, want nothing", store.inserted)
		}
	})

	t.Run("a payment recorded by a webhook mid-run is not double counted", func(t *testing.T) {
		provider := &fakeProvider{transactions: []ProviderTransaction{
			{ProviderPaymentID: "SALE-6", Status: "COMPLETED", AmountCents: 1000},
		}}
		// conflict: the unique index caught it, so a webhook got there first.
		store := &fakeStore{conflict: true}
		events := &capturedEvents{}

		recovered := newService(store, provider, events).
			backfillMissingPayments(context.Background(), donation, nil)

		if recovered != 0 {
			t.Errorf("recovered %d, want 0 -- the payment was already ours", recovered)
		}

		if len(events.records) != 0 {
			t.Error("a payment we did not actually recover should not appear in the feed")
		}
	})

	t.Run("an unreadable subscription does not stop the run", func(t *testing.T) {
		provider := &fakeProvider{err: errors.New("provider timeout")}

		// One subscription the provider will not talk about must not prevent every
		// other fund from being reconciled.
		if recovered := newService(&fakeStore{}, provider, &capturedEvents{}).
			backfillMissingPayments(context.Background(), donation, nil); recovered != 0 {
			t.Errorf("recovered %d, want 0", recovered)
		}
	})

	t.Run("a failed insert does not abandon the rest", func(t *testing.T) {
		provider := &fakeProvider{transactions: []ProviderTransaction{
			{ProviderPaymentID: "SALE-7", Status: "COMPLETED", AmountCents: 1000},
			{ProviderPaymentID: "SALE-8", Status: "COMPLETED", AmountCents: 2000},
		}}
		store := &fakeStore{insertErr: errors.New("deadlock")}

		if recovered := newService(store, provider, &capturedEvents{}).
			backfillMissingPayments(context.Background(), donation, nil); recovered != 0 {
			t.Errorf("recovered %d, want 0", recovered)
		}
	})
}

// Reconciliation asked only for "monthly" funds until daily ones existed, which
// left a daily fund's donations with no status backstop at all -- the single
// thing this job is for. A literal frequency is exactly the kind of thing that
// goes stale silently when a new one is added.
func TestReconciliationCoversEveryRecurringFrequency(t *testing.T) {
	store := &fakeStore{}

	if err := newService(store, &fakeProvider{}, &capturedEvents{}).
		RunRecurringDonationReconciliation(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	asked := map[string]bool{}
	for _, frequency := range store.asked {
		asked[frequency] = true
	}

	for _, frequency := range []donations.PayoutFrequency{
		donations.PayoutFrequencyMonthly,
		donations.PayoutFrequencyDaily,
	} {
		if !asked[string(frequency)] {
			t.Errorf("%s funds are never reconciled, so their subscriptions have no backstop", frequency)
		}
	}

	// A one-off fund has no subscription to check, and the separate one-time pass
	// covers its payments.
	if asked[string(donations.PayoutFrequencyOnce)] {
		t.Error("one-off funds have no subscriptions to reconcile")
	}
}

// Recovery here is by subscription. A one-time donation has none -- its payment
// came from a capture -- so asking the provider to list transactions for an empty
// subscription is a request with no answer, made once per donation per run.
//
// This is not hypothetical: the call was briefly wired into the one-time
// reconciliation path as well, by a replacement that matched in two places.
func TestBackfillSkipsDonationsWithNoSubscription(t *testing.T) {
	provider := &countingProvider{}

	oneTime := donations.Donation{
		ID: uuid.New(), FundID: uuid.New(), DonorID: uuid.New(),
		ProviderSubscriptionID: "",
	}

	recovered := newService(&fakeStore{}, provider, &capturedEvents{}).
		backfillMissingPayments(context.Background(), oneTime, nil)

	if recovered != 0 {
		t.Errorf("recovered %d, want 0", recovered)
	}

	if provider.calls != 0 {
		t.Errorf("asked the provider %d times about a subscription that does not exist", provider.calls)
	}
}

type countingProvider struct {
	paymentsProvider

	calls int
}

func (c *countingProvider) GetTransactionsForDonationSubscription(context.Context, string) ([]ProviderTransaction, error) {
	c.calls++

	return nil, nil
}

// The frequency list lives in one place so that adding one covers everything that
// must iterate them. Reconciliation named "monthly" directly and stopped covering
// everything the day daily funds existed.
func TestEveryFrequencyIsAccountedFor(t *testing.T) {
	var recurring int

	for _, frequency := range donations.PayoutFrequencies {
		if frequency.Recurring() {
			recurring++
		}
	}

	if recurring == 0 {
		t.Fatal("no recurring frequency in the canonical list, so reconciliation covers nothing")
	}

	// Every frequency the enum allows has to be in the list, or something that
	// iterates it silently skips a fund type.
	for _, frequency := range []donations.PayoutFrequency{
		donations.PayoutFrequencyMonthly,
		donations.PayoutFrequencyDaily,
		donations.PayoutFrequencyOnce,
	} {
		var found bool
		for _, known := range donations.PayoutFrequencies {
			if known == frequency {
				found = true
			}
		}

		if !found {
			t.Errorf("%s is missing from PayoutFrequencies", frequency)
		}
	}
}
