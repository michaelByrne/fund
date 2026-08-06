package store_test

import (
	"context"
	"testing"

	"boardfund/pg"
	hooksstore "boardfund/web/hooksweb/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The signature proves PayPal sent a request; it says nothing about whether we
// have already handled it. This table is what makes that answerable, and the
// answer has to survive a restart, so it is checked against a real database.
func TestRecordDelivery(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := hooksstore.NewDeliveryStore(pool)

	t.Run("a transmission we have not seen is new", func(t *testing.T) {
		fresh, errRecord := store.RecordDelivery(ctx, "tx-1", "PAYMENT.SALE.COMPLETED")
		require.NoError(t, errRecord)
		assert.True(t, fresh)
	})

	t.Run("the same transmission again is not", func(t *testing.T) {
		_, errFirst := store.RecordDelivery(ctx, "tx-2", "PAYMENT.SALE.COMPLETED")
		require.NoError(t, errFirst)

		// A replay, or PayPal redelivering something we already accepted. The same
		// thing from here, and neither needs publishing again.
		fresh, errSecond := store.RecordDelivery(ctx, "tx-2", "PAYMENT.SALE.COMPLETED")
		require.NoError(t, errSecond, "a replay is a thing to ignore, not a fault")
		assert.False(t, fresh)
	})

	t.Run("distinct transmissions of the same event type are independent", func(t *testing.T) {
		// PayPal issues a distinct transmission id per event, so two real payments
		// must not collapse into one.
		first, errFirst := store.RecordDelivery(ctx, "tx-3", "PAYMENT.SALE.COMPLETED")
		require.NoError(t, errFirst)

		second, errSecond := store.RecordDelivery(ctx, "tx-4", "PAYMENT.SALE.COMPLETED")
		require.NoError(t, errSecond)

		assert.True(t, first)
		assert.True(t, second)
	})
}
