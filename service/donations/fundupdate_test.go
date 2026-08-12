package donations_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

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

// Editing a fund used to leave no trace at all: the row's `updated` column is
// overwritten by the next change and never said who did it or what the value had
// been. These check that the trace now exists and says something.
func TestEditingAFundIsRecorded(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	provider := mocks.PaymentsProviderMock{}
	provider.CreateFundFunc = func(context.Context, string, string) (string, error) {
		return uuid.NewString(), nil
	}
	provider.CancelSubscriptionsFunc = func(_ context.Context, ids []string) ([]string, error) {
		return ids, nil
	}

	events := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)
	svc := donations.NewDonationService(
		donationsstore.NewDonationStore(pool), stubDocumentStorage{}, newFakeBucket(),
		&provider, events, []string{"payments"}, logger,
	)

	newFund := func(t *testing.T, name string) donations.Fund {
		t.Helper()

		expires := time.Now().Add(30 * 24 * time.Hour)

		fund, errCreate := svc.CreateFund(ctx, donations.Fund{
			Name: name, Description: "help with rent", Active: true,
			PayoutFrequency: donations.PayoutFrequencyMonthly,
			GoalCents:       50000, Expires: &expires,
		}, nil)
		require.NoError(t, errCreate)

		return *fund
	}

	updates := func(t *testing.T, fundID uuid.UUID) []fundevents.Event {
		t.Helper()

		all, errEvents := events.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
		require.NoError(t, errEvents)

		var found []fundevents.Event
		for _, event := range all {
			if event.Kind == fundevents.KindFundUpdated {
				found = append(found, event)
			}
		}

		return found
	}

	t.Run("an edit says what changed and who changed it", func(t *testing.T) {
		fund := newFund(t, "rent-recorded")
		actor := seedTestMember(t, ctx, pool)

		changed := fund
		changed.GoalCents = 75000

		_, err := svc.UpdateFund(ctx, changed, &actor)
		require.NoError(t, err)

		recorded := updates(t, fund.ID)
		require.Len(t, recorded, 1)

		assert.Equal(t, actor, *recorded[0].ActorMemberID, "the point of the record is who did it")
		assert.Contains(t, recorded[0].Detail, "$500.00")
		assert.Contains(t, recorded[0].Detail, "$750.00")
	})

	// The details form posts every field on every save. A no-op save that wrote a
	// line would fill the feed with entries that mean nothing, and a reader who
	// meets a few of those stops trusting the ones that do.
	t.Run("saving without changing anything records nothing", func(t *testing.T) {
		fund := newFund(t, "rent-untouched")
		actor := seedTestMember(t, ctx, pool)

		// Re-read first, so the value being saved is the one that came back from
		// Postgres -- which is how the handler gets it, and where the timestamptz
		// round-trip could otherwise manufacture a change.
		stored, err := svc.GetFundByID(ctx, fund.ID)
		require.NoError(t, err)

		_, err = svc.UpdateFund(ctx, *stored, &actor)
		require.NoError(t, err)

		assert.Empty(t, updates(t, fund.ID))
	})

	// The one that matters most: the end date decides when the fund stops taking
	// money and closes itself.
	t.Run("moving the end date is recorded with both dates", func(t *testing.T) {
		fund := newFund(t, "rent-extended")
		actor := seedTestMember(t, ctx, pool)

		was := fund.Expires.UTC().Format("2006-01-02")
		later := fund.Expires.Add(60 * 24 * time.Hour)

		changed := fund
		changed.Expires = &later

		_, err := svc.UpdateFund(ctx, changed, &actor)
		require.NoError(t, err)

		recorded := updates(t, fund.ID)
		require.Len(t, recorded, 1)

		assert.Contains(t, recorded[0].Detail, was)
		assert.Contains(t, recorded[0].Detail, later.UTC().Format("2006-01-02"))
	})

	// A refused edit changed nothing, so a line saying it did would be the audit
	// trail reporting something that never happened.
	t.Run("a refused edit records nothing", func(t *testing.T) {
		fund := newFund(t, "rent-closed")
		actor := seedTestMember(t, ctx, pool)

		require.NoError(t, svc.DeactivateFund(ctx, fund.ID, &actor))

		changed := fund
		changed.GoalCents = 90000

		_, err := svc.UpdateFund(ctx, changed, &actor)
		require.ErrorIs(t, err, donations.ErrFundClosed)

		assert.Empty(t, updates(t, fund.ID))
	})

	// The timeline a donor reads is built from these, so an edit has to reach it.
	t.Run("an edit reaches the public timeline", func(t *testing.T) {
		fund := newFund(t, "rent-public")
		actor := seedTestMember(t, ctx, pool)

		changed := fund
		changed.Description = "help with rent and utilities"

		_, err := svc.UpdateFund(ctx, changed, &actor)
		require.NoError(t, err)

		public, err := events.GetPublicFundEvents(ctx, fund.ID, fundevents.DefaultLimit)
		require.NoError(t, err)

		// Newest first, and the fund's own creation is the other public event on a
		// fund this young.
		require.Len(t, public, 2)
		assert.Equal(t, fundevents.KindFundCreated, public[1].Kind)

		edit := public[0]

		assert.Equal(t, fundevents.KindFundUpdated, edit.Kind)
		assert.NotEmpty(t, edit.ActorName, "an admin editing a fund is named")

		// The description itself is not repeated -- it can be paragraphs, and the
		// timeline is a list of one-line entries.
		assert.Equal(t, "description edited", edit.Detail)
		assert.False(t, strings.Contains(edit.Detail, "utilities"))
	})
}

// The switch that puts recipients' names on a page donors read. Three things had
// to be right: the column round-trips, an unrelated save does not turn it off,
// and the change is recorded.
func TestShowingRecipientsIsSavedAndRecorded(t *testing.T) {
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

	actor := seedTestMember(t, ctx, pool)

	fund, err := svc.CreateFund(ctx, donations.Fund{
		Name: "recipients-visible", Description: "d", Active: true,
		PayoutFrequency: donations.PayoutFrequencyMonthly,
	}, &actor)
	require.NoError(t, err)

	// Off for a fund nobody has decided about, which is what the column default
	// and the migration both say.
	assert.False(t, fund.EnrolleesVisible, "a new fund does not name its recipients")

	shown := *fund
	shown.EnrolleesVisible = true

	saved, err := svc.UpdateFund(ctx, shown, &actor)
	require.NoError(t, err)
	assert.True(t, saved.EnrolleesVisible)

	// Read back, not just returned: the adapter is where this would be dropped.
	stored, err := svc.GetFundByID(ctx, fund.ID)
	require.NoError(t, err)
	assert.True(t, stored.EnrolleesVisible, "the column should have kept the value")

	// UpdateFund sets the column unconditionally, so a save of something else
	// entirely must not turn it back off.
	other := *stored
	other.Description = "an unrelated edit"

	_, err = svc.UpdateFund(ctx, other, &actor)
	require.NoError(t, err)

	stored, err = svc.GetFundByID(ctx, fund.ID)
	require.NoError(t, err)
	assert.True(t, stored.EnrolleesVisible,
		"editing the description should not stop naming the recipients")

	// And the change itself is in the feed, with its own words rather than a
	// generic "details changed": this one publishes somebody's name.
	all, err := events.GetFundEvents(ctx, fund.ID, fundevents.DefaultLimit)
	require.NoError(t, err)

	var described string
	for _, event := range all {
		if event.Kind == fundevents.KindFundUpdated && strings.Contains(event.Detail, "recipient") {
			described = event.Detail
		}
	}

	assert.Contains(t, described, "recipient names shown to donors")
}
