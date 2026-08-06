package fundevents_test

import (
	"context"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Webhook delivery is at-least-once, so a handler that writes its rows and then
// fails to acknowledge is given the same event again. The money is safe -- a
// payment is unique on the provider's id -- but an event recorded unconditionally
// lands twice, and the feed shows one cancellation as two.
func TestFundEventDeduplication(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := fundeventstore.NewEventStore(pool)

	newFund := func(t *testing.T) uuid.UUID {
		t.Helper()

		fundID := uuid.New()
		_, errFund := pool.Exec(ctx,
			`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
			 VALUES ($1, $2, 'd', $3, 'paypal', 'once', now())`,
			fundID, uuid.NewString(), fundID.String(),
		)
		require.NoError(t, errFund)

		return fundID
	}

	count := func(t *testing.T, fundID uuid.UUID) int {
		t.Helper()

		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM fund_event WHERE fund_id = $1`, fundID).Scan(&n))

		return n
	}

	t.Run("the same key records once", func(t *testing.T) {
		fundID := newFund(t)

		record := fundevents.Record{
			FundID:    fundID,
			Kind:      fundevents.KindDonationCancelled,
			Detail:    "subscription cancelled at provider",
			DedupeKey: "subscription-ended:SUB-1:CANCELLED",
		}

		first, errFirst := store.InsertFundEvent(ctx, record)
		require.NoError(t, errFirst)
		require.NotNil(t, first, "the first delivery records the event")

		// The redelivery: same key, and nothing about it says it is a repeat.
		second, errSecond := store.InsertFundEvent(ctx, record)
		require.NoError(t, errSecond, "a redelivery is ordinary, not an error")
		assert.Nil(t, second, "a redelivery records nothing")

		assert.Equal(t, 1, count(t, fundID), "the feed should show one cancellation")
	})

	t.Run("different keys both record", func(t *testing.T) {
		fundID := newFund(t)

		// Two genuine failures against one subscription, separated by the
		// provider's own update time. Keying on the subscription alone would keep
		// the first and swallow every one after it.
		for _, when := range []string{"2026-08-06T10:00:00Z", "2026-08-13T10:00:00Z"} {
			_, errRecord := store.InsertFundEvent(ctx, fundevents.Record{
				FundID:    fundID,
				Kind:      fundevents.KindPaymentFailed,
				Detail:    "payment failed at provider",
				DedupeKey: "payment-failed:SUB-1:" + when,
			})
			require.NoError(t, errRecord)
		}

		assert.Equal(t, 2, count(t, fundID), "two failures are two events")
	})

	t.Run("events with no key are never deduplicated", func(t *testing.T) {
		fundID := newFund(t)

		// Everything a person does in the admin UI happens once by construction and
		// supplies no key. The partial index must not collapse those into one row
		// just because they are otherwise identical.
		for i := 0; i < 3; i++ {
			_, errRecord := store.InsertFundEvent(ctx, fundevents.Record{
				FundID: fundID,
				Kind:   fundevents.KindBatchApproved,
				Detail: "approved",
			})
			require.NoError(t, errRecord)
		}

		assert.Equal(t, 3, count(t, fundID), "unkeyed events are independent")
	})

	t.Run("a key is scoped to nothing but itself", func(t *testing.T) {
		// Two funds, one key. The index is global on purpose: the key already
		// carries the provider's identifiers, and scoping it per fund would let a
		// redelivery duplicate across funds if a subscription ever moved.
		first, second := newFund(t), newFund(t)
		key := "subscription-ended:SUB-SHARED:CANCELLED"

		_, errFirst := store.InsertFundEvent(ctx, fundevents.Record{
			FundID: first, Kind: fundevents.KindDonationCancelled, DedupeKey: key,
		})
		require.NoError(t, errFirst)

		duplicate, errSecond := store.InsertFundEvent(ctx, fundevents.Record{
			FundID: second, Kind: fundevents.KindDonationCancelled, DedupeKey: key,
		})
		require.NoError(t, errSecond)
		assert.Nil(t, duplicate)

		assert.Equal(t, 0, count(t, second))
	})

	t.Run("the occurred time is still the provider's", func(t *testing.T) {
		fundID := newFund(t)
		when := time.Date(2026, time.August, 1, 9, 30, 0, 0, time.UTC)

		event, errRecord := store.InsertFundEvent(ctx, fundevents.Record{
			FundID:     fundID,
			Kind:       fundevents.KindPaymentFailed,
			OccurredAt: when,
			DedupeKey:  "payment-failed:SUB-2:once",
		})
		require.NoError(t, errRecord)
		require.NotNil(t, event)

		// Deduplication must not have quietly become "insert with now()", which is
		// what a natural key including occurred_at would have forced.
		assert.WithinDuration(t, when, event.OccurredAt, time.Second)
	})
}
