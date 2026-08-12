package payouts_test

import (
	"context"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/payouts"
	payoutstore "boardfund/service/payouts/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Removing somebody from a fund cannot unpick a batch that is already planned:
// SubmitBatch reads the payout rows by batch id, and those froze the amount and
// the address when the batch was built. This is the lookup that lets the admin
// page say so before the removal rather than after the money has gone.
func TestEnrollmentsInUnsentBatches(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	enrollmentIDs := func(t *testing.T, fundID uuid.UUID) []uuid.UUID {
		t.Helper()

		var ids []uuid.UUID

		rows, errRows := pool.Query(ctx,
			`SELECT id FROM fund_enrollment WHERE fund_id = $1 ORDER BY created`, fundID)
		require.NoError(t, errRows)

		defer rows.Close()

		for rows.Next() {
			var id uuid.UUID
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}

		return ids
	}

	t.Run("a batch awaiting approval names its payees", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-Q1"})
		fundID := seedFundWithEnrollees(t, ctx, pool, 2)

		_, errPlan := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          fundID,
			PayoutDate:      time.Now(),
			AmountCents:     2500,
			RequireApproval: true,
		})
		require.NoError(t, errPlan)

		queued, errQueued := svc.EnrollmentsInUnsentBatches(ctx, fundID)
		require.NoError(t, errQueued)

		for _, id := range enrollmentIDs(t, fundID) {
			assert.True(t, queued[id], "enrollment %s is in the planned batch", id)
		}
	})

	// The warning is about money that has not moved. Once a batch reaches the
	// provider, removing the member cannot affect it, and saying so on every row
	// after every payout is the kind of notice people learn to skip.
	t.Run("a submitted batch is not warned about", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-Q2"})
		fundID := seedFundWithEnrollees(t, ctx, pool, 2)

		batch, errPlan := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          fundID,
			PayoutDate:      time.Now(),
			AmountCents:     2500,
			RequireApproval: false,
		})
		require.NoError(t, errPlan)

		_, errSubmit := svc.SubmitBatch(ctx, batch.ID)
		require.NoError(t, errSubmit)

		queued, errQueued := svc.EnrollmentsInUnsentBatches(ctx, fundID)
		require.NoError(t, errQueued)

		assert.Empty(t, queued, "the money has gone; there is nothing to warn about")
	})

	// A fund with nothing planned has to come back empty rather than error, since
	// the page renders that state on every visit.
	t.Run("no batches means nobody is queued", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-Q3"})
		fundID := seedFundWithEnrollees(t, ctx, pool, 2)

		queued, errQueued := svc.EnrollmentsInUnsentBatches(ctx, fundID)
		require.NoError(t, errQueued)
		assert.Empty(t, queued)
	})

	// Scoped by fund. The panel it feeds shows one fund's enrollments, and a
	// marker from somebody else's batch would be a warning about nothing.
	t.Run("another fund's batch does not leak in", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-Q4"})

		busy := seedFundWithEnrollees(t, ctx, pool, 2)
		quiet := seedFundWithEnrollees(t, ctx, pool, 2)

		_, errPlan := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          busy,
			PayoutDate:      time.Now(),
			AmountCents:     2500,
			RequireApproval: true,
		})
		require.NoError(t, errPlan)

		queued, errQueued := svc.EnrollmentsInUnsentBatches(ctx, quiet)
		require.NoError(t, errQueued)
		assert.Empty(t, queued)
	})
}

// A batch that comes to nothing must not leave a one-off fund unplannable.
//
// The planner clears next_payment as soon as a batch exists, before any money
// moves. Without a requeue, a rejected or expired batch left a 'once' fund with
// no anchor: the planner never picks it up again, and the expiry job reads the
// NULL as "the payout has been dealt with" and closes the fund on its balance.
func TestAOneTimeFundIsRequeuedWhenItsBatchComesToNothing(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	// A one-off fund, dated in the past so it is due, with money and a payee.
	onceFundDue := func(t *testing.T, name string) uuid.UUID {
		t.Helper()

		fundID := seedFund(t, ctx, pool)

		_, errFund := pool.Exec(ctx,
			`UPDATE fund
			 SET name = $2, payout_frequency = 'once',
			     expires = now() - INTERVAL '1 hour', next_payment = now() - INTERVAL '1 hour'
			 WHERE id = $1`, fundID, name)
		require.NoError(t, errFund)

		memberID := seedMember(t, ctx, pool)
		seedEnrollment(t, ctx, pool, fundID, memberID, memberID.String()+"@paypal.test")
		seedDonation(t, ctx, pool, fundID, 50000)

		return fundID
	}

	nextPayment := func(t *testing.T, fundID uuid.UUID) *time.Time {
		t.Helper()

		var next *time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT next_payment FROM fund WHERE id = $1`, fundID).Scan(&next))

		return next
	}

	t.Run("a rejected batch puts the payout back", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-R1"})
		fundID := onceFundDue(t, "once-rejected")

		planned, errPlan := svc.PlanDueBatches(ctx)
		require.NoError(t, errPlan)
		require.Equal(t, 1, planned.Planned)

		// Cleared by the planner, before any money moved.
		require.Nil(t, nextPayment(t, fundID))

		batches, errList := svc.GetBatchesAwaitingApproval(ctx)
		require.NoError(t, errList)
		require.NotEmpty(t, batches)

		_, errReject := svc.RejectBatch(ctx, batches[0].ID, "not like that")
		require.NoError(t, errReject)

		assert.NotNil(t, nextPayment(t, fundID),
			"a rejected payout is still owed, so the fund must stay plannable")

		// And the fund is held open, rather than closed on the balance it still
		// holds.
		open, errOpen := svc.EnrollmentsInUnsentBatches(ctx, fundID)
		require.NoError(t, errOpen)
		assert.Empty(t, open, "the rejected batch no longer counts as queued")

		// The planner picks it up again on the next run.
		again, errAgain := svc.PlanDueBatches(ctx)
		require.NoError(t, errAgain)
		assert.GreaterOrEqual(t, again.Planned, 1, "a rejected one-off fund should re-plan")
	})

	t.Run("an expired approval window puts the payout back", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-R2"})
		fundID := onceFundDue(t, "once-expired")

		planned, errPlan := svc.PlanDueBatches(ctx)
		require.NoError(t, errPlan)
		require.Equal(t, 1, planned.Planned)
		require.Nil(t, nextPayment(t, fundID))

		// Push the deadline into the past so the sweep cancels it.
		_, errDeadline := pool.Exec(ctx,
			`UPDATE batch_payout SET approval_deadline = now() - INTERVAL '1 hour'
			 WHERE fund_id = $1 AND status = 'awaiting_approval'`, fundID)
		require.NoError(t, errDeadline)

		require.NoError(t, svc.RunApprovalSweep(ctx))

		assert.NotNil(t, nextPayment(t, fundID),
			"nobody approved it, so the payout is still owed")
	})

	// A recurring fund's anchor has already moved to the next period and its
	// money rolls into that payout, so there is nothing to put back -- and
	// putting one back would pay the same period twice.
	//
	// AdvanceFundNextPayment only nulls the anchor for 'once', so a recurring
	// fund never reaches the state the requeue looks for and the frequency check
	// is belt-and-braces. The state is forced below so the check is tested for
	// what it is actually for: holding if that ever stops being true.
	t.Run("a recurring fund with no anchor is still left alone", func(t *testing.T) {
		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		_, errSetup := pool.Exec(ctx,
			`UPDATE fund SET expires = now() - INTERVAL '1 hour', next_payment = NULL WHERE id = $1`,
			fundID)
		require.NoError(t, errSetup)

		requeued, errRequeue := payoutstore.NewPayoutStore(pool).RequeueOneTimeFundPayout(ctx, fundID)
		require.NoError(t, errRequeue)

		assert.False(t, requeued, "only a one-off fund's payout is put back")
		assert.Nil(t, nextPayment(t, fundID))
	})

	t.Run("a recurring fund is left alone", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-R3"})
		fundID := seedFundWithEnrollees(t, ctx, pool, 1)
		seedDonation(t, ctx, pool, fundID, 50000)

		// With an end date, which is ordinary for a monthly fund and is what makes
		// this test able to see the requeue reaching for the wrong funds: the query
		// also refuses a NULL expires, so a fund without one would be left alone
		// for the wrong reason.
		_, errExpires := pool.Exec(ctx,
			`UPDATE fund SET expires = now() + INTERVAL '90 days' WHERE id = $1`, fundID)
		require.NoError(t, errExpires)

		batch, errPlan := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 2500, RequireApproval: true,
		})
		require.NoError(t, errPlan)

		before := nextPayment(t, fundID)

		_, errReject := svc.RejectBatch(ctx, batch.ID, "not this month")
		require.NoError(t, errReject)

		after := nextPayment(t, fundID)

		require.NotNil(t, before)
		require.NotNil(t, after)
		assert.WithinDuration(t, *before, *after, time.Second,
			"a monthly fund's schedule is not rewound by a rejection")
	})
}
