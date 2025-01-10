package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/members"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

func (s MemberStore) GetActiveMembers(ctx context.Context) ([]members.Member, error) {
	query := s.queries.GetActiveMembers

	return pg.FetchAll(ctx, query, fromDBMember)
}

func (s MemberStore) SetMemberToInactive(ctx context.Context, id uuid.UUID) (*members.Member, error) {
	query := s.queries.SetMemberToInactive

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	return pg.UpdateOne(ctx, id, query, argIdentity, fromDBMember)
}

func (s MemberStore) SetMemberToActive(ctx context.Context, id uuid.UUID) (*members.Member, error) {
	query := s.queries.SetMemberToActive

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	return pg.UpdateOne(ctx, id, query, argIdentity, fromDBMember)
}

func (s MemberStore) GetMembers(ctx context.Context) ([]members.Member, error) {
	query := s.queries.GetMembers

	return pg.FetchAll(ctx, query, fromDBMember)
}

func (s MemberStore) GetMemberByID(ctx context.Context, id uuid.UUID) (*members.Member, error) {
	query := s.queries.GetMemberById

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	return pg.FetchOne(ctx, id, query, argIdentity, fromDBMember)
}

func (s MemberStore) UpsertMember(ctx context.Context, member members.UpsertMember) (*members.Member, error) {
	query := s.queries.UpsertMember

	return pg.UpsertOne(ctx, member, query, toDBMemberUpsertParams, fromDBMember)
}

func (s MemberStore) GetMemberWithDonations(ctx context.Context, id uuid.UUID) (*members.Member, error) {
	query := s.queries.GetMemberWithDonations

	member, err := query(ctx, id)
	if err != nil {
		return nil, err
	}

	return fromDBMemberWithDonations(member)
}

func (s MemberStore) SearchMembersByUsername(ctx context.Context, arg string) ([]members.MemberSearchResult, error) {
	query := s.queries.SearchMembersByUsername

	argToPGText := func(username string) pgtype.Text { return pgtype.Text{String: username, Valid: true} }

	return pg.FetchMany(ctx, arg, query, argToPGText, fromDBMemberSearchResult)
}

func (s MemberStore) GetMemberByUsername(ctx context.Context, username string) (*members.Member, error) {
	query := s.queries.GetMemberByUsername

	argToPGText := func(username string) pgtype.Text { return pgtype.Text{String: username, Valid: true} }

	return pg.FetchOne(ctx, username, query, argToPGText, fromDBMember)
}
