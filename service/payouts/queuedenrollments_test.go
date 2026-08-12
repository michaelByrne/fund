package payouts_test

import (
	"context"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/payouts"

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
