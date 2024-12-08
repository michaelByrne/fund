package store

import (
	"boardfund/pg"
	"boardfund/service/members"
	"context"
	"fmt"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMemberStore_UpsertMember(t *testing.T) {
	t.Run("should successfully create and fetch a member", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())
		store := NewMemberStore(connPool)

		member := members.UpsertMember{
			ID:        uuid.New(),
			Email:     "member@gmail.com",
			BCOName:   "gofreescout",
			IPAddress: "172.0.0.1",
		}

		newMember, err := store.UpsertMember(context.Background(), member)
		require.NoError(t, err)

		fmt.Println(newMember)

		memberByIDOut, err := store.GetMemberByID(context.Background(), newMember.ID)
		require.NoError(t, err)

		assert.Equal(t, newMember.ID, memberByIDOut.ID)
		assert.Equal(t, member.Email, memberByIDOut.Email)
		assert.Equal(t, member.BCOName, memberByIDOut.BCOName)
		assert.Equal(t, member.IPAddress, memberByIDOut.IPAddress)
		assert.ElementsMatch(t, []members.MemberRole{"DONOR"}, memberByIDOut.Roles)
	})
}
