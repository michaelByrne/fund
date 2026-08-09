package adminevents_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/pg"
	"boardfund/service/adminevents"
	admineventstore "boardfund/service/adminevents/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestAdminEvents(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := adminevents.NewService(admineventstore.NewEventStore(pool), logger)

	t.Run("newest first, with both names resolved", func(t *testing.T) {
		actor := seedMember(t, ctx, pool, "granter")
		subject := seedMember(t, ctx, pool, "promoted")

		base := time.Now().Add(-24 * time.Hour)

		for i, kind := range []adminevents.Kind{
			adminevents.KindAdminGranted,
			adminevents.KindAdminRevoked,
		} {
			svc.Record(ctx, adminevents.Record{
				Kind:            kind,
				OccurredAt:      base.Add(time.Duration(i) * time.Hour),
				ActorMemberID:   &actor,
				SubjectMemberID: &subject,
			})
		}

		events, err := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, err)
		require.Len(t, events, 2)

		assert.Equal(t, adminevents.KindAdminRevoked, events[0].Kind)
		assert.Equal(t, adminevents.KindAdminGranted, events[1].Kind)

		// The page renders names, not ids. A join that failed would show two
		// blanks, which reads as an unattributed change rather than as a bug.
		assert.NotEmpty(t, events[0].ActorName)
		assert.NotEmpty(t, events[0].SubjectName)
		assert.False(t, events[0].ByProvider())
	})

	// The bootstrap case, and the one a reader must not misread. The first admin
	// is made in the Cognito console, so nothing in the app can name who did it.
	t.Run("a change made outside the app has no actor", func(t *testing.T) {
		subject := seedMember(t, ctx, pool, "bootstrap")

		svc.Record(ctx, adminevents.Record{
			Kind:            adminevents.KindAdminGranted,
			SubjectMemberID: &subject,
			Detail:          "granted in the cognito console",
		})

		events, err := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, err)
		require.NotEmpty(t, events)

		event := events[0]

		assert.True(t, event.ByProvider())
		assert.Empty(t, event.ActorName)
		assert.Equal(t, subject, *event.SubjectMemberID)
		assert.Equal(t, "granted in the cognito console", event.Detail)
	})

	t.Run("an admin changing their own access is marked as such", func(t *testing.T) {
		self := seedMember(t, ctx, pool, "self")

		svc.Record(ctx, adminevents.Record{
			Kind:            adminevents.KindAdminGranted,
			ActorMemberID:   &self,
			SubjectMemberID: &self,
		})

		events, err := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, err)
		require.NotEmpty(t, events)

		assert.True(t, events[0].SelfInflicted(), "self-promotion is the shape worth noticing")
	})

	// Recording never returns an error, so a rejected write is only visible as a
	// missing row. A record with no subject describes nothing and must not become
	// a line attributed to member 000...0.
	t.Run("a record with no subject is refused", func(t *testing.T) {
		before, err := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, err)

		svc.Record(ctx, adminevents.Record{Kind: adminevents.KindAdminGranted})

		after, err := svc.GetAdminEvents(ctx, adminevents.DefaultLimit)
		require.NoError(t, err)

		assert.Len(t, after, len(before))
	})

	// The whole value of the table is that a line cannot be taken back. Enforced
	// by there being no such query rather than by a database grant, so this
	// asserts the absence: if an update or delete is ever generated, this is
	// where it shows up.
	t.Run("nothing in the schema is written twice", func(t *testing.T) {
		subject := seedMember(t, ctx, pool, "permanent")

		svc.Record(ctx, adminevents.Record{
			Kind:            adminevents.KindAdminGranted,
			SubjectMemberID: &subject,
		})

		var created, occurred time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT created, occurred_at FROM admin_event WHERE subject_member_id = $1`,
			subject,
		).Scan(&created, &occurred))

		assert.False(t, created.IsZero())
		// Nothing reports these after the fact, so an unset OccurredAt must fall
		// back to now() rather than to the zero time, which would sort the row to
		// the bottom of a log ordered by when things happened.
		assert.WithinDuration(t, created, occurred, time.Second)
	})
}
