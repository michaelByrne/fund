package store

import (
	"boardfund/pg"
	"boardfund/service/donations"
	"boardfund/service/members"
	memberstore "boardfund/service/members/store"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDonationStore_CreateDonationPlan(t *testing.T) {
	t.Run("should successfully create and fetch a donation plan", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		store := NewDonationStore(connPool)

		insertFund := donations.InsertFund{
			ID:              uuid.New(),
			Name:            "test",
			Description:     "test",
			ProviderID:      "test-provider",
			Active:          true,
			ProviderName:    "paypal",
			PayoutFrequency: "monthly",
		}

		fund, err := store.InsertFund(context.Background(), insertFund)
		require.NoError(t, err)

		donationPlan := donations.UpsertDonationPlan{
			ID:             uuid.New(),
			Name:           "test",
			AmountCents:    100,
			IntervalUnit:   "MONTH",
			IntervalCount:  1,
			ProviderPlanID: "test-provider-plan",
			Active:         true,
			FundID:         fund.ID,
		}

		newDonationPlan, err := store.UpsertDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		donationPlanByIDOut, err := store.GetDonationPlanByID(context.Background(), newDonationPlan.ID)
		require.NoError(t, err)

		assert.Equal(t, newDonationPlan.ID, donationPlanByIDOut.ID)
		assert.Equal(t, donationPlan.Name, donationPlanByIDOut.Name)
		assert.Equal(t, donationPlan.AmountCents, donationPlanByIDOut.AmountCents)
		assert.Equal(t, donationPlan.IntervalUnit, donationPlanByIDOut.IntervalUnit)
		assert.Equal(t, donationPlan.IntervalCount, donationPlanByIDOut.IntervalCount)
	})
}

func TestDonationStore_CreateDonationPayment(t *testing.T) {
	t.Run("should successfully create and fetch a donation payment", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		donationStore := NewDonationStore(connPool)
		newMembers := memberstore.NewMemberStore(connPool)

		upsertFund := donations.InsertFund{
			ID:              uuid.New(),
			Name:            "test",
			Description:     "test",
			ProviderID:      "test-provider",
			Active:          true,
			ProviderName:    "paypal",
			PayoutFrequency: "monthly",
		}

		fund, err := donationStore.InsertFund(context.Background(), upsertFund)
		require.NoError(t, err)

		donationPlan := donations.UpsertDonationPlan{
			ID:             uuid.New(),
			Name:           "test",
			AmountCents:    100,
			IntervalUnit:   "MONTH",
			IntervalCount:  1,
			ProviderPlanID: "test-provider-plan",
			Active:         true,
			FundID:         fund.ID,
		}

		newDonationPlan, err := donationStore.UpsertDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		member := members.UpsertMember{
			ID:        uuid.New(),
			Email:     "fake@fake.com",
			BCOName:   "gofreescout",
			IPAddress: "127.0.0.1",
		}

		newMember, err := newMembers.UpsertMember(context.Background(), member)
		require.NoError(t, err)

		donation := donations.InsertDonation{
			ID: uuid.New(),
			PlanID: uuid.NullUUID{
				UUID:  newDonationPlan.ID,
				Valid: true,
			},
			DonorID: newMember.ID,
			FundID:  fund.ID,
		}

		newDonation, err := donationStore.InsertDonation(context.Background(), donation)
		require.NoError(t, err)

		donationPayment := donations.InsertDonationPayment{
			ID:                uuid.New(),
			DonationID:        newDonation.ID,
			ProviderPaymentID: "PAY-123456789",
			AmountCents:       100,
		}

		newDonationPayment, err := donationStore.InsertDonationPayment(context.Background(), donationPayment)
		require.NoError(t, err)

		donationPaymentByIDOut, err := donationStore.GetDonationPaymentByID(context.Background(), newDonationPayment.ID)
		require.NoError(t, err)

		assert.Equal(t, newDonationPayment.ID, donationPaymentByIDOut.ID)
		assert.Equal(t, donationPayment.DonationID, donationPaymentByIDOut.DonationID)
		assert.Equal(t, donationPayment.ProviderPaymentID, donationPaymentByIDOut.ProviderPaymentID)
		assert.Equal(t, donationPayment.AmountCents, donationPaymentByIDOut.AmountCents)
	})
}
