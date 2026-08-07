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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fund becomes due on the first cron run after its anchor, and the anchor used
// to carry the time of day it was created at. A daily fund created at 22:00 was
// not due at the 09:00 run the next morning and lost its first day, while one
// created at 08:00 ran as expected -- the same schedule behaving differently
// because of when somebody filled in the form.
func TestNewFundsAreAnchoredToMidnight(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	provider := &mocks.PaymentsProviderMock{
		CreateFundFunc: func(context.Context, string, string) (string, error) {
			return uuid.NewString(), nil
		},
	}

	svc := donations.NewDonationService(
		donationsstore.NewDonationStore(pool), stubDocumentStorage{}, provider,
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger,
	)

	nextPayment := func(t *testing.T, fundID uuid.UUID) time.Time {
		t.Helper()

		var next time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT next_payment FROM fund WHERE id = $1`, fundID).Scan(&next))

		return next.UTC()
	}

	for _, frequency := range []donations.PayoutFrequency{
		donations.PayoutFrequencyDaily,
		donations.PayoutFrequencyMonthly,
	} {
		t.Run(string(frequency), func(t *testing.T) {
			fund, errFund := svc.CreateFund(ctx, donations.Fund{
				Name: uuid.NewString(), Description: "d", PayoutFrequency: frequency,
			})
			require.NoError(t, errFund)

			next := nextPayment(t, fund.ID)

			// Midnight UTC, whatever the hour the fund was created at. Otherwise a
			// fund created after the cron runs waits an extra period.
			assert.Zero(t, next.Hour(), "anchored at %s, want midnight", next.Format(time.RFC3339))
			assert.Zero(t, next.Minute())
			assert.Zero(t, next.Second())
			assert.Zero(t, next.Nanosecond())

			// And still in the future: a fund must not be due the instant it exists.
			assert.True(t, next.After(time.Now().UTC()),
				"anchored at %s, which is not in the future", next.Format(time.RFC3339))
		})
	}

	t.Run("a daily fund is due at the next day's run", func(t *testing.T) {
		fund, errFund := svc.CreateFund(ctx, donations.Fund{
			Name: uuid.NewString(), Description: "d",
			PayoutFrequency: donations.PayoutFrequencyDaily,
		})
		require.NoError(t, errFund)

		// The cron runs at 09:00 UTC. Tomorrow's run has to find it due, which is
		// the whole point: anchored at 22:00 it would not have been.
		tomorrowsRun := time.Now().UTC().Truncate(24 * time.Hour).
			Add(24 * time.Hour).Add(9 * time.Hour)

		assert.True(t, nextPayment(t, fund.ID).Before(tomorrowsRun),
			"a daily fund created today should be due at tomorrow's 09:00 run")
	})
}
