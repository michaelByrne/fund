package store

import (
	"boardfund/pg"
	"boardfund/service/members"
	"context"
	_ "github.com/jackc/pgx"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"testing"
)

func TestMemberStore_UpsertMember(t *testing.T) {
	t.Run("should successfully create and fetch a member", func(t *testing.T) {
		container, connPool, err := pg.SetupTestDatabase()
		require.NoError(t, err)

		defer container.Terminate(context.Background())

		conn, err := connPool.Acquire(context.Background())
		require.NoError(t, err)

		store := NewMemberStore(conn)

		member := members.UpsertMember{
			MemberProviderEmail: "member@gmail.com",
			BCOName:             "gofreescout",
			IPAddress:           "172.0.0.1",
		}

		newMember, err := store.UpsertMember(context.Background(), member)
		require.NoError(t, err)

		memberByIDOut, err := store.GetMemberByID(context.Background(), newMember.ID)
		require.NoError(t, err)

		assert.Equal(t, newMember.ID, memberByIDOut.ID)
		assert.Equal(t, member.MemberProviderEmail, memberByIDOut.MemberProviderEmail)
		assert.Equal(t, member.BCOName, memberByIDOut.BCOName)
		assert.Equal(t, member.IPAddress, memberByIDOut.IPAddress)

		memberByEmailOut, err := store.GetMemberByPaypalEmail(context.Background(), member.MemberProviderEmail)
		require.NoError(t, err)

		assert.Equal(t, newMember.ID, memberByEmailOut.ID)
		assert.Equal(t, member.MemberProviderEmail, memberByEmailOut.MemberProviderEmail)
		assert.Equal(t, member.BCOName, memberByEmailOut.BCOName)
		assert.Equal(t, member.IPAddress, memberByEmailOut.IPAddress)
	})
}
