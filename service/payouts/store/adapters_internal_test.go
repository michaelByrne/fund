package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The query aggregates names and ids with the same ordering, so they arrive
// paired. This is what happens if that ever stops being true.
//
// Reachable only from here: the query cannot return mismatched lengths, so
// nothing going through the store can exercise it. It is worth keeping anyway --
// a name paired with the wrong id links a treasurer to somebody else's member
// page, and the page would look perfectly convincing.
func TestPayeesAreNotPairedWhenTheArraysDisagree(t *testing.T) {
	ada, bo := uuid.New(), uuid.New()

	t.Run("pairs by position", func(t *testing.T) {
		payees := payeesFrom([]string{"ada", "bo"}, []uuid.UUID{ada, bo})

		require.Len(t, payees, 2)
		require.Equal(t, "ada", payees[0].Name)
		require.Equal(t, ada, payees[0].ID)
		require.Equal(t, "bo", payees[1].Name)
		require.Equal(t, bo, payees[1].ID)
	})

	t.Run("refuses to guess when there are more names than ids", func(t *testing.T) {
		require.Nil(t, payeesFrom([]string{"ada", "bo"}, []uuid.UUID{ada}),
			"no names at all beats names attached to the wrong people")
	})

	t.Run("refuses to guess when there are more ids than names", func(t *testing.T) {
		require.Nil(t, payeesFrom([]string{"ada"}, []uuid.UUID{ada, bo}))
	})

	t.Run("empty is empty", func(t *testing.T) {
		require.Empty(t, payeesFrom(nil, nil))
	})

	// The nil uuid is how the query says there is no member row behind a name.
	t.Run("keeps a payee with no member", func(t *testing.T) {
		payees := payeesFrom([]string{"someone who left"}, []uuid.UUID{uuid.Nil})

		require.Len(t, payees, 1)
		require.False(t, payees[0].HasPage())
		require.Equal(t, "someone who left", payees[0].Name)
	})
}
