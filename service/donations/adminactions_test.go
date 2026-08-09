package donations_test

import (
	"bytes"
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

// The administrative half of the sweep: operations that changed something and
// left no trace of who. Each writes an event now, which -- because
// fundevents.Record logs what it records -- is also what puts them in the log.
func TestAdministrativeActionsAreRecorded(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	provider := mocks.PaymentsProviderMock{}
	provider.CreateFundFunc = func(context.Context, string, string) (string, error) {
		return uuid.NewString(), nil
	}

	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(
		donationsstore.NewDonationStore(pool), stubDocumentStorage{}, newFakeBucket(),
		&provider, events, []string{"payments"}, logger,
	)

	// A note can only be left by somebody with money surviving in the fund, so a
	// donor for these has to have actually given.
	giveTo := func(t *testing.T, fundID uuid.UUID) uuid.UUID {
		t.Helper()

		donorID := seedTestMember(t, ctx, pool)
		donationID := uuid.New()

		_, errDonation := pool.Exec(ctx,
			`INSERT INTO donation (id, recurring, donor_id, provider_order_id, fund_id)
			 VALUES ($1, false, $2, $3, $4)`,
			donationID, donorID, uuid.NewString(), fundID)
		require.NoError(t, errDonation)

		_, errPayment := pool.Exec(ctx,
			`INSERT INTO donation_payment (id, donation_id, paypal_payment_id, amount_cents, refunded_cents)
			 VALUES ($1, $2, $3, 2500, 0)`,
			uuid.New(), donationID, uuid.NewString())
		require.NoError(t, errPayment)

		return donorID
	}

	kinds := func(t *testing.T, fundID uuid.UUID) []fundevents.Event {
		t.Helper()

		all, errEvents := events.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
		require.NoError(t, errEvents)

		return all
	}

	// The first entry in a fund's history, which used to begin at its first
	// donation -- so a fund opened weeks before anyone gave to it read as though
	// it had come into existence with that donation.
	t.Run("opening a fund is the first thing in its history", func(t *testing.T) {
		actor := seedTestMember(t, ctx, pool)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: "created-recorded", Description: "d", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		}, &actor)
		require.NoError(t, errCreate)

		all := kinds(t, fund.ID)
		require.Len(t, all, 1)

		assert.Equal(t, fundevents.KindFundCreated, all[0].Kind)
		require.NotNil(t, all[0].ActorMemberID)
		assert.Equal(t, actor, *all[0].ActorMemberID)
	})

	t.Run("setting and removing the picture both say which", func(t *testing.T) {
		actor := seedTestMember(t, ctx, pool)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: "picture-recorded", Description: "d", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		}, &actor)
		require.NoError(t, errCreate)

		_, errSave := svc.SaveFundImage(ctx, fund.ID, bytes.NewReader(jpegOf(t, 120, 90)), &actor)
		require.NoError(t, errSave)

		require.NoError(t, svc.RemoveFundImage(ctx, fund.ID, &actor))

		all := kinds(t, fund.ID)
		require.Len(t, all, 3, "created, picture set, picture removed")

		// Newest first. Both are fund_updated, because a picture is a fund detail
		// like the goal -- the detail is what tells them apart, and it has to.
		assert.Equal(t, fundevents.KindFundUpdated, all[0].Kind)
		assert.Equal(t, "picture removed", all[0].Detail)
		assert.Equal(t, fundevents.KindFundUpdated, all[1].Kind)
		assert.Equal(t, "picture set", all[1].Detail)
	})

	// An admin taking down a member's words. fund_note.removed_by says who, but
	// only until the next removal of that note overwrites it.
	t.Run("removing a note names the admin and the member", func(t *testing.T) {
		actor := seedTestMember(t, ctx, pool)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: "note-recorded", Description: "d", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		}, &actor)
		require.NoError(t, errCreate)

		donor := giveTo(t, fund.ID)

		note, errNote := svc.SaveFundNote(ctx, fund.ID, donor, "this paid my rent", false)
		require.NoError(t, errNote)

		require.NoError(t, svc.RemoveFundNote(ctx, note.ID, actor))

		all := kinds(t, fund.ID)
		require.NotEmpty(t, all)

		removal := all[0]

		assert.Equal(t, fundevents.KindFundNoteRemoved, removal.Kind)
		require.NotNil(t, removal.ActorMemberID)
		assert.Equal(t, actor, *removal.ActorMemberID, "who took it down")
		require.NotNil(t, removal.SubjectMemberID)
		assert.Equal(t, donor, *removal.SubjectMemberID, "whose words they were")

		// Not the text. The note row still holds it -- that is the point of the
		// removal being soft -- and copying it into the feed would put the words
		// back on a page after somebody decided they should come down.
		assert.NotContains(t, removal.Detail, "rent")
	})

	// A second click changes no row. Recording it would put a removal in the feed
	// for something already gone -- and reaching the recorder at all with nothing
	// to describe logs an error about a fund event with no fund, which is a
	// complaint about a request that was perfectly ordinary.
	t.Run("removing a note twice records once and complains about neither", func(t *testing.T) {
		var out bytes.Buffer

		quiet := slog.New(slog.NewJSONHandler(&out, nil))
		noisy := donations.NewDonationService(
			donationsstore.NewDonationStore(pool), stubDocumentStorage{}, newFakeBucket(),
			&provider, fundevents.NewService(fundeventstore.NewEventStore(pool), quiet),
			[]string{"payments"}, quiet,
		)

		actor := seedTestMember(t, ctx, pool)

		fund, errCreate := noisy.CreateFund(ctx, donations.Fund{
			Name: "note-twice", Description: "d", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		}, &actor)
		require.NoError(t, errCreate)

		donor := giveTo(t, fund.ID)

		note, errNote := noisy.SaveFundNote(ctx, fund.ID, donor, "thank you", false)
		require.NoError(t, errNote)

		require.NoError(t, noisy.RemoveFundNote(ctx, note.ID, actor))

		out.Reset()
		require.NoError(t, noisy.RemoveFundNote(ctx, note.ID, actor))

		var removals int
		for _, event := range kinds(t, fund.ID) {
			if event.Kind == fundevents.KindFundNoteRemoved {
				removals++
			}
		}

		assert.Equal(t, 1, removals)
		assert.NotContains(t, out.String(), `"level":"ERROR"`,
			"a note already down is an ordinary second click, not a fault")
	})

	// A donor taking their own note down is the same event with the same person
	// on both sides, and the feed already knows how to say that without printing
	// the name twice.
	t.Run("a donor removing their own note is recorded too", func(t *testing.T) {
		actor := seedTestMember(t, ctx, pool)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: "own-note", Description: "d", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		}, &actor)
		require.NoError(t, errCreate)

		donor := giveTo(t, fund.ID)

		_, errNote := svc.SaveFundNote(ctx, fund.ID, donor, "on second thoughts", false)
		require.NoError(t, errNote)

		require.NoError(t, svc.RemoveOwnFundNote(ctx, fund.ID, donor))

		removal := kinds(t, fund.ID)[0]

		assert.Equal(t, fundevents.KindFundNoteRemoved, removal.Kind)
		assert.True(t, removal.ActorIsSubject(), "the donor is both")
	})

	// The note removal is about one identifiable member, so it must not reach the
	// page donors read. Kind.Public is an allowlist, which is what makes that the
	// default rather than something to remember.
	t.Run("note removals stay off the public timeline", func(t *testing.T) {
		actor := seedTestMember(t, ctx, pool)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: "note-private", Description: "d", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
		}, &actor)
		require.NoError(t, errCreate)

		donor := giveTo(t, fund.ID)

		note, errNote := svc.SaveFundNote(ctx, fund.ID, donor, "please help", false)
		require.NoError(t, errNote)
		require.NoError(t, svc.RemoveFundNote(ctx, note.ID, actor))

		public, errPublic := events.GetPublicFundEvents(ctx, fund.ID, fundevents.DefaultLimit)
		require.NoError(t, errPublic)

		for _, event := range public {
			assert.NotEqual(t, fundevents.KindFundNoteRemoved, event.Kind,
				"a member's note being taken down is not public business")
		}

		// The fund opening is, though, so this is not just an empty feed.
		require.Len(t, public, 1)
		assert.Equal(t, fundevents.KindFundCreated, public[0].Kind)
	})
}
