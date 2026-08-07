package donations_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cancelling produces two true accounts of one event: ours when we ask PayPal to
// stop, and PayPal's when it tells us it has. The feed showed both, so a donor who
// cancelled once appeared in the history twice -- "cancelled by donor" and
// "subscription cancelled at provider", a minute apart.
func TestACancellationIsRecordedOnce(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	provider := &mocks.PaymentsProviderMock{
		CancelSubscriptionsFunc: func(_ context.Context, ids []string) ([]string, error) {
			return ids, nil
		},
	}

	svc := donations.NewDonationService(store, stubDocumentStorage{}, newFakeBucket(), provider, events, nil, logger)
	handlers := donations.NewHandlers(store, events, logger)

	donor := seedMemberRow(t, ctx, pool)
	fundID := seedOnceFund(t, ctx, pool)
	subscriptionID := uuid.NewString()
	donationID := uuid.New()

	_, err = pool.Exec(ctx,
		`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id, active, provider_subscription_id)
		 VALUES ($1, true, $2, $3, $4, true, $5)`,
		donationID, donor, uuid.NewString(), fundID, subscriptionID,
	)
	require.NoError(t, err)

	cancellations := func(t *testing.T) []string {
		t.Helper()

		rows, errQuery := pool.Query(ctx,
			`SELECT detail FROM fund_event WHERE fund_id = $1 AND kind = 'donation_cancelled'`, fundID)
		require.NoError(t, errQuery)
		defer rows.Close()

		var details []string
		for rows.Next() {
			var detail string
			require.NoError(t, rows.Scan(&detail))
			details = append(details, detail)
		}

		return details
	}

	// The donor cancels.
	require.NoError(t, svc.CancelDonationForMember(ctx, donationID, donor))
	require.Len(t, cancellations(t), 1)

	// PayPal echoes it back, which is what actually happens a moment later.
	require.NoError(t, handlers.SubscriptionEndedForTest(
		[]byte(`{"id":"`+subscriptionID+`","status":"CANCELLED"}`)))

	details := cancellations(t)

	assert.Len(t, details, 1, "one cancellation should read as one cancellation")

	// Ours wins because it got there first, and it is the one that knows who did
	// it.
	assert.Equal(t, "cancelled by donor", details[0])
}

// Two different subscriptions cancelling must not collapse into one entry, which
// is what a shared or empty key would do.
func TestSeparateCancellationsAreSeparateEvents(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	handlers := donations.NewHandlers(store, events, logger)

	fundID := seedOnceFund(t, ctx, pool)

	for i := 0; i < 2; i++ {
		donor := seedMemberRow(t, ctx, pool)
		subscriptionID := uuid.NewString()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id, active, provider_subscription_id)
			 VALUES ($1, true, $2, $3, $4, true, $5)`,
			uuid.New(), donor, uuid.NewString(), fundID, subscriptionID,
		)
		require.NoError(t, errDonation)

		require.NoError(t, handlers.SubscriptionEndedForTest(
			[]byte(`{"id":"`+subscriptionID+`","status":"CANCELLED"}`)))
	}

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM fund_event WHERE fund_id = $1 AND kind = 'donation_cancelled'`,
		fundID).Scan(&count))

	assert.Equal(t, 2, count, "two donors cancelling is two events")
}
