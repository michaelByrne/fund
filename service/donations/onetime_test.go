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

// The one-time donation path used to write the browser's numbers straight to the
// database. The fund balance is what the planner divides between enrollees, and
// the PayPal balance is shared across funds, so an invented donation disbursed
// real money belonging to other funds.
func TestOneTimeDonationsAreVerifiedWithTheProvider(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	newFund := func(t *testing.T, provider *mocks.PaymentsProviderMock) (*donations.DonationService, uuid.UUID, uuid.UUID) {
		t.Helper()

		provider.CreateFundFunc = func(context.Context, string, string) (string, error) {
			return uuid.NewString(), nil
		}

		svc := donations.NewDonationService(store, stubDocumentStorage{}, newFakeBucket(), provider, events, []string{"payments"}, logger)

		// A one-off fund pays out on its end date, so it has to have one.
		endDate := time.Now().Add(30 * 24 * time.Hour)

		fund, errFund := svc.CreateFund(ctx, donations.Fund{
			Name:            uuid.NewString(),
			Description:     "d",
			PayoutFrequency: donations.PayoutFrequencyOnce,
			Expires:         &endDate,
		}, nil)
		require.NoError(t, errFund)

		return svc, fund.ID, seedMemberRow(t, ctx, pool)
	}

	balance := func(t *testing.T, fundID uuid.UUID) int64 {
		t.Helper()

		var cents int64
		errBalance := pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(dp.amount_cents), 0)
			 FROM donation JOIN donation_payment dp ON donation.id = dp.donation_id
			 WHERE donation.fund_id = $1`, fundID).Scan(&cents)
		require.NoError(t, errBalance)

		return cents
	}

	t.Run("records the provider's amount, not the caller's", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, memberID := newFund(t, provider)

		provider.GetOrderFunc = func(context.Context, string) (*donations.ProviderOrder, error) {
			return &donations.ProviderOrder{
				Status:            "COMPLETED",
				FundReferenceID:   fundID.String(),
				ProviderPaymentID: uuid.NewString(),
				AmountCents:       500,
			}, nil
		}

		// The caller claims a hundred times what was captured.
		err := svc.CompleteDonation(ctx, memberID, donations.OneTimeCompletion{
			AmountCents:       50000,
			FundID:            fundID,
			ProviderOrderID:   "order-1",
			ProviderPaymentID: "payment-the-caller-made-up",
		})
		require.NoError(t, err)

		assert.EqualValues(t, 500, balance(t, fundID), "the fund should hold what PayPal captured")
	})

	t.Run("refuses an order the provider has not captured", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, memberID := newFund(t, provider)

		provider.GetOrderFunc = func(context.Context, string) (*donations.ProviderOrder, error) {
			// Created but never paid, which is what an abandoned checkout leaves.
			return &donations.ProviderOrder{
				Status:          "CREATED",
				FundReferenceID: fundID.String(),
			}, nil
		}

		err := svc.CompleteDonation(ctx, memberID, donations.OneTimeCompletion{
			AmountCents:     2500,
			FundID:          fundID,
			ProviderOrderID: "order-2",
		})
		require.ErrorIs(t, err, donations.ErrOrderNotComplete)

		assert.Zero(t, balance(t, fundID), "an uncaptured order must not credit the fund")
	})

	t.Run("refuses an order belonging to another fund", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, memberID := newFund(t, provider)

		provider.GetOrderFunc = func(context.Context, string) (*donations.ProviderOrder, error) {
			// A real, paid order -- for somebody else's fund. The reference id is
			// set by us at creation, so this cannot happen by accident.
			return &donations.ProviderOrder{
				Status:            "COMPLETED",
				FundReferenceID:   uuid.NewString(),
				ProviderPaymentID: uuid.NewString(),
				AmountCents:       5000,
			}, nil
		}

		err := svc.CompleteDonation(ctx, memberID, donations.OneTimeCompletion{
			AmountCents:     5000,
			FundID:          fundID,
			ProviderOrderID: "order-3",
		})
		require.ErrorIs(t, err, donations.ErrOrderFundMismatch)

		assert.Zero(t, balance(t, fundID), "one fund must not be credited with another's money")
	})

	t.Run("a resubmitted completion is recorded once", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, memberID := newFund(t, provider)

		paymentID := uuid.NewString()
		provider.GetOrderFunc = func(context.Context, string) (*donations.ProviderOrder, error) {
			return &donations.ProviderOrder{
				Status:            "COMPLETED",
				FundReferenceID:   fundID.String(),
				ProviderPaymentID: paymentID,
				AmountCents:       1500,
			}, nil
		}

		completion := donations.OneTimeCompletion{
			AmountCents:     1500,
			FundID:          fundID,
			ProviderOrderID: "order-4",
		}

		require.NoError(t, svc.CompleteDonation(ctx, memberID, completion))

		// A double-submitted form is a success, not a 500 telling the donor to try
		// a payment that already went through.
		require.NoError(t, svc.CompleteDonation(ctx, memberID, completion))

		assert.EqualValues(t, 1500, balance(t, fundID), "the same capture must count once")
	})
}

// PayPal redelivers on any non-2xx and on its own schedule, so the recurring
// path receives the same payment more than once as a matter of course.
func TestRedeliveredPaymentsAreCountedOnce(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := donationsstore.NewDonationStore(pool)

	fundID := seedOnceFund(t, ctx, pool)
	donationID := seedDonationRow(t, ctx, pool, fundID)

	payment := donations.InsertDonationPayment{
		ID:                uuid.New(),
		DonationID:        donationID,
		ProviderPaymentID: "PAYPAL-SALE-1",
		AmountCents:       2000,
	}

	first, err := store.InsertDonationPayment(ctx, payment)
	require.NoError(t, err)
	require.NotNil(t, first, "the first delivery records the payment")

	// A different row id, as the handler generates one per delivery, but the same
	// provider payment.
	payment.ID = uuid.New()

	second, err := store.InsertDonationPayment(ctx, payment)
	require.NoError(t, err, "a redelivery is ordinary, not an error")
	assert.Nil(t, second, "a redelivery records nothing, and the caller reads nil as already-done")

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM donation_payment WHERE paypal_payment_id = $1`,
		payment.ProviderPaymentID).Scan(&count))

	assert.Equal(t, 1, count, "the fund balance is computed from these rows, so a second would double-count")
}

func seedOnceFund(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	fundID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
		 VALUES ($1, $2, 'd', $3, 'paypal', 'once', now())`,
		fundID, uuid.NewString(), fundID.String(),
	)
	require.NoError(t, err)

	return fundID
}

func seedMemberRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	memberID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO member (id, bco_name, email) VALUES ($1, $2, $3)`,
		memberID, uuid.NewString(), uuid.NewString()+"@test.test",
	)
	require.NoError(t, err)

	return memberID
}

func seedDonationRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID uuid.UUID) uuid.UUID {
	t.Helper()

	memberID := seedMemberRow(t, ctx, pool)

	donationID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
		 VALUES ($1, true, $2, $3, $4)`,
		donationID, memberID, donationID.String(), fundID,
	)
	require.NoError(t, err)

	return donationID
}
