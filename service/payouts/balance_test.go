package payouts_test

import (
	"context"
	"testing"

	"boardfund/pg"
	payoutstore "boardfund/service/payouts/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The balance is what the planner divides among payees, so it has to be money
// the account actually holds. It counted donations gross and payouts at face
// value, which overstated every fund by every fee it had ever been charged --
// growing without bound until a batch was planned for money that was not there.
func TestFundBalanceIsNetOfFees(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := payoutstore.NewPayoutStore(pool)

	// donate records a payment with the fee the provider took off it.
	donate := func(t *testing.T, fundID uuid.UUID, cents, feeCents, refundedCents int32) {
		t.Helper()

		donorID := seedMember(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
			 VALUES ($1, false, $2, $3, $4)`,
			donationID, donorID, uuid.NewString(), fundID)
		require.NoError(t, errDonation)

		_, errPayment := pool.Exec(ctx,
			`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, provider_fee_cents, refunded_cents)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			uuid.New(), donationID, uuid.NewString(), cents, feeCents, refundedCents)
		require.NoError(t, errPayment)
	}

	// payOut records a sent payout and what it cost to send.
	payOut := func(t *testing.T, fundID uuid.UUID, cents, feeCents int32, status string) {
		t.Helper()

		memberID := seedMember(t, ctx, pool)
		enrollmentID := uuid.New()

		_, errEnrollment := pool.Exec(ctx,
			`INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, paypal_email, active)
			 VALUES ($1, $2, $3, now(), $4, true)`,
			enrollmentID, fundID, memberID, memberID.String()+"@paypal.test")
		require.NoError(t, errEnrollment)

		batchID := uuid.New()
		_, errBatch := pool.Exec(ctx,
			`INSERT INTO batch_payout (id, fund_id, sender_batch_id, amount_cents, num_enrollments, status, payout_date)
			 VALUES ($1, $2, $3, $4, 1, 'pending', now())`,
			batchID, fundID, uuid.New(), cents)
		require.NoError(t, errBatch)

		_, errPayout := pool.Exec(ctx,
			`INSERT INTO payout (id, fund_enrollment_id, batch_id, amount_cents, provider_fee_cents, status, payout_date, destination_email)
			 VALUES ($1, $2, $3, $4, $5, $6, now(), $7)`,
			uuid.New(), enrollmentID, batchID, cents, feeCents, status,
			memberID.String()+"@paypal.test")
		require.NoError(t, errPayout)
	}

	balance := func(t *testing.T, fundID uuid.UUID) int64 {
		t.Helper()

		cents, errBalance := store.GetFundBalanceCents(ctx, fundID)
		require.NoError(t, errBalance)

		return cents
	}

	t.Run("a donation is worth what arrived, not what was given", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)

		// $5.00 at PayPal's 3.49% + 49c: the account receives $4.34.
		donate(t, fundID, 500, 66, 0)

		require.Equal(t, int64(434), balance(t, fundID))
	})

	t.Run("a payout costs its amount plus the fee to send it", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)

		donate(t, fundID, 1000, 84, 0)
		payOut(t, fundID, 350, 25, "paid")

		// 1000 - 84 in, 350 + 25 out.
		require.Equal(t, int64(916-375), balance(t, fundID))
	})

	// The fee is charged on the way in and PayPal keeps it when a donation is
	// refunded, so it stays subtracted whatever happens to the donation.
	t.Run("a refund does not return the fee", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)

		donate(t, fundID, 500, 66, 500)

		require.Equal(t, int64(-66), balance(t, fundID),
			"the money went back to the donor and the fee did not come back to the fund")
	})

	// unclaimed is money PayPal is holding for a recipient with no account: it has
	// left the fund and it was charged for, so it counts.
	t.Run("an unclaimed payout still counts against the fund", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)

		donate(t, fundID, 1000, 84, 0)
		payOut(t, fundID, 350, 25, "unclaimed")

		require.Equal(t, int64(916-375), balance(t, fundID))
	})

	// Nothing left the account, so nothing was charged for sending it.
	t.Run("a payout that never went does not cost anything", func(t *testing.T) {
		for _, status := range []string{"failed", "cancelled", "returned"} {
			fundID := seedFund(t, ctx, pool)

			donate(t, fundID, 1000, 84, 0)
			payOut(t, fundID, 350, 25, status)

			require.Equalf(t, int64(916), balance(t, fundID), "status %s", status)
		}
	})

	t.Run("a fund with nothing in it is zero, not an error", func(t *testing.T) {
		require.Zero(t, balance(t, seedFund(t, ctx, pool)))
	})
}
