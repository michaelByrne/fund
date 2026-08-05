package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/auth"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthStore struct {
	queries *db.Queries
}

func NewAuthStore(pool *pgxpool.Pool) AuthStore {
	return AuthStore{
		queries: db.New(pool),
	}
}

func (s AuthStore) GetApprovedEmail(ctx context.Context, arg string) (*auth.ApprovedEmail, error) {
	query := s.queries.GetApprovedEmail

	argIdentity := func(in string) string { return in }

	return pg.FetchOne(ctx, arg, query, argIdentity, fromDBApprovedEmail)
}

func (s AuthStore) MarkEmailAsUsed(ctx context.Context, email string) (*auth.ApprovedEmail, error) {
	query := s.queries.MarkApprovedEmailUsed

	argIdentity := func(in string) string { return in }

	return pg.UpdateOne(ctx, email, query, argIdentity, fromDBApprovedEmail)
}

func (s AuthStore) InsertApprovedEmail(ctx context.Context, email string) (*auth.ApprovedEmail, error) {
	query := s.queries.InsertApprovedEmail

	argIdentity := func(in string) string { return in }

	return pg.CreateOne(ctx, email, query, argIdentity, fromDBApprovedEmail)
}

func (s AuthStore) GetApprovedEmails(ctx context.Context) ([]auth.ApprovedEmail, error) {
	query := s.queries.GetApprovedEmails

	return pg.FetchAll(ctx, query, fromDBApprovedEmail)
}

func (s AuthStore) DeleteApprovedEmail(ctx context.Context, email string) (*auth.ApprovedEmail, error) {
	query := s.queries.DeleteApprovedEmail

	argIdentity := func(in string) string { return in }

	return pg.DeleteOne(ctx, email, query, argIdentity, fromDBApprovedEmail)
}
