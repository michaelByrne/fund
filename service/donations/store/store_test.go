package store

import (
	"boardfund/pg"
	"boardfund/service/donations"
	memberstore "boardfund/service/members/store"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDonationStore_CreateDonationPlan(t *testing.T) {
	t.Run("should successfully create and fetch a donation plan", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		conn, err := connPool.Acquire(context.Background())
		require.NoError(t, err)

		store := NewDonationStore(conn)

		donationPlan := donations.InsertDonationPlan{
			Name:          "test",
			AmountCents:   100,
			IntervalUnit:  "MONTH",
			IntervalCount: 1,
		}

		newDonationPlan, err := store.CreateDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		donationPlanByIDOut, err := store.GetDonationPlanByID(context.Background(), newDonationPlan.ID)
		require.NoError(t, err)

		assert.Equal(t, newDonationPlan.ID, donationPlanByIDOut.ID)
		assert.Equal(t, donationPlan.Name, donationPlanByIDOut.Name)
		assert.Equal(t, donationPlan.AmountCents, donationPlanByIDOut.AmountCents)
		assert.Equal(t, donationPlan.IntervalUnit, string(donationPlanByIDOut.IntervalUnit))
		assert.Equal(t, donationPlan.IntervalCount, donationPlanByIDOut.IntervalCount)
	})
}

func TestDonationStore_CreateDonation(t *testing.T) {
	t.Run("should successfully create and fetch a donation", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		conn, err := connPool.Acquire(context.Background())
		require.NoError(t, err)

		donationStore := NewDonationStore(conn)
		memberStore := memberstore.NewMemberStore(conn)

		donationPlan := donations.InsertDonationPlan{
			Name:          "test",
			AmountCents:   100,
			IntervalUnit:  "MONTH",
			IntervalCount: 1,
		}

		newDonationPlan, err := donationStore.CreateDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		member := memberstore.InsertMember{
			PaypalEmail: "fake@fake.com",
			BCOName:     "gofreescout",
			IPAddress:   "172.0.0.1",
		}

		newMember, err := memberStore.CreateMember(context.Background(), member)
		require.NoError(t, err)

		donation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newMember.ID,
		}

		newDonation, err := donationStore.CreateDonation(context.Background(), donation)
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

		conn, err := connPool.Acquire(context.Background())
		require.NoError(t, err)

		donationStore := NewDonationStore(conn)
		memberStore := memberstore.NewMemberStore(conn)

		donationPlan := donations.InsertDonationPlan{
			Name:          "test",
			AmountCents:   100,
			IntervalUnit:  "MONTH",
			IntervalCount: 1,
		}

		newDonationPlan, err := donationStore.CreateDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		firstMember := memberstore.InsertMember{
			PaypalEmail: "fake@fake.com",
			BCOName:     "gofreescout",
			IPAddress:   "127.0.0.1",
		}

		secondMember := memberstore.InsertMember{
			PaypalEmail: "dummy@dummy.com",
			BCOName:     "gofreescout",
			IPAddress:   "127.0.0.1",
		}

		newFirstMember, err := memberStore.CreateMember(context.Background(), firstMember)
		require.NoError(t, err)

		newSecondMember, err := memberStore.CreateMember(context.Background(), secondMember)
		require.NoError(t, err)

		firstDonation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		secondDonation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newSecondMember.ID,
		}

		thirdDonation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		newFirstDonation, err := donationStore.CreateDonation(context.Background(), firstDonation)
		require.NoError(t, err)

		_, err = donationStore.CreateDonation(context.Background(), secondDonation)
		require.NoError(t, err)

		newThirdDonation, err := donationStore.CreateDonation(context.Background(), thirdDonation)
		require.NoError(t, err)

		donationsByEmail, err := donationStore.GetDonationsByMemberPaypalEmail(context.Background(), firstMember.PaypalEmail)
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

		conn, err := connPool.Acquire(context.Background())
		require.NoError(t, err)

		donationStore := NewDonationStore(conn)
		memberStore := memberstore.NewMemberStore(conn)

		donationPlan := donations.InsertDonationPlan{
			Name:          "test",
			AmountCents:   100,
			IntervalUnit:  "MONTH",
			IntervalCount: 1,
		}

		newDonationPlan, err := donationStore.CreateDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		member := memberstore.InsertMember{
			PaypalEmail: "fake@fake.com",
			BCOName:     "gofreescout",
			IPAddress:   "127.0.0.1",
		}

		newMember, err := memberStore.CreateMember(context.Background(), member)
		require.NoError(t, err)

		donation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newMember.ID,
		}

		newDonation, err := donationStore.CreateDonation(context.Background(), donation)
		require.NoError(t, err)

		donationPayment := donations.InsertDonationPayment{
			DonationID:      newDonation.ID,
			PaypalPaymentID: "PAY-123456789",
			AmountCents:     100,
		}

		newDonationPayment, err := donationStore.CreateDonationPayment(context.Background(), donationPayment)
		require.NoError(t, err)

		donationPaymentByIDOut, err := donationStore.GetDonationPaymentByID(context.Background(), newDonationPayment.ID)
		require.NoError(t, err)

		assert.Equal(t, newDonationPayment.ID, donationPaymentByIDOut.ID)
		assert.Equal(t, donationPayment.DonationID, donationPaymentByIDOut.DonationID)
		assert.Equal(t, donationPayment.PaypalPaymentID, donationPaymentByIDOut.PaypalPaymentID)
		assert.Equal(t, donationPayment.AmountCents, donationPaymentByIDOut.AmountCents)
	})
}

func TestDonationStore_GetDonationPaymentsByMemberPaypalEmail(t *testing.T) {
	t.Run("should successfully get donation payments by member paypal email", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		conn, err := connPool.Acquire(context.Background())
		require.NoError(t, err)

		donationStore := NewDonationStore(conn)
		memberStore := memberstore.NewMemberStore(conn)

		donationPlan := donations.InsertDonationPlan{
			Name:          "test",
			AmountCents:   100,
			IntervalUnit:  "MONTH",
			IntervalCount: 1,
		}

		newDonationPlan, err := donationStore.CreateDonationPlan(context.Background(), donationPlan)
		require.NoError(t, err)

		firstMember := memberstore.InsertMember{
			PaypalEmail: "fake@fake.com",
			BCOName:     "gofreescout",
			IPAddress:   "127.0.0.1",
		}

		secondMember := memberstore.InsertMember{
			PaypalEmail: "dummy@dummy.com",
			BCOName:     "gofreescout",
			IPAddress:   "127.0.0.1",
		}

		newFirstMember, err := memberStore.CreateMember(context.Background(), firstMember)
		require.NoError(t, err)

		newSecondMember, err := memberStore.CreateMember(context.Background(), secondMember)
		require.NoError(t, err)

		firstDonation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		secondDonation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newSecondMember.ID,
		}

		thirdDonation := donations.InsertDonation{
			DonationPlanID: newDonationPlan.ID,
			DonorID:        newFirstMember.ID,
		}

		newFirstDonation, err := donationStore.CreateDonation(context.Background(), firstDonation)
		require.NoError(t, err)

		newSecondDonation, err := donationStore.CreateDonation(context.Background(), secondDonation)
		require.NoError(t, err)

		newThirdDonation, err := donationStore.CreateDonation(context.Background(), thirdDonation)
		require.NoError(t, err)

		firstDonationPayment := donations.InsertDonationPayment{
			DonationID:      newFirstDonation.ID,
			PaypalPaymentID: "PAY-123456789",
			AmountCents:     100,
		}

		secondDonationPayment := donations.InsertDonationPayment{
			DonationID:      newSecondDonation.ID,
			PaypalPaymentID: "PAY-987654321",
			AmountCents:     100,
		}

		thirdDonationPayment := donations.InsertDonationPayment{
			DonationID:      newThirdDonation.ID,
			PaypalPaymentID: "PAY-123456789",
			AmountCents:     100,
		}

		fourthDonationPayment := donations.InsertDonationPayment{
			DonationID:      newFirstDonation.ID,
			PaypalPaymentID: "PAY-987654321",
			AmountCents:     100,
		}

		firstNewDonationPayment, err := donationStore.CreateDonationPayment(context.Background(), firstDonationPayment)
		require.NoError(t, err)

		_, err = donationStore.CreateDonationPayment(context.Background(), secondDonationPayment)
		require.NoError(t, err)

		thirdNewDonationPayment, err := donationStore.CreateDonationPayment(context.Background(), thirdDonationPayment)
		require.NoError(t, err)

		fourthNewDonationPayment, err := donationStore.CreateDonationPayment(context.Background(), fourthDonationPayment)
		require.NoError(t, err)

		donationPaymentsByEmail, err := donationStore.GetDonationPaymentsByMemberPaypalEmail(context.Background(), firstMember.PaypalEmail)

		expectedPayments := []donations.DonationPayment{*firstNewDonationPayment, *fourthNewDonationPayment, *thirdNewDonationPayment}

		assert.ElementsMatch(t, expectedPayments, donationPaymentsByEmail)
	})
}
