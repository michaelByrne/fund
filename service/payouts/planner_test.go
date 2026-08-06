package payouts_test

import (
	"context"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/payouts"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDonation gives the fund money to pay out. The balance query sums
// donation_payment, so a donation with no payment rows contributes nothing --
// which is correct, and is why the payment is created here too.
func seedDonation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID, cents int32) {
	t.Helper()

	donorID := seedMember(t, ctx, pool)
	donationID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
		 VALUES ($1, false, $2, $3, $4)`,
		donationID, donorID, donationID.String(), fundID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents)
		 VALUES ($1, $2, $3, $4)`,
		uuid.New(), donationID, uuid.NewString(), cents,
	)
	require.NoError(t, err)
}

func setFundNextPayment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID, at time.Time) {
	t.Helper()

	_, err := pool.Exec(ctx, `UPDATE fund SET next_payment = $2 WHERE id = $1`, fundID, at)
	require.NoError(t, err)
}

func fundNextPayment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID) *time.Time {
	t.Helper()

	var next *time.Time
	err := pool.QueryRow(ctx, `SELECT next_payment FROM fund WHERE id = $1`, fundID).Scan(&next)
	require.NoError(t, err)

	return next
}

// setFundFrequency switches an already-seeded fund's schedule. seedFund creates
// monthly funds, and the frequency is what decides the step AdvanceFundNextPayment
// takes.
func setFundFrequency(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID, freq string) {
	t.Helper()

	_, err := pool.Exec(ctx, `UPDATE fund SET payout_frequency = $2::payout_frequency WHERE id = $1`, fundID, freq)
	require.NoError(t, err)
}

func TestPlanDueBatches(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	t.Run("splits the balance evenly and leaves the remainder in the fund", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-1"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 3)
		seedDonation(t, ctx, pool, fundID, 1000)

		result, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, result.Planned, 1)

		batches, err := svc.GetBatchesForFund(ctx, fundID)
		require.NoError(t, err)
		require.Len(t, batches, 1)

		// 1000 / 3 floors to 333 each. The batch commits 999 and the odd cent stays
		// put -- paying it to whoever sorted first would make the payout depend on
		// row order.
		assert.Equal(t, int32(999), batches[0].AmountCents)
		assert.Equal(t, int32(3), batches[0].NumEnrollments)

		items, err := svc.GetPayoutsForBatch(ctx, batches[0].ID)
		require.NoError(t, err)
		require.Len(t, items, 3)

		for _, item := range items {
			assert.Equal(t, int32(333), item.AmountCents)
		}

		// Never sent by planning alone, whatever the amounts worked out to.
		assert.Equal(t, payouts.StatusAwaitingApproval, batches[0].Status)
	})

	t.Run("a second run does not plan the same period twice", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-2"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)
		seedDonation(t, ctx, pool, fundID, 5000)

		before := fundNextPayment(t, ctx, pool, fundID)
		require.NotNil(t, before)

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		after := fundNextPayment(t, ctx, pool, fundID)
		require.NotNil(t, after)

		// Compared against the value the database held a moment ago, not against
		// the host clock. Postgres runs in a VM whose clock is milliseconds ahead
		// of the test process, so "is it in the future" is true of a date that was
		// never advanced at all -- this assertion passed against a build with the
		// advance removed entirely.
		assert.WithinDuration(t, before.AddDate(0, 1, 0), *after, time.Minute,
			"next_payment should have moved on by one period")

		// The whole reason the planner advances the date. Without it a daily cron
		// plans a fresh batch every morning for a fund that already has one.
		_, err = svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		batches, err := svc.GetBatchesForFund(ctx, fundID)
		require.NoError(t, err)
		assert.Len(t, batches, 1)
	})

	t.Run("an unfunded fund is retried rather than skipped", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-3"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)

		due := time.Now().Add(-time.Hour)
		setFundNextPayment(t, ctx, pool, fundID, due)

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		batches, err := svc.GetBatchesForFund(ctx, fundID)
		require.NoError(t, err)
		assert.Empty(t, batches, "nothing to pay means no batch")

		// Deliberately still due: the payout is owed, and donations may arrive
		// tomorrow. Advancing here would silently drop the period.
		next := fundNextPayment(t, ctx, pool, fundID)
		require.NotNil(t, next)
		assert.WithinDuration(t, due, *next, time.Second)
	})

	t.Run("a fund with no payable enrollees moves on", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-4"})

		fundID := seedFund(t, ctx, pool)
		seedDonation(t, ctx, pool, fundID, 5000)
		setFundNextPayment(t, ctx, pool, fundID, time.Now().Add(-time.Hour))

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		// Nobody to pay, and waiting will not produce enrollees for a period that
		// has already passed. Left alone this fund would report itself due every
		// day forever.
		next := fundNextPayment(t, ctx, pool, fundID)
		require.NotNil(t, next)
		assert.True(t, next.After(time.Now()))
	})

	t.Run("the batch is dated for the period it pays, not for today", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-5"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)
		seedDonation(t, ctx, pool, fundID, 4000)

		// A run that catches up after a missed day must record what it is paying
		// for. Dating it today would misreport the period and defeat the unique
		// index on (fund_id, payout_date).
		missed := time.Now().Add(-72 * time.Hour)
		setFundNextPayment(t, ctx, pool, fundID, missed)

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		batches, err := svc.GetBatchesForFund(ctx, fundID)
		require.NoError(t, err)
		require.Len(t, batches, 1)

		assert.WithinDuration(t, missed, batches[0].PayoutDate, time.Second)
	})

	t.Run("a one-time fund is never due again", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-6"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)
		seedDonation(t, ctx, pool, fundID, 2000)

		_, err := pool.Exec(ctx, `UPDATE fund SET payout_frequency = 'once' WHERE id = $1`, fundID)
		require.NoError(t, err)
		setFundNextPayment(t, ctx, pool, fundID, time.Now().Add(-time.Hour))

		_, err = svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		assert.Nil(t, fundNextPayment(t, ctx, pool, fundID),
			"a fund that pays once has no next payment")
	})

	t.Run("committed money is not offered to a second batch", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-7"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)
		seedDonation(t, ctx, pool, fundID, 1000)

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		// Still unapproved and unsent, but promised. Counting only settled payouts
		// here would let the next period promise the same cents again.
		setFundNextPayment(t, ctx, pool, fundID, time.Now().Add(-time.Minute))

		_, err = svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		batches, err := svc.GetBatchesForFund(ctx, fundID)
		require.NoError(t, err)
		assert.Len(t, batches, 1, "the balance was already spoken for")
	})
}

func TestSubmitApprovedBatches(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	t.Run("a batch dated in the future is approved but not sent", func(t *testing.T) {
		provider := &stubProvider{batchID: "S-1"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)
		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID,
			// Approval is permission to pay on the payout date, not permission to
			// pay now. Without the date guard, approving next month's batch early
			// would send it immediately.
			PayoutDate:      time.Now().Add(48 * time.Hour),
			AmountCents:     1000,
			RequireApproval: true,
		})
		require.NoError(t, err)

		_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
		require.NoError(t, err)

		submitted, err := svc.SubmitApprovedBatches(ctx)
		require.NoError(t, err)

		assert.Zero(t, submitted)
		assert.Empty(t, provider.submitted)
	})

	t.Run("a batch that is due is sent", func(t *testing.T) {
		provider := &stubProvider{batchID: "S-2"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)
		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          fundID,
			PayoutDate:      time.Now().Add(-time.Hour),
			AmountCents:     1500,
			RequireApproval: true,
		})
		require.NoError(t, err)

		_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
		require.NoError(t, err)

		// The dry run must select exactly what the send selects, or it is a preview
		// of something other than what happens.
		preview, err := svc.GetBatchesReadyToSubmit(ctx)
		require.NoError(t, err)
		require.Len(t, preview, 1)
		assert.Equal(t, batch.ID, preview[0].ID)

		submitted, err := svc.SubmitApprovedBatches(ctx)
		require.NoError(t, err)

		assert.Equal(t, 1, submitted)
		require.Len(t, provider.submitted, 1)
	})

	t.Run("an unapproved batch is never sent", func(t *testing.T) {
		provider := &stubProvider{batchID: "S-3"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		_, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          fundID,
			PayoutDate:      time.Now().Add(-time.Hour),
			AmountCents:     1000,
			RequireApproval: true,
		})
		require.NoError(t, err)

		submitted, err := svc.SubmitApprovedBatches(ctx)
		require.NoError(t, err)

		assert.Zero(t, submitted)
		assert.Empty(t, provider.submitted, "the treasurer gate must survive automation")
	})
}

// A daily fund exists so the payout lifecycle can be watched end to end without
// waiting a month for the second period. That only holds if the schedule really
// advances by a day: stepping a month would make the second run a month away,
// and the whole point of the frequency would be lost.
func TestDailyFundsAdvanceByADay(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	t.Run("advances one day, not one month", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-DAILY-1"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)
		setFundFrequency(t, ctx, pool, fundID, "daily")
		seedDonation(t, ctx, pool, fundID, 1000)

		due := time.Now().Add(-time.Hour)
		setFundNextPayment(t, ctx, pool, fundID, due)

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		next := fundNextPayment(t, ctx, pool, fundID)
		require.NotNil(t, next)

		// The anchor was an hour ago, so the next one is 23 hours out.
		assert.WithinDuration(t, due.Add(24*time.Hour), *next, time.Second)
		assert.True(t, next.After(time.Now()), "a daily fund must not stay due")
		assert.Less(t, time.Until(*next), 48*time.Hour,
			"stepping by a month would defeat the point of a daily fund")
	})

	t.Run("a long-stale daily fund catches up to the next future day", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "P-DAILY-2"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)
		setFundFrequency(t, ctx, pool, fundID, "daily")
		seedDonation(t, ctx, pool, fundID, 1000)

		// Ten days of missed runs, and deliberately not a whole number of them.
		// An anchor exactly N days old advances to the first strictly-future day,
		// which is this instant -- so whether it reads as future depends on the
		// milliseconds between the update and the assertion, and on how closely the
		// container's clock tracks this one. The extra hours put the answer a clear
		// nineteen hours out and the race disappears.
		due := time.Now().Add(-10*24*time.Hour - 5*time.Hour)
		setFundNextPayment(t, ctx, pool, fundID, due)

		_, err := svc.PlanDueBatches(ctx)
		require.NoError(t, err)

		next := fundNextPayment(t, ctx, pool, fundID)
		require.NotNil(t, next)
		assert.True(t, next.After(time.Now()), "one pass should reach a future date")
		assert.Less(t, time.Until(*next), 24*time.Hour, "and not overshoot past tomorrow")
	})
}
