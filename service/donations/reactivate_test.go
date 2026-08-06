package donations_test

import (
	"context"
	"testing"

	"boardfund/pg"
	donationsstore "boardfund/service/donations/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reactivation is guarded in SQL, and the guard is the whole safety argument: a
// payment may bring back a subscription PayPal suspended, and must not bring back
// one a member cancelled or one whose fund has closed. Those are the same row in
// the same state apart from why it got there, so the guard cannot be checked
// anywhere but against a real database.
func TestReactivateSuspendedDonation(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := donationsstore.NewDonationStore(pool)

	seed := func(t *testing.T, active bool, reason string, fundActive bool) string {
		t.Helper()

		fundID := seedOnceFund(t, ctx, pool)
		if !fundActive {
			_, errFund := pool.Exec(ctx, `UPDATE fund SET active = false WHERE id = $1`, fundID)
			require.NoError(t, errFund)
		}

		subscriptionID := uuid.NewString()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id,
			                       active, provider_subscription_id, inactive_reason)
			 VALUES ($1, true, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), seedMemberRow(t, ctx, pool), uuid.NewString(), fundID,
			active, subscriptionID, reason,
		)
		require.NoError(t, errDonation)

		return subscriptionID
	}

	isActive := func(t *testing.T, subscriptionID string) bool {
		t.Helper()

		var active bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT active FROM donation WHERE provider_subscription_id = $1`,
			subscriptionID).Scan(&active))

		return active
	}

	t.Run("brings back a suspended donation", func(t *testing.T) {
		subscriptionID := seed(t, false, "SUSPENDED", true)

		resumed, errResume := store.ReactivateSuspendedDonation(ctx, subscriptionID)
		require.NoError(t, errResume)
		require.NotNil(t, resumed, "a suspension is exactly what a payment should overturn")

		assert.True(t, isActive(t, subscriptionID))
	})

	t.Run("leaves a cancelled donation cancelled", func(t *testing.T) {
		// Cancelled by the member. A late or duplicate payment is not evidence
		// that anybody wants it running again.
		subscriptionID := seed(t, false, "CANCELLED", true)

		resumed, errResume := store.ReactivateSuspendedDonation(ctx, subscriptionID)
		require.NoError(t, errResume)
		assert.Nil(t, resumed, "a cancellation must not be undone by a payment")

		assert.False(t, isActive(t, subscriptionID))
	})

	t.Run("leaves an expired donation expired", func(t *testing.T) {
		subscriptionID := seed(t, false, "EXPIRED", true)

		resumed, errResume := store.ReactivateSuspendedDonation(ctx, subscriptionID)
		require.NoError(t, errResume)
		assert.Nil(t, resumed)

		assert.False(t, isActive(t, subscriptionID))
	})

	t.Run("will not reactivate into a closed fund", func(t *testing.T) {
		// Suspended for the right reason, but the fund has since closed. Bringing
		// this back would leave it collecting money the fund can no longer pay out.
		subscriptionID := seed(t, false, "SUSPENDED", false)

		resumed, errResume := store.ReactivateSuspendedDonation(ctx, subscriptionID)
		require.NoError(t, errResume)
		assert.Nil(t, resumed, "a closed fund must not start collecting again")

		assert.False(t, isActive(t, subscriptionID))
	})

	t.Run("an already active donation reports nothing to do", func(t *testing.T) {
		subscriptionID := seed(t, true, "", true)

		resumed, errResume := store.ReactivateSuspendedDonation(ctx, subscriptionID)
		require.NoError(t, errResume)
		assert.Nil(t, resumed, "nothing changed, so the handler should record nothing")

		assert.True(t, isActive(t, subscriptionID))
	})

	t.Run("an unknown subscription is not an error", func(t *testing.T) {
		resumed, errResume := store.ReactivateSuspendedDonation(ctx, uuid.NewString())
		require.NoError(t, errResume, "a subscription we do not know is ordinary, not a failure")
		assert.Nil(t, resumed)
	})
}
