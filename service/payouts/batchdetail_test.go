package payouts_test

import (
	"context"
	"testing"

	"boardfund/pg"
	"boardfund/service/payouts"
	payoutstore "boardfund/service/payouts/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The approval page asks two questions the batch itself cannot answer: which fund
// is about to send money, and who is being paid. Both come from this query.
func TestDetailedBatchesCarryTheFundAndThePayees(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := payoutstore.NewPayoutStore(pool)

	// namedFund is a fund with a name worth reading back.
	namedFund := func(t *testing.T, name string) uuid.UUID {
		t.Helper()

		fundID := uuid.New()
		_, errFund := pool.Exec(ctx,
			`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
			 VALUES ($1, $2, 'd', $3, 'paypal', 'monthly', now())`,
			fundID, name, fundID.String())
		require.NoError(t, errFund)

		return fundID
	}

	// payTo builds a batch awaiting approval that pays the named people.
	payTo := func(t *testing.T, fundID uuid.UUID, names ...string) uuid.UUID {
		t.Helper()

		batchID := uuid.New()
		_, errBatch := pool.Exec(ctx,
			`INSERT INTO batch_payout (id, fund_id, sender_batch_id, amount_cents, num_enrollments, status, payout_date)
			 VALUES ($1, $2, $3, 1000, $4, 'awaiting_approval', now())`,
			batchID, fundID, uuid.New(), len(names))
		require.NoError(t, errBatch)

		for _, name := range names {
			memberID := uuid.New()
			_, errMember := pool.Exec(ctx,
				`INSERT INTO member (id, email, bco_name, active) VALUES ($1, $2, $3, true)`,
				memberID, memberID.String()+"@test.org", name)
			require.NoError(t, errMember)

			enrollmentID := uuid.New()
			_, errEnrollment := pool.Exec(ctx,
				`INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, paypal_email, active)
				 VALUES ($1, $2, $3, now(), $4, true)`,
				enrollmentID, fundID, memberID, memberID.String()+"@paypal.test")
			require.NoError(t, errEnrollment)

			_, errPayout := pool.Exec(ctx,
				`INSERT INTO payout (id, fund_enrollment_id, batch_id, amount_cents, status, payout_date, destination_email)
				 VALUES ($1, $2, $3, 500, 'planned', now(), $4)`,
				uuid.New(), enrollmentID, batchID, memberID.String()+"@paypal.test")
			require.NoError(t, errPayout)
		}

		return batchID
	}

	find := func(t *testing.T, batches []payouts.BatchDetail, batchID uuid.UUID) payouts.BatchDetail {
		t.Helper()

		for _, batch := range batches {
			if batch.ID == batchID {
				return batch
			}
		}

		t.Fatalf("batch %s not listed", batchID)

		return payouts.BatchDetail{}
	}

	fundID := namedFund(t, "human fund "+uuid.NewString())
	withPayees := payTo(t, fundID, "ada-"+uuid.NewString(), "bo-"+uuid.NewString())

	// A batch planned but with no payout rows yet. LEFT JOIN or it vanishes from
	// the page that exists to approve it.
	emptyBatch := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO batch_payout (id, fund_id, sender_batch_id, amount_cents, num_enrollments, status, payout_date)
		 VALUES ($1, $2, $3, 1000, 3, 'awaiting_approval', now())`,
		emptyBatch, fundID, uuid.New())
	require.NoError(t, err)

	batches, err := store.GetDetailedBatchesByStatus(ctx, payouts.StatusAwaitingApproval)
	require.NoError(t, err)

	t.Run("carries the fund name", func(t *testing.T) {
		require.Equal(t, "human fund", find(t, batches, withPayees).FundName[:10])
	})

	t.Run("carries every payee", func(t *testing.T) {
		names := find(t, batches, withPayees).PayeeNames
		require.Len(t, names, 2)
	})

	t.Run("a batch with no payouts still lists", func(t *testing.T) {
		batch := find(t, batches, emptyBatch)
		require.Empty(t, batch.PayeeNames)
		require.Equal(t, int32(3), batch.NumEnrollments, "the count is on the batch, not the join")
	})

	t.Run("payees do not leak between batches", func(t *testing.T) {
		other := payTo(t, namedFund(t, "winter fund "+uuid.NewString()), "cyd-"+uuid.NewString())

		refreshed, errList := store.GetDetailedBatchesByStatus(ctx, payouts.StatusAwaitingApproval)
		require.NoError(t, errList)

		require.Len(t, find(t, refreshed, other).PayeeNames, 1)
		require.Len(t, find(t, refreshed, withPayees).PayeeNames, 2,
			"the group by must keep each batch's payees to itself")
	})

	t.Run("only the status asked for", func(t *testing.T) {
		ready, errReady := store.GetDetailedBatchesByStatus(ctx, payouts.StatusReady)
		require.NoError(t, errReady)

		for _, batch := range ready {
			require.NotEqual(t, withPayees, batch.ID)
		}
	})
}
