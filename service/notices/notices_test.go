package notices_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"boardfund/pg"
	"boardfund/service/notices"
	noticestore "boardfund/service/notices/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	memberID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO member (id, email, bco_name, active) VALUES ($1, $2, $3, true)`,
		memberID, memberID.String()+"@test.org", "admin-"+memberID.String()[:8],
	)
	require.NoError(t, err)

	return memberID
}

func TestNotices(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := notices.NewService(noticestore.NewNoticeStore(pool), logger)

	t.Run("a new notice is active and attributed", func(t *testing.T) {
		admin := seedMember(t, ctx, pool)

		notice, errCreate := svc.Create(ctx, "payouts are delayed this week", &admin)
		require.NoError(t, errCreate)

		assert.True(t, notice.Active, "a notice is posted to be read")
		require.NotNil(t, notice.CreatedBy)
		assert.Equal(t, admin, *notice.CreatedBy)

		// Both set on insert, so "who last touched this" is answerable before
		// anybody has touched it again.
		require.NotNil(t, notice.UpdatedBy)
		assert.Equal(t, admin, *notice.UpdatedBy)
	})

	t.Run("whitespace is trimmed and an empty notice is refused", func(t *testing.T) {
		admin := seedMember(t, ctx, pool)

		notice, errCreate := svc.Create(ctx, "   spaced out   ", &admin)
		require.NoError(t, errCreate)
		assert.Equal(t, "spaced out", notice.Body)

		for _, empty := range []string{"", "   ", "\n\t "} {
			_, errEmpty := svc.Create(ctx, empty, &admin)
			assert.ErrorIs(t, errEmpty, notices.ErrEmptyBody, "%q is nothing to say", empty)
		}
	})

	// Counted in runes, matching char_length on the column. Counting bytes would
	// refuse a message the database would have accepted, which is the sort of
	// difference that only shows up once somebody uses an accent.
	t.Run("the length limit is the column's, counted the same way", func(t *testing.T) {
		admin := seedMember(t, ctx, pool)

		_, errLong := svc.Create(ctx, strings.Repeat("a", notices.MaxBodyLength+1), &admin)
		assert.ErrorIs(t, errLong, notices.ErrBodyTooLong)

		// Multi-byte, and exactly at the limit: this is refused only by a check
		// that counts bytes, and the column would have taken it.
		atLimit := strings.Repeat("é", notices.MaxBodyLength)

		_, errAccented := svc.Create(ctx, atLimit, &admin)
		assert.NoError(t, errAccented, "the limit is characters, not bytes")
	})

	// The home page asks for this. A notice taken down must not be in it, which
	// is the entire point of the column.
	t.Run("taking one down removes it from what the home page reads", func(t *testing.T) {
		admin := seedMember(t, ctx, pool)

		notice, errCreate := svc.Create(ctx, "a notice that will come down", &admin)
		require.NoError(t, errCreate)

		active, errActive := svc.Active(ctx)
		require.NoError(t, errActive)
		require.True(t, holds(active, notice.ID))

		taken, errDown := svc.SetActive(ctx, notice.ID, false, &admin)
		require.NoError(t, errDown)
		assert.False(t, taken.Active)

		active, errActive = svc.Active(ctx)
		require.NoError(t, errActive)
		assert.False(t, holds(active, notice.ID),
			"a notice that is down should not reach the home page")

		// Still in the admin panel, so it can be put back up without retyping.
		all, errAll := svc.All(ctx)
		require.NoError(t, errAll)
		assert.True(t, holds(all, notice.ID))

		back, errUp := svc.SetActive(ctx, notice.ID, true, &admin)
		require.NoError(t, errUp)
		assert.True(t, back.Active)

		active, errActive = svc.Active(ctx)
		require.NoError(t, errActive)
		assert.True(t, holds(active, notice.ID))
	})

	// Setting the state wanted rather than flipping what is stored: two admins
	// clicking at once both get what they asked for instead of each undoing the
	// other.
	t.Run("setting the same state twice is not a toggle", func(t *testing.T) {
		admin := seedMember(t, ctx, pool)

		notice, errCreate := svc.Create(ctx, "posted once", &admin)
		require.NoError(t, errCreate)

		_, errFirst := svc.SetActive(ctx, notice.ID, false, &admin)
		require.NoError(t, errFirst)

		second, errSecond := svc.SetActive(ctx, notice.ID, false, &admin)
		require.NoError(t, errSecond)

		assert.False(t, second.Active, "asking for the state it is already in leaves it there")
	})

	t.Run("a change records who made it", func(t *testing.T) {
		author, editor := seedMember(t, ctx, pool), seedMember(t, ctx, pool)

		notice, errCreate := svc.Create(ctx, "posted by one, taken down by another", &author)
		require.NoError(t, errCreate)

		changed, errDown := svc.SetActive(ctx, notice.ID, false, &editor)
		require.NoError(t, errDown)

		require.NotNil(t, changed.CreatedBy)
		assert.Equal(t, author, *changed.CreatedBy, "who posted it does not change")
		require.NotNil(t, changed.UpdatedBy)
		assert.Equal(t, editor, *changed.UpdatedBy, "who took it down")
	})

	// Newest first: a notice put up today is the one nobody has read yet.
	t.Run("the newest notice is first", func(t *testing.T) {
		admin := seedMember(t, ctx, pool)

		_, errOld := svc.Create(ctx, "the older one", &admin)
		require.NoError(t, errOld)

		newest, errNew := svc.Create(ctx, "the newer one", &admin)
		require.NoError(t, errNew)

		active, errActive := svc.Active(ctx)
		require.NoError(t, errActive)
		require.NotEmpty(t, active)

		assert.Equal(t, newest.ID, active[0].ID)
	})
}

func holds(found []notices.Notice, id uuid.UUID) bool {
	for _, notice := range found {
		if notice.ID == id {
			return true
		}
	}

	return false
}
