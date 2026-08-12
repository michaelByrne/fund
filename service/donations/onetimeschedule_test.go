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

// A one-off fund's end date is its payout date: InsertFund anchors next_payment
// to it, and the planner only picks up funds whose next_payment has arrived.
func TestAOneTimeFundPaysOutOnItsEndDate(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	provider := mocks.PaymentsProviderMock{}
	provider.CreateFundFunc = func(context.Context, string, string) (string, error) {
		return uuid.NewString(), nil
	}
	provider.CancelSubscriptionsFunc = func(_ context.Context, ids []string) ([]string, error) {
		return ids, nil
	}

	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(
		donationsstore.NewDonationStore(pool), stubDocumentStorage{}, newFakeBucket(),
		&provider, events, []string{"payments"}, logger,
	)

	create := func(t *testing.T, name string, frequency donations.PayoutFrequency, expires *time.Time) (*donations.Fund, error) {
		t.Helper()

		return svc.CreateFund(ctx, donations.Fund{
			Name: name, Description: "d", Active: true,
			PayoutFrequency: frequency, Expires: expires,
		}, nil)
	}

	day := func(offset int) *time.Time {
		when := time.Now().AddDate(0, 0, offset)

		return &when
	}

	t.Run("the payout date is the end date", func(t *testing.T) {
		end := day(30)

		fund, errCreate := create(t, "once-anchored", donations.PayoutFrequencyOnce, end)
		require.NoError(t, errCreate)

		assert.WithinDuration(t, *end, fund.NextPayment, time.Second)
		assert.WithinDuration(t, *end, fund.NextPaymentAfter(time.Now()), time.Second)
	})

	// Without one, next_payment is NULL and GetFundsDueForPayout requires it to be
	// set -- so the fund would take donations forever and pay out none of them,
	// with nothing anywhere saying so.
	t.Run("a one-time fund without an end date is refused", func(t *testing.T) {
		// Counted rather than compared to zero: the mock accumulates across the
		// subtests above, and what matters is that this call added nothing.
		before := len(provider.CreateFundCalls())

		_, errCreate := create(t, "once-undated", donations.PayoutFrequencyOnce, nil)

		assert.ErrorIs(t, errCreate, donations.ErrOneTimeFundNeedsEndDate)

		// Refused before the provider, so no orphaned catalogue product is left
		// behind for a fund that was never created. Asserted against the provider
		// rather than against the table: moving the check to after CreateFund and
		// before InsertFund would leave the product behind and still insert no row,
		// which a row count alone cannot tell apart.
		assert.Len(t, provider.CreateFundCalls(), before,
			"a refused fund should not have reached paypal")
		assert.Empty(t, fundNamed(t, ctx, pool, "once-undated"))
	})

	// A recurring fund's schedule stands on its own, so an end date really is
	// optional there.
	t.Run("a recurring fund without an end date is fine", func(t *testing.T) {
		fund, errCreate := create(t, "monthly-open-ended", donations.PayoutFrequencyMonthly, nil)
		require.NoError(t, errCreate)

		assert.Nil(t, fund.Expires)
		assert.False(t, fund.NextPayment.IsZero(), "a monthly fund is anchored a month out")
	})

	// The bug: expires moved and next_payment did not, so the fund closed on one
	// date and paid on another.
	t.Run("moving the end date moves the payout", func(t *testing.T) {
		fund, errCreate := create(t, "once-moved", donations.PayoutFrequencyOnce, day(30))
		require.NoError(t, errCreate)

		later := day(60)

		moved := *fund
		moved.Expires = later

		saved, errUpdate := svc.UpdateFund(ctx, moved, nil)
		require.NoError(t, errUpdate)

		assert.WithinDuration(t, *later, saved.NextPayment, time.Second,
			"the payout should follow the end date")

		// Read back, because the adapter is where this would be dropped.
		stored, errRead := svc.GetFundByID(ctx, fund.ID)
		require.NoError(t, errRead)
		assert.WithinDuration(t, *later, stored.NextPayment, time.Second)
	})

	t.Run("clearing the end date of a one-time fund is refused", func(t *testing.T) {
		fund, errCreate := create(t, "once-cleared", donations.PayoutFrequencyOnce, day(30))
		require.NoError(t, errCreate)

		cleared := *fund
		cleared.Expires = nil

		_, errUpdate := svc.UpdateFund(ctx, cleared, nil)
		assert.ErrorIs(t, errUpdate, donations.ErrOneTimeFundNeedsEndDate)

		// And the stored date is untouched, rather than half-applied.
		stored, errRead := svc.GetFundByID(ctx, fund.ID)
		require.NoError(t, errRead)
		require.NotNil(t, stored.Expires)
	})

	// A recurring fund's anchor is the schedule's origin, not an end date, so an
	// edit to expires must leave it alone.
	t.Run("a recurring fund's schedule is not moved by its end date", func(t *testing.T) {
		fund, errCreate := create(t, "monthly-dated", donations.PayoutFrequencyMonthly, day(30))
		require.NoError(t, errCreate)

		moved := *fund
		moved.Expires = day(90)

		saved, errUpdate := svc.UpdateFund(ctx, moved, nil)
		require.NoError(t, errUpdate)

		assert.WithinDuration(t, fund.NextPayment, saved.NextPayment, time.Second,
			"a monthly fund pays on its schedule, not on its end date")
	})

	// Once the batch has been planned, next_payment is NULL and has to stay NULL.
	// Editing anything on the fund afterwards must not schedule a second payout of
	// a fund that has already paid.
	t.Run("an edit after the payout does not schedule another", func(t *testing.T) {
		fund, errCreate := create(t, "once-paid", donations.PayoutFrequencyOnce, day(30))
		require.NoError(t, errCreate)

		_, errNull := pool.Exec(ctx, `UPDATE fund SET next_payment = NULL WHERE id = $1`, fund.ID)
		require.NoError(t, errNull)

		edited, errRead := svc.GetFundByID(ctx, fund.ID)
		require.NoError(t, errRead)

		edited.Description = "an edit after the money went out"

		saved, errUpdate := svc.UpdateFund(ctx, *edited, nil)
		require.NoError(t, errUpdate)

		assert.True(t, saved.NextPayment.IsZero(), "a fund that has paid must not be rescheduled")
	})
}

func fundNamed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) []uuid.UUID {
	t.Helper()

	rows, err := pool.Query(ctx, `SELECT id FROM fund WHERE name = $1`, name)
	require.NoError(t, err)

	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}

	return ids
}

// The setting is chosen when a fund is created, not only afterwards. It used to
// be editable only from the details card, so a fund whose recipients had agreed
// to be named still spent its first moments not naming them.
func TestAFundCanNameItsRecipientsFromTheStart(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	provider := mocks.PaymentsProviderMock{}
	provider.CreateFundFunc = func(context.Context, string, string) (string, error) {
		return uuid.NewString(), nil
	}

	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(
		donationsstore.NewDonationStore(pool), stubDocumentStorage{}, newFakeBucket(),
		&provider, events, []string{"payments"}, logger,
	)

	create := func(t *testing.T, name string, visible bool) *donations.Fund {
		t.Helper()

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: name, Description: "d", Active: true,
			PayoutFrequency:  donations.PayoutFrequencyMonthly,
			EnrolleesVisible: visible,
		}, nil)
		require.NoError(t, errCreate)

		return fund
	}

	t.Run("the setting is stored, not dropped on the way in", func(t *testing.T) {
		fund := create(t, "named-from-the-start", true)

		assert.True(t, fund.EnrolleesVisible)

		// Read back, because the insert adapter is where this would be lost --
		// the column has a default of false and would have quietly supplied it.
		stored, errRead := svc.GetFundByID(ctx, fund.ID)
		require.NoError(t, errRead)
		assert.True(t, stored.EnrolleesVisible)
	})

	// Unticked is the default, and a checkbox submits nothing when unticked.
	t.Run("a fund created without it does not name anybody", func(t *testing.T) {
		fund := create(t, "unnamed-from-the-start", false)

		stored, errRead := svc.GetFundByID(ctx, fund.ID)
		require.NoError(t, errRead)
		assert.False(t, stored.EnrolleesVisible)
	})

	// A fund created already naming people is worth seeing in the history.
	t.Run("creating one that names recipients says so in the feed", func(t *testing.T) {
		fund := create(t, "named-and-recorded", true)

		recorded, errEvents := events.GetFundEvents(ctx, fund.ID, fundevents.DefaultLimit)
		require.NoError(t, errEvents)
		require.NotEmpty(t, recorded)

		created := recorded[len(recorded)-1]

		require.Equal(t, fundevents.KindFundCreated, created.Kind)
		assert.Contains(t, created.Detail, "monthly")
		assert.Contains(t, created.Detail, "recipient names shown to donors")
	})

	// And the phrase stays off every other fund, or it becomes one readers skip.
	t.Run("an ordinary fund's line is not cluttered with it", func(t *testing.T) {
		fund := create(t, "ordinary-line", false)

		recorded, errEvents := events.GetFundEvents(ctx, fund.ID, fundevents.DefaultLimit)
		require.NoError(t, errEvents)
		require.NotEmpty(t, recorded)

		assert.Equal(t, "monthly", recorded[len(recorded)-1].Detail)
	})
}
