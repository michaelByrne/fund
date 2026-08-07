package donations_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/mocks"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// payoutItem is one line of a batch: who was paid, how much, and how it settled.
type payoutItem struct {
	member uuid.UUID
	cents  int32
	status string
}

// seedPaidBatch writes one batch paying everyone in it, which is how batches
// actually work -- and is forced by the unique index on (fund_id, payout_date),
// correctly, since two batches for one fund on one date is what double-paying
// looks like.
//
// Status matters per item: the archive reports what was handed out, so anything
// unsettled must not reach those figures.
func seedPaidBatch(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID, when time.Time, items ...payoutItem) {
	t.Helper()

	var total int32
	for _, item := range items {
		total += item.cents
	}

	batchID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO batch_payout (id, fund_id, sender_batch_id, amount_cents, num_enrollments, status, payout_date)
		 VALUES ($1, $2, $3, $4, $5, 'paid', $6)`,
		batchID, fundID, uuid.New(), total, len(items), when,
	)
	require.NoError(t, err)

	for _, item := range items {
		email := item.member.String() + "@paypal.test"

		enrollmentID := uuid.New()
		_, err = pool.Exec(ctx,
			`INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, member_bco_name, paypal_email, active)
			 VALUES ($1, $2, $3, now(), $4, $5, true)`,
			enrollmentID, fundID, item.member, item.member.String()[:8], email,
		)
		require.NoError(t, err)

		_, err = pool.Exec(ctx,
			`INSERT INTO payout (id, fund_enrollment_id, batch_id, amount_cents, status, payout_date, destination_email)
			 VALUES ($1, $2, $3, $4, $5::payout_status, $6, $7)`,
			uuid.New(), enrollmentID, batchID, item.cents, item.status, when, email,
		)
		require.NoError(t, err)
	}
}

// seedDonation gives a fund money. The stats query sums donation_payment, so a
// donation with no payment row contributes nothing -- which is correct, and is
// why the payment is written here too.
func seedDonation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID, cents int32) {
	t.Helper()

	donorID := seedTestMember(t, ctx, pool)
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

func newArchiveService(t *testing.T, pool *pgxpool.Pool) *donations.DonationService {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	return donations.NewDonationService(
		donationsstore.NewDonationStore(pool),
		stubDocumentStorage{}, newFakeBucket(),
		&mocks.PaymentsProviderMock{},
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger),
		[]string{"payments"},
		logger,
	)
}

func TestListClosedFunds(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	svc := newArchiveService(t, pool)

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(24 * time.Hour)
	paidOn := time.Now().Add(-2 * time.Hour)

	closedID := insertFund(t, ctx, pool, "archive-closed", true, &past)
	insertFund(t, ctx, pool, "archive-open", true, &future)
	emptyID := insertFund(t, ctx, pool, "archive-nopayouts", false, nil)

	seedDonation(t, ctx, pool, closedID, 5000)

	memberOne := seedTestMember(t, ctx, pool)
	memberTwo := seedTestMember(t, ctx, pool)

	seedPaidBatch(t, ctx, pool, closedID, paidOn,
		payoutItem{memberOne, 1500, "paid"},
		payoutItem{memberTwo, 1500, "paid"},
		// Neither of these counts towards what the fund handed out. In the same
		// batch on purpose: a batch marked paid can still contain items that were
		// not, and the figures must come from the items rather than the batch.
		payoutItem{seedTestMember(t, ctx, pool), 900, "pending"},
		payoutItem{seedTestMember(t, ctx, pool), 700, "failed"},
	)

	archive, err := svc.ListClosedFunds(ctx)
	require.NoError(t, err)

	byName := make(map[string]donations.ClosedFund, len(archive))
	for _, fund := range archive {
		byName[fund.Name] = fund
	}

	require.Contains(t, byName, "archive-closed")
	require.Contains(t, byName, "archive-nopayouts")
	assert.NotContains(t, byName, "archive-open", "an open fund is not archive material")

	got := byName["archive-closed"]

	assert.Equal(t, int64(3000), got.Payouts.TotalPaidCents, "only settled money counts")
	assert.Equal(t, int64(2), got.Payouts.TotalRecipients)
	assert.Equal(t, int64(2), got.Payouts.TotalPayouts)
	assert.Equal(t, int32(5000), got.Stats.TotalDonated)
	assert.Equal(t, int64(2000), got.Undisbursed())

	require.NotNil(t, got.Payouts.LastPayoutDate)
	assert.WithinDuration(t, paidOn, *got.Payouts.LastPayoutDate, time.Minute)

	// The outer join has to survive a fund that paid nobody: zero, not unknown,
	// and no date at all rather than a zero time that renders as a real one.
	empty := byName["archive-nopayouts"]

	assert.Equal(t, emptyID, empty.ID)
	assert.Zero(t, empty.Payouts.TotalPaidCents)
	assert.Zero(t, empty.Payouts.TotalRecipients)
	assert.Zero(t, empty.Payouts.TotalPayouts)
	assert.Nil(t, empty.Payouts.LastPayoutDate)
}

// countingStore embeds the real store and counts the one call that would make
// the archive scale with its own size. Embedding means every other method still
// hits the database, so this measures the real code path rather than a fake one.
type countingStore struct {
	donationsstore.DonationStore

	payoutStatsCalls int
}

func (c *countingStore) GetFundPayoutStats(ctx context.Context, fundID uuid.UUID) (donations.PayoutStats, error) {
	c.payoutStatsCalls++

	return c.DonationStore.GetFundPayoutStats(ctx, fundID)
}

// The archive drives the front page and only grows, so its cost must not scale
// with the number of closed funds.
//
// Counted at the store rather than in the database. The first version read
// pg_stat_database, which is snapshot-cached and reported identical figures
// before and after -- it passed against a deliberately reintroduced per-fund
// loop, which is how it was caught.
func TestListClosedFundsDoesNotQueryPerFund(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := &countingStore{DonationStore: donationsstore.NewDonationStore(pool)}

	svc := donations.NewDonationService(
		store,
		stubDocumentStorage{}, newFakeBucket(),
		&mocks.PaymentsProviderMock{},
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger),
		[]string{"payments"},
		logger,
	)

	past := time.Now().Add(-time.Hour)
	for i := 0; i < 6; i++ {
		fundID := insertFund(t, ctx, pool, "bulk-"+uuid.NewString()[:8], true, &past)
		seedPaidBatch(t, ctx, pool, fundID, past, payoutItem{seedTestMember(t, ctx, pool), 100, "paid"})
	}

	archive, err := svc.ListClosedFunds(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(archive), 6, "the funds must actually be in the archive")

	assert.Zero(t, store.payoutStatsCalls,
		"the archive query carries its own aggregates; a per-fund lookup means it scales with the archive")
}

func TestGetClosedFund(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	svc := newArchiveService(t, pool)

	past := time.Now().Add(-time.Hour)
	fundID := insertFund(t, ctx, pool, "summary-fund", true, &past)

	seedDonation(t, ctx, pool, fundID, 4000)
	seedPaidBatch(t, ctx, pool, fundID, past, payoutItem{seedTestMember(t, ctx, pool), 2500, "paid"})

	fund, err := svc.GetClosedFund(ctx, fundID)
	require.NoError(t, err)

	assert.Equal(t, "summary-fund", fund.Name)
	assert.Equal(t, int64(2500), fund.Payouts.TotalPaidCents)
	assert.Equal(t, int64(1), fund.Payouts.TotalRecipients)
	assert.Equal(t, int64(1500), fund.Undisbursed())
	assert.True(t, fund.Closed())

	// Expiry is the closure date when there is one; a deactivated fund with no
	// end date falls back to when it was last touched.
	assert.WithinDuration(t, past, fund.ClosedOn(), time.Minute)

	deactivatedID := insertFund(t, ctx, pool, "no-expiry-closed", false, nil)

	deactivated, err := svc.GetClosedFund(ctx, deactivatedID)
	require.NoError(t, err)

	assert.WithinDuration(t, deactivated.Updated, deactivated.ClosedOn(), time.Second)
}
