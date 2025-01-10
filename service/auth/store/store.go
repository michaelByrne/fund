package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/auth"
	"context"
	"github.com/google/uuid"
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

func (s AuthStore) InsertPasskeyUser(ctx context.Context, arg auth.InsertPasskeyUser) (*auth.PasskeyUser, error) {
	query := s.queries.InsertPasskeyUser

	return pg.CreateOne(ctx, arg, query, toDBInsertPasskeyUserParams, fromDBPasskeyUser)
}

func (s AuthStore) GetPasskeyUser(ctx context.Context, arg string) (*auth.PasskeyUser, error) {
	query := s.queries.GetPasskeyUser

	argIdentity := func(in string) string { return in }

	return pg.FetchOne(ctx, arg, query, argIdentity, fromDBPasskeyUser)
}

func (s AuthStore) GetPasskeyUserByID(ctx context.Context, arg uuid.UUID) (*auth.PasskeyUser, error) {
	query := s.queries.GetPasskeyUserById

	argTransform := func(in uuid.UUID) []byte { return []byte(in.String()) }

	return pg.FetchOne(ctx, arg, query, argTransform, fromDBPasskeyUser)
}

func (s AuthStore) UpdatePasskeyUserCredentials(ctx context.Context, credentials auth.UpdatePasskeyUserCredentials) (*auth.PasskeyUser, error) {
	query := s.queries.UpdatePasskeyUserCredentials

	return pg.UpdateOne(ctx, credentials, query, toDBUpdatePasskeyUserCredentialsParams, fromDBPasskeyUser)
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

func (s AuthStore) PasskeyUsernameExists(ctx context.Context, username string) (bool, error) {
	query := s.queries.PasskeyUsernameExists

	resultIdentity := func(in bool) bool { return in }

	return pg.FetchScalar(ctx, username, query, resultIdentity)
}

func (s AuthStore) PasskeyEmailExists(ctx context.Context, email string) (bool, error) {
	query := s.queries.PasskeyUserEmailExists

	resultIdentity := func(in bool) bool { return in }

	return pg.FetchScalar(ctx, email, query, resultIdentity)
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
