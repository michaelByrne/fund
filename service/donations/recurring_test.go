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

// The recurring completion took the subscription id, the plan, the fund and the
// amount from the browser and wrote all four unchecked.
//
// The fund is the one that cost money. A subscription created against one fund's
// plan could be recorded against another, and every payment on it then joined the
// wrong fund's balance and was paid out to the wrong fund's enrollees.
func TestRecurringDonationsAreVerifiedWithTheProvider(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	// setup makes a fund with a plan, and returns the service, the fund, the plan
	// and a member to donate as.
	setup := func(t *testing.T, provider *mocks.PaymentsProviderMock) (*donations.DonationService, uuid.UUID, donations.DonationPlan, uuid.UUID) {
		t.Helper()

		provider.CreateFundFunc = func(context.Context, string, string) (string, error) {
			return uuid.NewString(), nil
		}
		provider.CreatePlanFunc = func(context.Context, donations.CreatePlan) (string, error) {
			return "PROVIDER-PLAN-" + uuid.NewString(), nil
		}

		svc := donations.NewDonationService(store, stubDocumentStorage{}, newFakeBucket(), provider, events, nil, logger)

		fund, errFund := svc.CreateFund(ctx, donations.Fund{
			Name: uuid.NewString(), Description: "d",
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		})
		require.NoError(t, errFund)

		plan, errPlan := svc.CreateDonationPlan(ctx, donations.CreatePlan{
			Name: uuid.NewString(), AmountCents: 2500,
			IntervalUnit: donations.IntervalUnitMonth, IntervalCount: 1,
			FundID: fund.ID, ProviderFundID: fund.ProviderID,
		})
		require.NoError(t, errPlan)

		return svc, fund.ID, *plan, seedMemberRow(t, ctx, pool)
	}

	donationCount := func(t *testing.T, fundID uuid.UUID) int {
		t.Helper()

		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM donation WHERE fund_id = $1`, fundID).Scan(&n))

		return n
	}

	t.Run("records a subscription that matches its plan and fund", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, plan, memberID := setup(t, provider)

		provider.GetSubscriptionFunc = func(context.Context, string) (*donations.ProviderSubscription, error) {
			return &donations.ProviderSubscription{
				Status: "ACTIVE", ProviderPlanID: plan.ProviderPlanID,
			}, nil
		}

		err := svc.CompleteRecurringDonation(ctx, memberID, donations.RecurringCompletion{
			PlanID:                 uuid.NullUUID{UUID: plan.ID, Valid: true},
			FundID:                 fundID,
			AmountCents:            2500,
			ProviderSubscriptionID: uuid.NewString(),
		})
		require.NoError(t, err)

		assert.Equal(t, 1, donationCount(t, fundID))
	})

	t.Run("refuses a subscription for another fund's plan", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, plan, memberID := setup(t, provider)

		// A real, active subscription -- paying into a plan that is not this
		// fund's. Recording it would send this fund's enrollees money paid to
		// another fund.
		provider.GetSubscriptionFunc = func(context.Context, string) (*donations.ProviderSubscription, error) {
			return &donations.ProviderSubscription{
				Status: "ACTIVE", ProviderPlanID: "PROVIDER-PLAN-SOMEBODY-ELSE",
			}, nil
		}

		err := svc.CompleteRecurringDonation(ctx, memberID, donations.RecurringCompletion{
			PlanID:                 uuid.NullUUID{UUID: plan.ID, Valid: true},
			FundID:                 fundID,
			ProviderSubscriptionID: uuid.NewString(),
		})
		require.ErrorIs(t, err, donations.ErrSubscriptionPlanMismatch)

		assert.Zero(t, donationCount(t, fundID))
	})

	t.Run("refuses a plan belonging to a different fund", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, plan, memberID := setup(t, provider)

		// The subscription genuinely pays into this plan, but the caller claims a
		// different fund. The plan knows which fund it belongs to.
		other, errOther := svc.CreateFund(ctx, donations.Fund{
			Name: uuid.NewString(), Description: "d",
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		})
		require.NoError(t, errOther)

		provider.GetSubscriptionFunc = func(context.Context, string) (*donations.ProviderSubscription, error) {
			return &donations.ProviderSubscription{
				Status: "ACTIVE", ProviderPlanID: plan.ProviderPlanID,
			}, nil
		}

		err := svc.CompleteRecurringDonation(ctx, memberID, donations.RecurringCompletion{
			PlanID:                 uuid.NullUUID{UUID: plan.ID, Valid: true},
			FundID:                 other.ID,
			ProviderSubscriptionID: uuid.NewString(),
		})
		require.ErrorIs(t, err, donations.ErrSubscriptionPlanMismatch)

		assert.Zero(t, donationCount(t, other.ID))
		assert.Zero(t, donationCount(t, fundID))
	})

	t.Run("refuses a subscription the provider does not consider live", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, plan, memberID := setup(t, provider)

		for _, status := range []string{"CANCELLED", "SUSPENDED", "EXPIRED", ""} {
			provider.GetSubscriptionFunc = func(context.Context, string) (*donations.ProviderSubscription, error) {
				return &donations.ProviderSubscription{
					Status: status, ProviderPlanID: plan.ProviderPlanID,
				}, nil
			}

			err := svc.CompleteRecurringDonation(ctx, memberID, donations.RecurringCompletion{
				PlanID:                 uuid.NullUUID{UUID: plan.ID, Valid: true},
				FundID:                 fundID,
				ProviderSubscriptionID: uuid.NewString(),
			})
			require.ErrorIsf(t, err, donations.ErrSubscriptionNotActive, "status %q", status)
		}

		assert.Zero(t, donationCount(t, fundID))
	})

	t.Run("accepts an approved subscription", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, plan, memberID := setup(t, provider)

		// APPROVED is the state the completion flow actually runs in: the donor has
		// authorised it and PayPal has not yet taken the first payment. Refusing it
		// would reject every honest subscription.
		provider.GetSubscriptionFunc = func(context.Context, string) (*donations.ProviderSubscription, error) {
			return &donations.ProviderSubscription{
				Status: "APPROVED", ProviderPlanID: plan.ProviderPlanID,
			}, nil
		}

		err := svc.CompleteRecurringDonation(ctx, memberID, donations.RecurringCompletion{
			PlanID:                 uuid.NullUUID{UUID: plan.ID, Valid: true},
			FundID:                 fundID,
			ProviderSubscriptionID: uuid.NewString(),
		})
		require.NoError(t, err)

		assert.Equal(t, 1, donationCount(t, fundID))
	})

	t.Run("refuses a completion naming no plan", func(t *testing.T) {
		provider := &mocks.PaymentsProviderMock{}
		svc, fundID, _, memberID := setup(t, provider)

		provider.GetSubscriptionFunc = func(context.Context, string) (*donations.ProviderSubscription, error) {
			return &donations.ProviderSubscription{Status: "ACTIVE", ProviderPlanID: "X"}, nil
		}

		// Without a plan there is nothing to check the subscription against.
		err := svc.CompleteRecurringDonation(ctx, memberID, donations.RecurringCompletion{
			FundID:                 fundID,
			ProviderSubscriptionID: uuid.NewString(),
		})
		require.ErrorIs(t, err, donations.ErrSubscriptionPlanMismatch)

		assert.Zero(t, donationCount(t, fundID))
	})
}
