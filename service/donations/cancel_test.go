package donations_test

import (
	"context"
	"errors"
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

// Cancelling is the one destructive thing a donor can do to their own record, and
// the one place the app touches somebody's money on their behalf. Both halves
// matter: it must reach PayPal, and it must only ever reach their own.
func TestCancelDonationForMember(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	seedRecurring := func(t *testing.T, donorID uuid.UUID, subscriptionID string) uuid.UUID {
		t.Helper()

		fundID := seedOnceFund(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id, active, provider_subscription_id)
			 VALUES ($1, true, $2, $3, $4, true, $5)`,
			donationID, donorID, uuid.NewString(), fundID, subscriptionID,
		)
		require.NoError(t, errDonation)

		return donationID
	}

	isActive := func(t *testing.T, donationID uuid.UUID) bool {
		t.Helper()

		var active bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT active FROM donation WHERE id = $1`, donationID).Scan(&active))

		return active
	}

	newService := func(provider *mocks.PaymentsProviderMock) *donations.DonationService {
		return donations.NewDonationService(store, stubDocumentStorage{}, provider, events, nil, logger)
	}

	t.Run("cancels at the provider and then here", func(t *testing.T) {
		donor := seedMemberRow(t, ctx, pool)
		subscriptionID := uuid.NewString()
		donationID := seedRecurring(t, donor, subscriptionID)

		var asked []string
		provider := &mocks.PaymentsProviderMock{
			CancelSubscriptionsFunc: func(_ context.Context, ids []string) ([]string, error) {
				asked = ids

				return ids, nil
			},
		}

		require.NoError(t, newService(provider).CancelDonationForMember(ctx, donationID, donor))

		assert.Equal(t, []string{subscriptionID}, asked, "the donor's subscription is what should stop")
		assert.False(t, isActive(t, donationID))
	})

	t.Run("a provider failure leaves the donation running", func(t *testing.T) {
		donor := seedMemberRow(t, ctx, pool)
		donationID := seedRecurring(t, donor, uuid.NewString())

		provider := &mocks.PaymentsProviderMock{
			CancelSubscriptionsFunc: func(context.Context, []string) ([]string, error) {
				return nil, errors.New("paypal is down")
			},
		}

		err := newService(provider).CancelDonationForMember(ctx, donationID, donor)
		require.Error(t, err)

		// Marking it cancelled here would tell the donor their payments had stopped
		// while their card kept being charged, and nothing would correct it.
		assert.True(t, isActive(t, donationID), "a donation PayPal is still charging must not read as cancelled")
	})

	t.Run("a provider that cancels nothing leaves it running", func(t *testing.T) {
		donor := seedMemberRow(t, ctx, pool)
		donationID := seedRecurring(t, donor, uuid.NewString())

		// No error, and nothing cancelled. Checked by membership rather than by
		// counting, so a provider returning some other id does not pass.
		provider := &mocks.PaymentsProviderMock{
			CancelSubscriptionsFunc: func(context.Context, []string) ([]string, error) {
				return []string{"a-different-subscription"}, nil
			},
		}

		require.Error(t, newService(provider).CancelDonationForMember(ctx, donationID, donor))
		assert.True(t, isActive(t, donationID))
	})

	t.Run("refuses somebody else's donation", func(t *testing.T) {
		owner := seedMemberRow(t, ctx, pool)
		stranger := seedMemberRow(t, ctx, pool)
		donationID := seedRecurring(t, owner, uuid.NewString())

		var called bool
		provider := &mocks.PaymentsProviderMock{
			CancelSubscriptionsFunc: func(_ context.Context, ids []string) ([]string, error) {
				called = true

				return ids, nil
			},
		}

		err := newService(provider).CancelDonationForMember(ctx, donationID, stranger)
		require.ErrorIs(t, err, donations.ErrDonationNotYours)

		assert.False(t, called, "somebody else's subscription must never be sent to the provider")
		assert.True(t, isActive(t, donationID))
	})

	t.Run("a donation that does not exist is refused the same way", func(t *testing.T) {
		// Same error as somebody else's, so a member cannot learn which ids exist.
		err := newService(&mocks.PaymentsProviderMock{}).
			CancelDonationForMember(ctx, uuid.New(), seedMemberRow(t, ctx, pool))

		require.ErrorIs(t, err, donations.ErrDonationNotYours)
	})

	t.Run("cancelling twice succeeds", func(t *testing.T) {
		donor := seedMemberRow(t, ctx, pool)
		donationID := seedRecurring(t, donor, uuid.NewString())

		provider := &mocks.PaymentsProviderMock{
			CancelSubscriptionsFunc: func(_ context.Context, ids []string) ([]string, error) {
				return ids, nil
			},
		}

		svc := newService(provider)
		require.NoError(t, svc.CancelDonationForMember(ctx, donationID, donor))

		// A double-submitted form asked for it to be off, and it is off.
		require.NoError(t, svc.CancelDonationForMember(ctx, donationID, donor))
	})

	t.Run("a one-off donation has nothing to cancel", func(t *testing.T) {
		donor := seedMemberRow(t, ctx, pool)
		fundID := seedOnceFund(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id, active)
			 VALUES ($1, false, $2, $3, $4, true)`,
			donationID, donor, uuid.NewString(), fundID,
		)
		require.NoError(t, errDonation)

		err := newService(&mocks.PaymentsProviderMock{}).CancelDonationForMember(ctx, donationID, donor)
		require.ErrorIs(t, err, donations.ErrDonationNotCancellable)
	})
}

// The page has to show a control that matches what the server will do, or a donor
// clicks cancel and gets an error for a donation that was never cancellable.
func TestOnlyLiveRecurringDonationsOfferCancellation(t *testing.T) {
	cases := []struct {
		name string
		row  donations.MemberDonationRow
		want bool
	}{
		{"live recurring", donations.MemberDonationRow{Active: true, Recurring: true, HasSubscription: true}, true},
		{"already ended", donations.MemberDonationRow{Active: false, Recurring: true, HasSubscription: true}, false},
		{"one-off", donations.MemberDonationRow{Active: true, Recurring: false}, false},
		// Recurring, live, and no subscription recorded: there is nothing to send
		// the provider, so the control would promise what it cannot do.
		{"no subscription recorded", donations.MemberDonationRow{Active: true, Recurring: true}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := donations.NewMemberDonation(c.row).Cancellable(); got != c.want {
				t.Errorf("Cancellable() = %v, want %v", got, c.want)
			}
		})
	}
}
