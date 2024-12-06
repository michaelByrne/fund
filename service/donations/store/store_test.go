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

func TestDonationStore_CreateDonation(t *testing.T) {
	t.Run("should successfully create and fetch a donation", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		donationStore := NewDonationStore(connPool)
		memberStore := memberstore.NewMemberStore(connPool)

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
			ID:                  uuid.New(),
			MemberProviderEmail: "fake@fake.com",
			BCOName:             "gofreescout",
			IPAddress:           "172.0.0.1",
		}

		newMember, err := memberStore.UpsertMember(context.Background(), member)
		require.NoError(t, err)

		donation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newMember.ID,
		}

		newDonation, err := donationStore.InsertRecurringDonation(context.Background(), donation)
		require.NoError(t, err)

		donationByIDOut, err := donationStore.GetDonationByID(context.Background(), newDonation.ID)
		require.NoError(t, err)

		assert.Equal(t, newDonation.ID, donationByIDOut.ID)
		assert.Equal(t, donation.DonationPlanID, donationByIDOut.DonationPlanID)
		assert.Equal(t, donation.DonorID, donationByIDOut.DonorID)
	})
}

func TestDonationStore_GetDonationsByMemberPaypalEmail(t *testing.T) {
	t.Run("should successfully get donations by member paypal email", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		donationStore := NewDonationStore(connPool)
		memberStore := memberstore.NewMemberStore(connPool)

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

		firstMember := members.UpsertMember{
			ID:                  uuid.New(),
			MemberProviderEmail: "fake@fake.com",
			BCOName:             "gofreescout",
			IPAddress:           "127.0.0.1",
		}

		secondMember := members.UpsertMember{
			ID:                  uuid.New(),
			MemberProviderEmail: "dummy@dummy.com",
			BCOName:             "gofreescout",
			IPAddress:           "127.0.0.1",
		}

		newFirstMember, err := memberStore.UpsertMember(context.Background(), firstMember)
		require.NoError(t, err)

		newSecondMember, err := memberStore.UpsertMember(context.Background(), secondMember)
		require.NoError(t, err)

		firstDonation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		secondDonation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newSecondMember.ID,
		}

		thirdDonation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		newFirstDonation, err := donationStore.InsertRecurringDonation(context.Background(), firstDonation)
		require.NoError(t, err)

		_, err = donationStore.InsertRecurringDonation(context.Background(), secondDonation)
		require.NoError(t, err)

		newThirdDonation, err := donationStore.InsertRecurringDonation(context.Background(), thirdDonation)
		require.NoError(t, err)

		donationsByEmail, err := donationStore.GetDonationsByMemberPaypalEmail(context.Background(), firstMember.MemberProviderEmail)
		require.NoError(t, err)

		assert.Len(t, donationsByEmail, 2)

		expectedDonations := []donations.Donation{*newFirstDonation, *newThirdDonation}

		assert.ElementsMatch(t, expectedDonations, donationsByEmail)
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
			ID:                  uuid.New(),
			MemberProviderEmail: "fake@fake.com",
			BCOName:             "gofreescout",
			IPAddress:           "127.0.0.1",
		}

		newMember, err := newMembers.UpsertMember(context.Background(), member)
		require.NoError(t, err)

		donation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newMember.ID,
		}

		newDonation, err := donationStore.InsertRecurringDonation(context.Background(), donation)
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

func TestDonationStore_GetDonationPaymentsByMemberPaypalEmail(t *testing.T) {
	t.Run("should successfully get donation payments by member paypal email", func(t *testing.T) {
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

		firstMember := members.UpsertMember{
			ID:                  uuid.New(),
			MemberProviderEmail: "fake@fake.com",
			BCOName:             "gofreescout",
			IPAddress:           "127.0.0.1",
		}

		secondMember := members.UpsertMember{
			ID:                  uuid.New(),
			MemberProviderEmail: "dummy@dummy.com",
			BCOName:             "gofreescout",
			IPAddress:           "127.0.0.1",
		}

		newFirstMember, err := newMembers.UpsertMember(context.Background(), firstMember)
		require.NoError(t, err)

		newSecondMember, err := newMembers.UpsertMember(context.Background(), secondMember)
		require.NoError(t, err)

		firstDonation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		secondDonation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newSecondMember.ID,
		}

		thirdDonation := donations.InsertRecurringDonation{
			ID:             uuid.New(),
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		newFirstDonation, err := donationStore.InsertRecurringDonation(context.Background(), firstDonation)
		require.NoError(t, err)

		newSecondDonation, err := donationStore.InsertRecurringDonation(context.Background(), secondDonation)
		require.NoError(t, err)

		newThirdDonation, err := donationStore.InsertRecurringDonation(context.Background(), thirdDonation)
		require.NoError(t, err)

		firstDonationPayment := donations.InsertDonationPayment{
			ID:                uuid.New(),
			DonationID:        newFirstDonation.ID,
			ProviderPaymentID: "PAY-123456789",
			AmountCents:       100,
		}

		secondDonationPayment := donations.InsertDonationPayment{
			ID:                uuid.New(),
			DonationID:        newSecondDonation.ID,
			ProviderPaymentID: "PAY-987654321",
			AmountCents:       100,
		}

		thirdDonationPayment := donations.InsertDonationPayment{
			ID:                uuid.New(),
			DonationID:        newThirdDonation.ID,
			ProviderPaymentID: "PAY-123456789",
			AmountCents:       100,
		}

		fourthDonationPayment := donations.InsertDonationPayment{
			ID:                uuid.New(),
			DonationID:        newFirstDonation.ID,
			ProviderPaymentID: "PAY-987654321",
			AmountCents:       100,
		}

		firstNewDonationPayment, err := donationStore.InsertDonationPayment(context.Background(), firstDonationPayment)
		require.NoError(t, err)

		_, err = donationStore.InsertDonationPayment(context.Background(), secondDonationPayment)
		require.NoError(t, err)

		thirdNewDonationPayment, err := donationStore.InsertDonationPayment(context.Background(), thirdDonationPayment)
		require.NoError(t, err)

		fourthNewDonationPayment, err := donationStore.InsertDonationPayment(context.Background(), fourthDonationPayment)
		require.NoError(t, err)

		donationPaymentsByEmail, err := donationStore.GetDonationPaymentsByMemberPaypalEmail(context.Background(), firstMember.MemberProviderEmail)

		expectedPayments := []donations.DonationPayment{*firstNewDonationPayment, *fourthNewDonationPayment, *thirdNewDonationPayment}

		assert.ElementsMatch(t, expectedPayments, donationPaymentsByEmail)
	})
}
