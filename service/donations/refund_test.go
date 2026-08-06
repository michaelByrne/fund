package donations_test

import (
	"context"
	"testing"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	payoutsstore "boardfund/service/payouts/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fund balance is what the payout planner divides between enrollees. A refund
// that is not subtracted leaves the fund paying out money it no longer holds, out
// of a PayPal balance shared with every other fund.
func TestRefundsLeaveTheFund(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := donationsstore.NewDonationStore(pool)
	payouts := payoutsstore.NewPayoutStore(pool)

	seedPaidDonation := func(t *testing.T, cents int32) (uuid.UUID, string) {
		t.Helper()

		fundID := seedOnceFund(t, ctx, pool)
		donationID := seedDonationRow(t, ctx, pool, fundID)
		providerPaymentID := uuid.NewString()

		_, errPayment := pool.Exec(ctx,
			`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents)
			 VALUES ($1, $2, $3, $4)`,
			uuid.New(), donationID, providerPaymentID, cents,
		)
		require.NoError(t, errPayment)

		return fundID, providerPaymentID
	}

	balance := func(t *testing.T, fundID uuid.UUID) int64 {
		t.Helper()

		available, errBalance := payouts.GetFundBalanceCents(ctx, fundID)
		require.NoError(t, errBalance)

		return available
	}

	t.Run("a full refund removes the money from the balance", func(t *testing.T) {
		fundID, paymentID := seedPaidDonation(t, 5000)
		require.EqualValues(t, 5000, balance(t, fundID))

		refunded, errRefund := store.SetDonationPaymentRefunded(ctx, paymentID, 5000)
		require.NoError(t, errRefund)
		require.NotNil(t, refunded)

		assert.Zero(t, balance(t, fundID), "a fully refunded payment must not be payable")

		// And the caller has what the activity entry needs without a second query.
		assert.Equal(t, fundID, refunded.FundID)
		assert.NotEqual(t, uuid.Nil, refunded.DonorID)
	})

	t.Run("a partial refund leaves the rest payable", func(t *testing.T) {
		fundID, paymentID := seedPaidDonation(t, 5000)

		_, errRefund := store.SetDonationPaymentRefunded(ctx, paymentID, 2000)
		require.NoError(t, errRefund)

		assert.EqualValues(t, 3000, balance(t, fundID), "only the refunded part leaves")
	})

	t.Run("the refunded total is set, not accumulated", func(t *testing.T) {
		fundID, paymentID := seedPaidDonation(t, 5000)

		// PayPal reports total_refunded_amount, a running total. A second partial
		// refund of 1000 arrives reporting 3000, not 1000.
		_, errFirst := store.SetDonationPaymentRefunded(ctx, paymentID, 2000)
		require.NoError(t, errFirst)

		_, errSecond := store.SetDonationPaymentRefunded(ctx, paymentID, 3000)
		require.NoError(t, errSecond)

		assert.EqualValues(t, 2000, balance(t, fundID), "adding the two would have left 0")
	})

	t.Run("reports what changed as well as the new total", func(t *testing.T) {
		_, paymentID := seedPaidDonation(t, 5000)

		first, errFirst := store.SetDonationPaymentRefunded(ctx, paymentID, 2000)
		require.NoError(t, errFirst)
		require.NotNil(t, first)

		assert.EqualValues(t, 0, first.PreviouslyRefundedCents)
		assert.EqualValues(t, 2000, first.NewlyRefundedCents())

		// The running total goes 2000 -> 3000, but only 1000 came back this time.
		// The balance wants the total; the activity feed wants the difference, and
		// recording the total there would report 3000 returned when 1000 did.
		second, errSecond := store.SetDonationPaymentRefunded(ctx, paymentID, 3000)
		require.NoError(t, errSecond)
		require.NotNil(t, second)

		assert.EqualValues(t, 2000, second.PreviouslyRefundedCents,
			"the prior total must be read before the update overwrites it")
		assert.EqualValues(t, 3000, second.RefundedCents)
		assert.EqualValues(t, 1000, second.NewlyRefundedCents(),
			"a second partial refund moved only its own amount")
	})

	t.Run("a redelivered refund reports nothing to do", func(t *testing.T) {
		fundID, paymentID := seedPaidDonation(t, 5000)

		first, errFirst := store.SetDonationPaymentRefunded(ctx, paymentID, 5000)
		require.NoError(t, errFirst)
		require.NotNil(t, first)

		// The same webhook again. The handler reads nil as "already recorded" and
		// skips the fund event, so the feed shows one refund.
		second, errSecond := store.SetDonationPaymentRefunded(ctx, paymentID, 5000)
		require.NoError(t, errSecond, "a redelivery is ordinary, not an error")
		assert.Nil(t, second)

		assert.Zero(t, balance(t, fundID))
	})

	t.Run("a payment we do not know is not an error", func(t *testing.T) {
		// The same PayPal account can carry sales this fund did not originate.
		refunded, errRefund := store.SetDonationPaymentRefunded(ctx, uuid.NewString(), 100)
		require.NoError(t, errRefund)
		assert.Nil(t, refunded)
	})

	t.Run("a refund cannot exceed the payment", func(t *testing.T) {
		_, paymentID := seedPaidDonation(t, 5000)

		// Guarded in the schema. Without it a bad or malicious amount would make
		// the fund's balance negative, and a negative balance is not a state the
		// planner has any sensible answer for.
		_, errRefund := store.SetDonationPaymentRefunded(ctx, paymentID, 9000)
		assert.Error(t, errRefund, "refunding more than was taken must be refused")
	})

	t.Run("what a fund reports as collected drops too", func(t *testing.T) {
		fundID, paymentID := seedPaidDonation(t, 5000)

		before, errBefore := store.GetTotalDonatedByFundID(ctx, fundID)
		require.NoError(t, errBefore)
		require.EqualValues(t, 5000, before)

		_, errRefund := store.SetDonationPaymentRefunded(ctx, paymentID, 5000)
		require.NoError(t, errRefund)

		after, errAfter := store.GetTotalDonatedByFundID(ctx, fundID)
		require.NoError(t, errAfter)

		// Otherwise the public page reports money the fund does not have, which is
		// the number donors judge it by.
		assert.Zero(t, after, "a refunded donation is not money the fund collected")
	})
}

var _ = donations.RefundedPayment{}
