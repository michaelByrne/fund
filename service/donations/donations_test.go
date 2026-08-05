package donations_test

import (
	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/members"
	membersstore "boardfund/service/members/store"
	"boardfund/service/mocks"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	"testing"
)

// stubDocumentStorage satisfies the donations documentStorage interface without
// reaching S3, so bucket creation is a no-op during tests.
type stubDocumentStorage struct{}

func (stubDocumentStorage) CreateFundBucket(ctx context.Context, prefix string, fundID uuid.UUID) error {
	return nil
}

func TestDonationService_DeactivateFund(t *testing.T) {
	providerFundID := "fund-id"

	t.Run("fund is deactivated", func(t *testing.T) {
		ctx := context.Background()

		container, pool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(ctx)

		paymentsMock := mocks.PaymentsProviderMock{}

		paymentsMock.CreateFundFunc = func(ctx context.Context, name, description string) (string, error) {
			return providerFundID, nil
		}

		paymentsMock.CancelSubscriptionsFunc = func(ctx context.Context, ids []string) ([]string, error) {
			return ids, nil
		}

		paymentsMock.CreatePlanFunc = func(ctx context.Context, createPlan donations.CreatePlan) (string, error) {
			return "provider-plan-id", nil
		}

		nopHandler := slog.NewJSONHandler(io.Discard, nil)
		logger := slog.New(nopHandler)

		donationTestStore := donationsstore.NewDonationStore(pool)
		fundEvents := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
		donationTestService := donations.NewDonationService(donationTestStore, stubDocumentStorage{}, &paymentsMock, fundEvents, []string{"payments"}, logger)

		memberTestStore := membersstore.NewMemberStore(pool)
		memberTestService := members.NewMemberService(memberTestStore, donationTestStore, &paymentsMock, logger)

		createFund := donations.Fund{
			Name:            "Test Fund",
			Description:     "Test Description",
			PayoutFrequency: donations.PayoutFrequencyMonthly,
			Active:          true,
			GoalCents:       10000,
		}

		fund, err := donationTestService.CreateFund(ctx, createFund)
		require.NoError(t, err)

		createMember := members.CreateMember{
			FirstName: "Test",
			LastName:  "User",
			Email:     "test@test.org",
			BCOName:   "gofreescout",
		}

		member, err := memberTestService.CreateMember(ctx, createMember)
		require.NoError(t, err)

		completeDonationOne := donations.OneTimeCompletion{
			AmountCents:       1000,
			ProviderOrderID:   "provider-order-id",
			FundID:            fund.ID,
			ProviderPaymentID: "provider-payment-id",
		}

		err = donationTestService.CompleteDonation(ctx, member.ID, completeDonationOne)
		require.NoError(t, err)

		createPlan := donations.CreatePlan{
			Name:           "Test Plan",
			Description:    "Test Description",
			AmountCents:    1000,
			ProviderFundID: "provider-fund-id",
			IntervalUnit:   donations.IntervalUnitMonth,
			IntervalCount:  1,
			FundID:         fund.ID,
		}

		plan, err := donationTestService.CreateDonationPlan(ctx, createPlan)
		require.NoError(t, err)

		completeDonationTwo := donations.RecurringCompletion{
			PlanID: uuid.NullUUID{
				UUID:  plan.ID,
				Valid: true,
			},
			ProviderOrderID:        "provider-order-id",
			ProviderSubscriptionID: "provider-subscription-id",
			AmountCents:            10000,
			FundID:                 fund.ID,
		}

		err = donationTestService.CompleteRecurringDonation(ctx, member.ID, completeDonationTwo)
		require.NoError(t, err)

		err = donationTestService.DeactivateFund(ctx, fund.ID, member.ID)
		require.NoError(t, err)

		fund, err = donationTestService.GetFundByID(ctx, fund.ID)
		require.NoError(t, err)

		assert.False(t, fund.Active)

		memberWithDonations, err := memberTestService.GetMemberWithDonations(ctx, member.ID)
		require.NoError(t, err)

		for _, donation := range memberWithDonations.Donations {
			assert.False(t, donation.Active)
		}

		argIDs := paymentsMock.CancelSubscriptionsCalls()[0].Ids
		require.Len(t, argIDs, 1)

		assert.Equal(t, completeDonationTwo.ProviderSubscriptionID, argIDs[0])
	})
}

// The provider is called before anything is written, so a cancellation that
// fails leaves the fund open rather than closed-but-still-billing. The previous
// order committed first, and nothing retried the cancellation afterwards.
func TestDeactivateFundLeavesTheFundOpenIfCancellationFails(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	paymentsMock := mocks.PaymentsProviderMock{}
	paymentsMock.CreateFundFunc = func(ctx context.Context, name, description string) (string, error) {
		return "provider-fund-id", nil
	}
	paymentsMock.CreatePlanFunc = func(ctx context.Context, plan donations.CreatePlan) (string, error) {
		return "provider-plan-id", nil
	}

	donationStore := donationsstore.NewDonationStore(pool)
	fundEvents := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(donationStore, stubDocumentStorage{}, &paymentsMock, fundEvents, []string{"payments"}, logger)

	memberStore := membersstore.NewMemberStore(pool)
	memberSvc := members.NewMemberService(memberStore, donationStore, &paymentsMock, logger)

	fund, err := svc.CreateFund(ctx, donations.Fund{
		Name: "Test Fund", Description: "d",
		PayoutFrequency: donations.PayoutFrequencyMonthly, Active: true,
	})
	require.NoError(t, err)

	member, err := memberSvc.CreateMember(ctx, members.CreateMember{
		FirstName: "Test", LastName: "User", Email: "cancelfail@test.org", BCOName: "cancelfail",
	})
	require.NoError(t, err)

	plan, err := svc.CreateDonationPlan(ctx, donations.CreatePlan{
		Name: "Plan", AmountCents: 1000, ProviderFundID: "pf",
		IntervalUnit: donations.IntervalUnitMonth, IntervalCount: 1, FundID: fund.ID,
	})
	require.NoError(t, err)

	require.NoError(t, svc.CompleteRecurringDonation(ctx, member.ID, donations.RecurringCompletion{
		PlanID:                 uuid.NullUUID{UUID: plan.ID, Valid: true},
		ProviderOrderID:        "order",
		ProviderSubscriptionID: "sub-that-will-not-cancel",
		AmountCents:            1000,
		FundID:                 fund.ID,
	}))

	// The provider refuses: nothing comes back cancelled.
	paymentsMock.CancelSubscriptionsFunc = func(ctx context.Context, ids []string) ([]string, error) {
		return nil, nil
	}

	err = svc.DeactivateFund(ctx, fund.ID, member.ID)
	require.Error(t, err, "a fund must not close while its subscriptions are still live")

	after, err := svc.GetFundByID(ctx, fund.ID)
	require.NoError(t, err)
	assert.True(t, after.Active, "the fund should still be open")

	// And the donation is still running, so the donor is not paying into a fund
	// the admin believes is closed.
	withDonations, err := memberSvc.GetMemberWithDonations(ctx, member.ID)
	require.NoError(t, err)
	require.NotEmpty(t, withDonations.Donations)

	for _, donation := range withDonations.Donations {
		assert.True(t, donation.Active, "donations should be untouched when cancellation fails")
	}
}
