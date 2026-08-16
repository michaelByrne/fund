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

// The archive's four money figures have to reconcile: collected, less what
// PayPal took, less what recipients received, is what the fund still holds.
//
// It used to compare a gross collected figure against a net paid-out one, so
// every fee ever charged appeared as "collected but not paid out" -- on the one
// line of that page that means money never reached anybody. A fund that
// disbursed everything it could still showed a shortfall.
func TestTheArchiveLedgerAddsUp(t *testing.T) {
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

	end := time.Now().Add(24 * time.Hour)

	fund, err := svc.CreateFund(ctx, donations.Fund{
		Name: "ledger", Description: "d", Active: true,
		PayoutFrequency: donations.PayoutFrequencyOnce, Expires: &end,
	}, nil)
	require.NoError(t, err)

	// $10.00 given, of which PayPal kept 54c on the way in.
	donorID := seedTestMember(t, ctx, pool)
	donationID := uuid.New()

	_, err = pool.Exec(ctx,
		`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
		 VALUES ($1, false, $2, $3, $4)`, donationID, donorID, uuid.NewString(), fund.ID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, provider_fee_cents)
		 VALUES ($1, $2, $3, 1000, 54)`, uuid.New(), donationID, uuid.NewString())
	require.NoError(t, err)

	// Two payouts of 448c, each costing 25c to send: 946 in, 946 out.
	enrollmentIDs := make([]uuid.UUID, 0, 2)
	for range 2 {
		memberID := seedTestMember(t, ctx, pool)
		enrollmentID := uuid.New()

		_, errEnroll := pool.Exec(ctx,
			`INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, paypal_email, active)
			 VALUES ($1, $2, $3, now() - INTERVAL '1 day', $4, true)`,
			enrollmentID, fund.ID, memberID, memberID.String()+"@paypal.test")
		require.NoError(t, errEnroll)

		enrollmentIDs = append(enrollmentIDs, enrollmentID)
	}

	batchID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO batch_payout (id, fund_id, status, amount_cents, num_enrollments, payout_date, sender_batch_id)
		 VALUES ($1, $2, 'paid', 896, 2, now(), $3)`, batchID, fund.ID, uuid.New())
	require.NoError(t, err)

	for _, enrollmentID := range enrollmentIDs {
		_, errPayout := pool.Exec(ctx,
			`INSERT INTO payout (id, batch_id, fund_enrollment_id, amount_cents, provider_fee_cents, status, payout_date, destination_email)
			 VALUES ($1, $2, $3, 448, 25, 'paid', now(), $4)`, uuid.New(), batchID, enrollmentID, uuid.NewString()+"@paypal.test")
		require.NoError(t, errPayout)
	}

	require.NoError(t, svc.DeactivateFund(ctx, fund.ID, nil))

	closed, err := svc.GetClosedFund(ctx, fund.ID)
	require.NoError(t, err)

	assert.Equal(t, int32(1000), closed.Stats.TotalDonated, "what donors gave")
	assert.Equal(t, int64(896), closed.Payouts.TotalPaidCents, "what recipients received")

	// 54 in and 25 x 2 out. The fund absorbs the fee in both directions.
	assert.Equal(t, int64(104), closed.Payouts.ProviderFeeCents, "what paypal took")

	// The whole point: this fund disbursed everything it could.
	assert.Equal(t, int64(0), closed.Undisbursed(),
		"collected less fees less paid out is what the fund still holds")

	// And the figures shown on the page reconcile without the reader taking any
	// of them on trust.
	assert.Equal(t,
		int64(closed.Stats.TotalDonated),
		closed.Payouts.ProviderFeeCents+closed.Payouts.TotalPaidCents+closed.Undisbursed())
}

// A real remainder still shows. The fix must not make the figure always zero,
// which would hide the case it exists for.
func TestARealRemainderStillShows(t *testing.T) {
	closed := donations.ClosedFund{
		Fund: donations.Fund{Stats: donations.FundStats{TotalDonated: 1000}},
		Payouts: donations.PayoutStats{
			TotalPaidCents:   800,
			ProviderFeeCents: 104,
		},
	}

	assert.Equal(t, int64(96), closed.Undisbursed(),
		"money the fund kept is still reported")
}
