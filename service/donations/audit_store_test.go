package donations_test

import (
	"context"
	"testing"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/finance"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The audit page reads this. It used to read a CSV out of S3 by column position,
// so it could only show a fund as some past run had left it.
func TestFundPaymentsForAudit(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	store := donationsstore.NewDonationStore(pool)

	fundID := seedOnceFund(t, ctx, pool)
	donationID := seedDonationRow(t, ctx, pool, fundID)
	paymentID := uuid.New()

	_, err = pool.Exec(ctx,
		`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, provider_fee_cents)
		 VALUES ($1, $2, 'SALE-1', 5000, 150)`,
		paymentID, donationID)
	require.NoError(t, err)

	t.Run("an unreconciled payment reports as unchecked", func(t *testing.T) {
		payments, errAudit := store.GetFundPaymentsForAudit(ctx, fundID)
		require.NoError(t, errAudit)
		require.Len(t, payments, 1)

		payment := payments[0]

		assert.Nil(t, payment.ReconciledAt, "nothing has checked it yet")
		assert.Equal(t, finance.AuditUnchecked, payment.Verdict())

		// The donor is joined because a payment id is not something anyone can act
		// on; the question is usually about a person.
		assert.NotEmpty(t, payment.DonorName)
		assert.EqualValues(t, 5000, payment.AmountCents)
	})

	t.Run("recording what the provider said makes it checked", func(t *testing.T) {
		status := "COMPLETED"
		amount := int32(5000)

		require.NoError(t, store.SetPaymentReconciliation(ctx, donations.SetPaymentReconciliation{
			PaymentID:           paymentID,
			ProviderStatus:      &status,
			ProviderAmountCents: &amount,
		}))

		payments, errAudit := store.GetFundPaymentsForAudit(ctx, fundID)
		require.NoError(t, errAudit)
		require.Len(t, payments, 1)

		assert.NotNil(t, payments[0].ReconciledAt)
		assert.Equal(t, finance.AuditOK, payments[0].Verdict())
	})

	t.Run("a check that found nothing is still a check", func(t *testing.T) {
		// The provider returned no transaction, which for a recent payment is
		// routine -- its reporting lags by hours. Recorded so the page can tell
		// this apart from never having looked.
		require.NoError(t, store.SetPaymentReconciliation(ctx, donations.SetPaymentReconciliation{
			PaymentID: paymentID,
		}))

		payments, errAudit := store.GetFundPaymentsForAudit(ctx, fundID)
		require.NoError(t, errAudit)

		assert.NotNil(t, payments[0].ReconciledAt)
		assert.Equal(t, finance.AuditMissingAtProvider, payments[0].Verdict())
	})
}
