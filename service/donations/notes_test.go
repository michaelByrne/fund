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
