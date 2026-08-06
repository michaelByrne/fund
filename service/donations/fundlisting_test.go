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

func insertFund(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, active bool, expires *time.Time) uuid.UUID {
	t.Helper()

	fundID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, active, expires, next_payment)
		 VALUES ($1, $2, 'd', $3, 'paypal', 'monthly', $4, $5, now())`,
		fundID, name, fundID.String(), active, expires,
	)
	require.NoError(t, err)

	return fundID
}

func fundNames(funds []donations.Fund) map[string]donations.Fund {
	byName := make(map[string]donations.Fund, len(funds))
	for _, fund := range funds {
		byName[fund.Name] = fund
	}

	return byName
}

// A fund that expires stops taking donations. It does not stop existing, and the
// admin listing is where someone goes to see what it paid out -- which is most
// likely on the day it closed.
func TestAdminListingKeepsClosedFunds(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(store, stubDocumentStorage{}, &mocks.PaymentsProviderMock{}, events, []string{"payments"}, logger)

	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-time.Hour)

	insertFund(t, ctx, pool, "open-fund", true, &future)
	insertFund(t, ctx, pool, "no-expiry-fund", true, nil)
	insertFund(t, ctx, pool, "expired-fund", true, &past)
	insertFund(t, ctx, pool, "deactivated-fund", false, &future)

	all, err := svc.ListAllFunds(ctx)
	require.NoError(t, err)

	byName := fundNames(all)

	for _, name := range []string{"open-fund", "no-expiry-fund", "expired-fund", "deactivated-fund"} {
		assert.Contains(t, byName, name, "the admin listing must not lose a fund")
	}

	// The two ways a fund stops collecting are reported separately: one happened
	// on its own, the other is something a person did.
	assert.True(t, byName["expired-fund"].Expired())
	assert.True(t, byName["expired-fund"].Active, "expiry is not deactivation")
	assert.False(t, byName["deactivated-fund"].Expired())
	assert.False(t, byName["deactivated-fund"].Active)

	assert.False(t, byName["open-fund"].Closed())
	assert.False(t, byName["no-expiry-fund"].Closed(), "a fund with no end date never expires")
	assert.True(t, byName["expired-fund"].Closed())
	assert.True(t, byName["deactivated-fund"].Closed())

	// The public listing keeps the opposite default on purpose: a donor must not
	// be offered a fund that is closed.
	active, err := svc.ListActiveFunds(ctx)
	require.NoError(t, err)

	activeByName := fundNames(active)

	assert.Contains(t, activeByName, "open-fund")
	assert.Contains(t, activeByName, "no-expiry-fund")
	assert.NotContains(t, activeByName, "expired-fund")
	assert.NotContains(t, activeByName, "deactivated-fund")
}

// Closed funds sort last so the working set stays at the top of a list that only
// ever grows.
func TestAdminListingSortsClosedFundsLast(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(store, stubDocumentStorage{}, &mocks.PaymentsProviderMock{}, events, []string{"payments"}, logger)

	past := time.Now().Add(-time.Hour)

	// Inserted closed-first so passing cannot be an accident of insertion order.
	insertFund(t, ctx, pool, "closed-one", true, &past)
	insertFund(t, ctx, pool, "closed-two", false, nil)
	insertFund(t, ctx, pool, "open-one", true, nil)

	all, err := svc.ListAllFunds(ctx)
	require.NoError(t, err)
	require.Len(t, all, 3)

	assert.False(t, all[0].Closed(), "an open fund should lead the list")
	assert.True(t, all[1].Closed())
	assert.True(t, all[2].Closed())
}
