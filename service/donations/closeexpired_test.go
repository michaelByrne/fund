package donations_test

import (
	"context"
	"errors"
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

func TestCloseExpiredFunds(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	newSvc := func(provider *mocks.PaymentsProviderMock) *donations.DonationService {
		store := donationsstore.NewDonationStore(pool)
		events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

		return donations.NewDonationService(store, stubDocumentStorage{}, newFakeBucket(), provider, events, []string{"payments"}, logger)
	}

	isActive := func(t *testing.T, name string) bool {
		t.Helper()

		var active bool
		err := pool.QueryRow(ctx, `SELECT active FROM fund WHERE name = $1`, name).Scan(&active)
		require.NoError(t, err)

		return active
	}

	// A recurring donation is what makes DeactivateFund call the provider at all:
	// with none, there is nothing to cancel and the fund closes without a network
	// call.
	seedRecurringDonation := func(t *testing.T, fundName string) {
		t.Helper()

		donorID := seedTestMember(t, ctx, pool)
		donationID := uuid.New()

		_, err := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, active, donor_id, provider_order_id, provider_subscription_id, fund_id)
			 SELECT $1, true, true, $2, $3, $4, id FROM fund WHERE name = $5`,
			donationID, donorID, donationID.String(), "SUB-"+donationID.String(), fundName,
		)
		require.NoError(t, err)
	}

	t.Run("an expired fund is closed and an open one is left alone", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		provider.CancelSubscriptionsFunc = func(ctx context.Context, ids []string) ([]string, error) {
			return ids, nil
		}

		svc := newSvc(provider)

		past := time.Now().Add(-time.Hour)
		future := time.Now().Add(24 * time.Hour)

		insertFund(t, ctx, pool, "expired-closes", true, &past)
		insertFund(t, ctx, pool, "open-stays", true, &future)
		insertFund(t, ctx, pool, "endless-stays", true, nil)

		closed, err := svc.CloseExpiredFunds(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, closed)

		assert.False(t, isActive(t, "expired-closes"), "an expired fund must stop collecting")
		assert.True(t, isActive(t, "open-stays"))
		assert.True(t, isActive(t, "endless-stays"), "a fund with no end date never expires")
	})

	t.Run("closing is recorded with no actor", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		provider.CancelSubscriptionsFunc = func(ctx context.Context, ids []string) ([]string, error) {
			return ids, nil
		}

		svc := newSvc(provider)

		past := time.Now().Add(-time.Hour)
		fundID := insertFund(t, ctx, pool, "expired-event", true, &past)

		_, err := svc.CloseExpiredFunds(ctx)
		require.NoError(t, err)

		// Nobody closed this, the date did. Attributing it to a person would be
		// wrong, and a zero uuid would fail fund_event's foreign key to member --
		// so this is also what proves the actor is genuinely nullable end to end.
		var actor *string
		err = pool.QueryRow(ctx,
			`SELECT actor_member_id::text FROM fund_event
			 WHERE fund_id = $1 AND kind = 'fund_closed'`, fundID).Scan(&actor)
		require.NoError(t, err, "closing an expired fund must record an event")
		assert.Nil(t, actor, "an automatic closure has no actor")
	})

	t.Run("a fund whose subscriptions will not cancel stays open", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		provider.CancelSubscriptionsFunc = func(ctx context.Context, ids []string) ([]string, error) {
			return nil, errors.New("paypal is down")
		}

		svc := newSvc(provider)

		past := time.Now().Add(-time.Hour)
		insertFund(t, ctx, pool, "provider-down", true, &past)

		seedRecurringDonation(t, "provider-down")

		closed, err := svc.CloseExpiredFunds(ctx)

		// The run itself succeeds: one fund failing must not abandon the others.
		require.NoError(t, err)
		assert.Zero(t, closed)

		// Left open deliberately. A fund closed locally while donors keep being
		// charged is invisible; one that failed to close is not, and is retried.
		assert.True(t, isActive(t, "provider-down"))
	})
}

// Expiry is one date doing two jobs on a one-off fund: pay out, and close. Two
// crons run those, so without a guard the closure can win -- and a closed fund is
// skipped by the planner, which strands the donations it collected.
func TestAOneTimeFundIsNotClosedBeforeItPays(t *testing.T) {
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

	svc := donations.NewDonationService(
		donationsstore.NewDonationStore(pool), stubDocumentStorage{}, newFakeBucket(),
		&provider, fundevents.NewService(fundeventstore.NewEventStore(pool), logger),
		[]string{"payments"}, logger,
	)

	expired := func(t *testing.T, name string, frequency donations.PayoutFrequency) uuid.UUID {
		t.Helper()

		end := time.Now().AddDate(0, 0, 30)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: name, Description: "d", Active: true,
			PayoutFrequency: frequency, Expires: &end,
		}, nil)
		require.NoError(t, errCreate)

		// Pushed into the past directly, rather than creating with a past date:
		// next_payment is anchored at insert and this leaves that relationship
		// intact while making the fund due.
		_, errExpire := pool.Exec(ctx,
			`UPDATE fund SET expires = now() - INTERVAL '1 hour' WHERE id = $1`, fund.ID)
		require.NoError(t, errExpire)

		return fund.ID
	}

	listed := func(t *testing.T, fundID uuid.UUID) bool {
		t.Helper()

		found, errList := svc.ListExpiredOpenFunds(ctx)
		require.NoError(t, errList)

		for _, fund := range found {
			if fund.ID == fundID {
				return true
			}
		}

		return false
	}

	t.Run("a payout still owed keeps the fund open", func(t *testing.T) {
		fundID := expired(t, "once-owed", donations.PayoutFrequencyOnce)

		assert.False(t, listed(t, fundID),
			"closing it now would strand whatever it collected")

		closed, errClose := svc.CloseExpiredFunds(ctx)
		require.NoError(t, errClose)
		assert.Equal(t, 0, closed)
	})

	// next_payment IS NULL is what "the payout has been dealt with" looks like:
	// the planner nulls it for a 'once' fund once a batch exists, and also when
	// there was nobody payable to plan for.
	t.Run("once the payout is dealt with it closes", func(t *testing.T) {
		fundID := expired(t, "once-paid-then-closed", donations.PayoutFrequencyOnce)

		_, errNull := pool.Exec(ctx, `UPDATE fund SET next_payment = NULL WHERE id = $1`, fundID)
		require.NoError(t, errNull)

		assert.True(t, listed(t, fundID))

		closed, errClose := svc.CloseExpiredFunds(ctx)
		require.NoError(t, errClose)
		assert.GreaterOrEqual(t, closed, 1)

		var active bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT active FROM fund WHERE id = $1`, fundID).Scan(&active))
		assert.False(t, active)
	})

	// A recurring fund always has a next_payment, so the guard must not apply to
	// it -- otherwise no monthly fund would ever close on its end date.
	t.Run("a recurring fund still closes on its end date", func(t *testing.T) {
		fundID := expired(t, "monthly-closes", donations.PayoutFrequencyMonthly)

		assert.True(t, listed(t, fundID), "a monthly fund always has a next payment")

		closed, errClose := svc.CloseExpiredFunds(ctx)
		require.NoError(t, errClose)
		assert.GreaterOrEqual(t, closed, 1)
	})
}
