package donations_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
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

// A note is the first thing this application publishes that a member wrote, and
// the rule about who may write one is the whole of its access control.
func TestFundNotes(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	svc := donations.NewDonationService(store, stubDocumentStorage{}, &mocks.PaymentsProviderMock{},
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger)

	// giveTo records a payment, optionally refunded, and returns the donor.
	giveTo := func(t *testing.T, fundID uuid.UUID, cents, refunded int32) uuid.UUID {
		t.Helper()

		donorID := seedMemberRow(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
			 VALUES ($1, false, $2, $3, $4)`,
			donationID, donorID, uuid.NewString(), fundID)
		require.NoError(t, errDonation)

		if cents > 0 {
			_, errPayment := pool.Exec(ctx,
				`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, refunded_cents)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New(), donationID, uuid.NewString(), cents, refunded)
			require.NoError(t, errPayment)
		}

		return donorID
	}

	t.Run("a donor can leave a note", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)

		note, errSave := svc.SaveFundNote(ctx, fundID, donorID, "  this fund got me through a hard month  ", false)
		require.NoError(t, errSave)
		require.NotNil(t, note)

		// Trimmed, because a note that is mostly whitespace renders as a gap.
		assert.Equal(t, "this fund got me through a hard month", note.Body)

		notes, errList := svc.ListFundNotes(ctx, fundID)
		require.NoError(t, errList)
		require.Len(t, notes, 1)
		assert.NotEmpty(t, notes[0].AuthorName)
	})

	t.Run("somebody who has not given cannot", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		stranger := seedMemberRow(t, ctx, pool)

		_, errSave := svc.SaveFundNote(ctx, fundID, stranger, "hello", false)
		require.ErrorIs(t, errSave, donations.ErrNotADonor)
	})

	t.Run("a donation with no payment yet is not money given", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		// A subscription created today that PayPal has not charged. Active, and
		// not a donation of money.
		donorID := giveTo(t, fundID, 0, 0)

		_, errSave := svc.SaveFundNote(ctx, fundID, donorID, "hello", false)
		require.ErrorIs(t, errSave, donations.ErrNotADonor)
	})

	t.Run("money refunded in full was not given", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 5000)

		_, errSave := svc.SaveFundNote(ctx, fundID, donorID, "hello", false)
		require.ErrorIs(t, errSave, donations.ErrNotADonor)
	})

	t.Run("a partial refund still leaves a donor", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 2000)

		_, errSave := svc.SaveFundNote(ctx, fundID, donorID, "still counts", false)
		require.NoError(t, errSave)
	})

	t.Run("a second note edits the first", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)

		_, errFirst := svc.SaveFundNote(ctx, fundID, donorID, "first thoughts", false)
		require.NoError(t, errFirst)

		_, errSecond := svc.SaveFundNote(ctx, fundID, donorID, "second thoughts", false)
		require.NoError(t, errSecond)

		notes, errList := svc.ListFundNotes(ctx, fundID)
		require.NoError(t, errList)

		// One note per donor per fund. Otherwise it is a comment section, and a
		// comment section needs moderation this does not have.
		require.Len(t, notes, 1)
		assert.Equal(t, "second thoughts", notes[0].Body)
	})

	t.Run("an anonymous note carries no name", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)

		_, errSave := svc.SaveFundNote(ctx, fundID, donorID, "rather not say", true)
		require.NoError(t, errSave)

		notes, errList := svc.ListFundNotes(ctx, fundID)
		require.NoError(t, errList)
		require.Len(t, notes, 1)

		// Withheld at the store, not the template: a name left on the struct is one
		// careless render from being shown.
		assert.Empty(t, notes[0].AuthorName)
		assert.True(t, notes[0].Anonymous)
	})

	t.Run("an empty note is refused", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)

		_, errSave := svc.SaveFundNote(ctx, fundID, donorID, "   \n  ", false)
		require.ErrorIs(t, errSave, donations.ErrNoteEmpty)
	})

	t.Run("an over-long note is refused", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)

		_, errSave := svc.SaveFundNote(ctx, fundID, donorID,
			strings.Repeat("a", donations.MaxNoteLength+1), false)
		require.ErrorIs(t, errSave, donations.ErrNoteTooLong)
	})

	t.Run("length is counted in characters, not bytes", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)

		// Three bytes each. Counting bytes would cut a donor off at a third of the
		// length for writing in their own language.
		_, errSave := svc.SaveFundNote(ctx, fundID, donorID,
			strings.Repeat("あ", donations.MaxNoteLength), false)
		require.NoError(t, errSave)
	})

	t.Run("a removed note disappears, including from its author", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)
		admin := seedMemberRow(t, ctx, pool)

		note, errSave := svc.SaveFundNote(ctx, fundID, donorID, "take this down", false)
		require.NoError(t, errSave)

		require.NoError(t, svc.RemoveFundNote(ctx, note.ID, admin))

		notes, errList := svc.ListFundNotes(ctx, fundID)
		require.NoError(t, errList)
		assert.Empty(t, notes, "a removed note should not be published")

		own, errOwn := svc.GetFundNoteForMember(ctx, fundID, donorID)
		require.NoError(t, errOwn)
		assert.Nil(t, own, "editing it would be a way to discover it had been removed")

		// The row survives, with who removed it. After taking something down that
		// is what you want to still have.
		var removedBy uuid.UUID
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT removed_by FROM fund_note WHERE id = $1`, note.ID).Scan(&removedBy))
		assert.Equal(t, admin, removedBy)
	})

	t.Run("editing does not resurrect a removed note", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := giveTo(t, fundID, 5000, 0)
		admin := seedMemberRow(t, ctx, pool)

		note, errSave := svc.SaveFundNote(ctx, fundID, donorID, "first", false)
		require.NoError(t, errSave)
		require.NoError(t, svc.RemoveFundNote(ctx, note.ID, admin))

		// The donor writes again. A moderator's decision is not something the
		// moderated party gets to undo.
		_, errAgain := svc.SaveFundNote(ctx, fundID, donorID, "trying again", false)
		require.NoError(t, errAgain)

		notes, errList := svc.ListFundNotes(ctx, fundID)
		require.NoError(t, errList)
		assert.Empty(t, notes, "a removed note stayed removed")
	})
}

// Eligibility used to be a payment that survived refunds, and that refused the
// donor we most want to hear from: somebody who has just set up a monthly gift.
// PayPal has not charged them yet, so there is no payment row until the first
// webhook lands -- and the thank-you screen, which is where the note is now asked
// for, comes before that. What it must still refuse is money that came back, and
// a subscription that ended before it ever charged.
func TestWhoMayLeaveANote(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	svc := donations.NewDonationService(store, stubDocumentStorage{}, &mocks.PaymentsProviderMock{},
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger)

	// seedDonation writes a donation and, when cents is non-zero, a payment.
	seedDonation := func(t *testing.T, fundID uuid.UUID, recurring, active bool, cents, refunded int32) uuid.UUID {
		t.Helper()

		donorID := seedMemberRow(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, active, donor_id, provider_order_id, fund_id, provider_subscription_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			donationID, recurring, active, donorID, uuid.NewString(), fundID, uuid.NewString())
		require.NoError(t, errDonation)

		if cents > 0 {
			_, errPayment := pool.Exec(ctx,
				`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, refunded_cents)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New(), donationID, uuid.NewString(), cents, refunded)
			require.NoError(t, errPayment)
		}

		return donorID
	}

	cases := []struct {
		name      string
		recurring bool
		active    bool
		cents     int32
		refunded  int32
		want      bool
	}{
		// The case the change was made for: subscribed a moment ago, nothing charged.
		{"a subscription with no payment yet", true, true, 0, 0, true},
		{"a subscription that has charged", true, true, 5000, 0, true},
		// It ended before it ever took money, so no money was ever given.
		{"a subscription cancelled before charging", true, false, 0, 0, false},
		// It ran and charged. The money is real whatever happened afterwards.
		{"a subscription that ended after charging", true, false, 5000, 0, true},
		{"a one-off that was paid", false, true, 5000, 0, true},
		// A one-off is only ever money. Without a payment there is nothing.
		{"a one-off with no payment", false, true, 0, 0, false},
		{"money refunded in full", false, true, 5000, 5000, false},
		{"money partly refunded", false, true, 5000, 4000, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fundID := seedOnceFund(t, ctx, pool)
			donorID := seedDonation(t, fundID, c.recurring, c.active, c.cents, c.refunded)

			given, errGiven := svc.MemberHasGivenToFund(ctx, fundID, donorID)
			require.NoError(t, errGiven)
			require.Equal(t, c.want, given)

			// And the rule is the same one that guards writing, not just the one the
			// page asks to decide whether to show a box.
			_, errNote := svc.SaveFundNote(ctx, fundID, donorID, "thanks", false)
			if c.want {
				require.NoError(t, errNote)
			} else {
				require.ErrorIs(t, errNote, donations.ErrNotADonor)
			}
		})
	}
}

// A donor taking their own note down, which before this had to be asked of an
// admin.
func TestADonorCanRemoveTheirOwnNote(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	svc := donations.NewDonationService(store, stubDocumentStorage{}, &mocks.PaymentsProviderMock{},
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger)

	seedDonor := func(t *testing.T, fundID uuid.UUID) uuid.UUID {
		t.Helper()

		donorID := seedMemberRow(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
			 VALUES ($1, false, $2, $3, $4)`,
			donationID, donorID, uuid.NewString(), fundID)
		require.NoError(t, errDonation)

		_, errPayment := pool.Exec(ctx,
			`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, refunded_cents)
			 VALUES ($1, $2, $3, 5000, 0)`,
			uuid.New(), donationID, uuid.NewString())
		require.NoError(t, errPayment)

		return donorID
	}

	t.Run("takes down their own and nobody else's", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		mine := seedDonor(t, fundID)
		theirs := seedDonor(t, fundID)

		_, errMine := svc.SaveFundNote(ctx, fundID, mine, "mine", false)
		require.NoError(t, errMine)

		_, errTheirs := svc.SaveFundNote(ctx, fundID, theirs, "theirs", false)
		require.NoError(t, errTheirs)

		require.NoError(t, svc.RemoveOwnFundNote(ctx, fundID, mine))

		notes, errList := svc.ListFundNotes(ctx, fundID)
		require.NoError(t, errList)
		require.Len(t, notes, 1)
		require.Equal(t, "theirs", notes[0].Body, "removing one note must not touch another donor's")
	})

	t.Run("only on the fund it was asked for", func(t *testing.T) {
		here := seedOnceFund(t, ctx, pool)
		elsewhere := seedOnceFund(t, ctx, pool)

		donorID := seedMemberRow(t, ctx, pool)
		for _, fundID := range []uuid.UUID{here, elsewhere} {
			donationID := uuid.New()
			_, errDonation := pool.Exec(ctx,
				`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
				 VALUES ($1, false, $2, $3, $4)`,
				donationID, donorID, uuid.NewString(), fundID)
			require.NoError(t, errDonation)

			_, errPayment := pool.Exec(ctx,
				`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, refunded_cents)
				 VALUES ($1, $2, $3, 5000, 0)`,
				uuid.New(), donationID, uuid.NewString())
			require.NoError(t, errPayment)

			_, errNote := svc.SaveFundNote(ctx, fundID, donorID, "a note", false)
			require.NoError(t, errNote)
		}

		require.NoError(t, svc.RemoveOwnFundNote(ctx, here, donorID))

		remaining, errElsewhere := svc.ListFundNotes(ctx, elsewhere)
		require.NoError(t, errElsewhere)
		require.Len(t, remaining, 1, "the same donor's note on another fund must survive")
	})

	t.Run("removing twice is not an error", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := seedDonor(t, fundID)

		_, errNote := svc.SaveFundNote(ctx, fundID, donorID, "mine", false)
		require.NoError(t, errNote)

		require.NoError(t, svc.RemoveOwnFundNote(ctx, fundID, donorID))
		// A double-submitted form asked for it gone, and it is gone.
		require.NoError(t, svc.RemoveOwnFundNote(ctx, fundID, donorID))
	})

	t.Run("removing a note that was never written is not an error", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		require.NoError(t, svc.RemoveOwnFundNote(ctx, fundID, seedMemberRow(t, ctx, pool)))
	})

	t.Run("a removed note does not come back when the fund is listed for its author", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)
		donorID := seedDonor(t, fundID)

		_, errNote := svc.SaveFundNote(ctx, fundID, donorID, "mine", false)
		require.NoError(t, errNote)
		require.NoError(t, svc.RemoveOwnFundNote(ctx, fundID, donorID))

		byFund, errMine := svc.ListFundNotesForMember(ctx, donorID)
		require.NoError(t, errMine)
		require.NotContains(t, byFund, fundID)
	})
}
