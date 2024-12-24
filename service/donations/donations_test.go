package donations_test

import (
	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
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

		authMock := mocks.AuthProviderMock{}

		authMock.CreateUserFunc = func(ctx context.Context, username, email string, memberID uuid.UUID) (string, error) {
			return "cognito-id", nil
		}

		nopHandler := slog.NewJSONHandler(io.Discard, nil)
		logger := slog.New(nopHandler)

		donationTestStore := donationsstore.NewDonationStore(pool)
		donationTestService := donations.NewDonationService(donationTestStore, &paymentsMock, logger)

		memberTestStore := membersstore.NewMemberStore(pool)
		memberTestService := members.NewMemberService(memberTestStore, donationTestStore, &authMock, &paymentsMock, logger)

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

		err = donationTestService.DeactivateFund(ctx, fund.ID)
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
