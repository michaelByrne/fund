package fundevents_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedFund(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	fundID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
		 VALUES ($1, 'Test Fund', 'd', $2, 'paypal', 'monthly', now())`,
		fundID, fundID.String(),
	)
	require.NoError(t, err)

	return fundID
}

func seedMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()

	memberID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO member (id, email, bco_name, active) VALUES ($1, $2, $3, true)`,
		memberID, memberID.String()+"@test.org", name+"-"+memberID.String()[:8],
	)
	require.NoError(t, err)

	return memberID
}

func TestFundEvents(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := fundevents.NewService(fundeventstore.NewEventStore(pool), logger)

	t.Run("newest first, scoped to one fund", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)
		otherFundID := seedFund(t, ctx, pool)
		donor := seedMember(t, ctx, pool, "donor")

		base := time.Now().Add(-24 * time.Hour)
		amount := int32(2500)

		for i, kind := range []fundevents.Kind{
			fundevents.KindDonationStarted,
			fundevents.KindPaymentReceived,
			fundevents.KindDonationCancelled,
		} {
			svc.Record(ctx, fundevents.Record{
				FundID:          fundID,
				Kind:            kind,
				OccurredAt:      base.Add(time.Duration(i) * time.Hour),
				SubjectMemberID: &donor,
				AmountCents:     &amount,
			})
		}

		svc.Record(ctx, fundevents.Record{
			FundID: otherFundID,
			Kind:   fundevents.KindDonationStarted,
		})

		events, err := svc.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
		require.NoError(t, err)
		require.Len(t, events, 3, "the other fund's event must not appear")

		assert.Equal(t, fundevents.KindDonationCancelled, events[0].Kind)
		assert.Equal(t, fundevents.KindPaymentReceived, events[1].Kind)
		assert.Equal(t, fundevents.KindDonationStarted, events[2].Kind)
	})

	// The case that motivated recording at all: a subscription ends at the
	// provider, no person is involved, and the domain row's `updated` column
	// would be the only trace -- overwritten by the next change to that row.
	t.Run("a provider cancellation has no actor and keeps the provider's timestamp", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)
		donor := seedMember(t, ctx, pool, "donor")
		donationID := uuid.New()

		endedAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)

		svc.Record(ctx, fundevents.Record{
			FundID:          fundID,
			Kind:            fundevents.KindDonationCancelled,
			OccurredAt:      endedAt,
			SubjectMemberID: &donor,
			Detail:          "subscription cancelled at provider",
			ReferenceID:     &donationID,
		})

		events, err := svc.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
		require.NoError(t, err)
		require.Len(t, events, 1)

		event := events[0]

		assert.True(t, event.ByProvider(), "no actor means the provider or a job did it")
		assert.Empty(t, event.ActorName)
		assert.Equal(t, donor, *event.SubjectMemberID)
		assert.Equal(t, donationID, *event.ReferenceID)
		assert.Equal(t, "subscription cancelled at provider", event.Detail)

		// Not "when we heard about it": the feed must read in the order things
		// actually happened.
		assert.WithinDuration(t, endedAt, event.OccurredAt, time.Second)
	})

	t.Run("an approval records who did it", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)
		treasurer := seedMember(t, ctx, pool, "treasurer")
		batchID := uuid.New()
		amount := int32(7500)

		svc.Record(ctx, fundevents.Record{
			FundID:        fundID,
			Kind:          fundevents.KindBatchApproved,
			ActorMemberID: &treasurer,
			AmountCents:   &amount,
			ReferenceID:   &batchID,
		})

		events, err := svc.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
		require.NoError(t, err)
		require.Len(t, events, 1)

		assert.False(t, events[0].ByProvider())
		assert.Equal(t, treasurer, *events[0].ActorMemberID)
		assert.NotEmpty(t, events[0].ActorName, "the actor's name is resolved by the query, not a second lookup")
		assert.Equal(t, int32(7500), *events[0].AmountCents)
	})

	// member.bco_name is nullable, so an actor can be recorded with no name to
	// resolve. That must still read as a person, not as an automated action.
	t.Run("an actor with no name is still not automatic", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)

		nameless := uuid.New()
		_, err := pool.Exec(ctx,
			`INSERT INTO member (id, email, bco_name, active) VALUES ($1, $2, NULL, true)`,
			nameless, nameless.String()+"@test.org",
		)
		require.NoError(t, err)

		svc.Record(ctx, fundevents.Record{
			FundID:        fundID,
			Kind:          fundevents.KindBatchApproved,
			ActorMemberID: &nameless,
		})

		events, err := svc.GetFundEvents(ctx, fundID, fundevents.DefaultLimit)
		require.NoError(t, err)
		require.Len(t, events, 1)

		assert.False(t, events[0].ByProvider(), "an actor is recorded, so a person did this")
		assert.Empty(t, events[0].ActorName)
	})

	t.Run("limit is honoured", func(t *testing.T) {
		fundID := seedFund(t, ctx, pool)

		for range 5 {
			svc.Record(ctx, fundevents.Record{FundID: fundID, Kind: fundevents.KindPaymentReceived})
		}

		events, err := svc.GetFundEvents(ctx, fundID, 2)
		require.NoError(t, err)
		assert.Len(t, events, 2)
	})

	// Recording must never be able to take down the operation it describes.
	t.Run("an unusable record is dropped, not panicked on", func(t *testing.T) {
		require.NotPanics(t, func() {
			svc.Record(ctx, fundevents.Record{Kind: fundevents.KindDonationStarted})
		})
	})
}
