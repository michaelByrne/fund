package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/members"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemberStore struct {
	queries *db.Queries
}

func NewMemberStore(conn *pgxpool.Pool) MemberStore {
	return MemberStore{
		queries: db.New(conn),
	}
}

func (s MemberStore) GetMemberByID(ctx context.Context, id uuid.UUID) (*members.Member, error) {
	query := s.queries.GetMemberById

	return pg.FetchOne(ctx, id, query, fromDBMember)
}

func (s MemberStore) GetMemberByPaypalEmail(ctx context.Context, email string) (*members.Member, error) {
	query := s.queries.GetMemberByPaypalEmail

	return pg.FetchOne(ctx, email, query, fromDBMember)
}

func (s MemberStore) UpsertMember(ctx context.Context, member members.UpsertMember) (*members.Member, error) {
	query := s.queries.UpsertMember

	return pg.UpsertOne(ctx, member, query, toDBMemberUpsertParams, fromDBMember)
}
