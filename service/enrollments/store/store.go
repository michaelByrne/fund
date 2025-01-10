package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/enrollments"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EnrollmentStore struct {
	queries *db.Queries
	conn    *pgxpool.Pool
}

func NewEnrollmentStore(conn *pgxpool.Pool) EnrollmentStore {
	return EnrollmentStore{
		conn:    conn,
		queries: db.New(conn),
	}
}

func (s EnrollmentStore) InsertEnrollment(ctx context.Context, arg enrollments.InsertEnrollment) (*enrollments.Enrollment, error) {
	query := s.queries.InsertEnrollment

	return pg.UpsertOne(ctx, arg, query, toDBEnrollmentParams, fromDBEnrollment)
}

func (s EnrollmentStore) UpdatePaypalEmail(ctx context.Context, arg enrollments.UpdatePaypalEmail) (*enrollments.PayeeMember, error) {
	query := s.queries.UpdatePaypalEmail

	return pg.UpdateOne(ctx, arg, query, toDBUpdatePaypalEmailParams, fromDBPayeeMember)
}

func (s EnrollmentStore) InsertEnrollmentWithPaypalEmail(ctx context.Context, insertEnrollment enrollments.InsertEnrollment, updatePaypalEmail enrollments.UpdatePaypalEmail) (*enrollments.Enrollment, error) {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	enrollmentQuery := txQueries.InsertEnrollment

	enrollment, err := pg.UpsertOne(ctx, insertEnrollment, enrollmentQuery, toDBEnrollmentParams, fromDBEnrollment)
	if err != nil {
		return nil, err
	}

	paypalEmailQuery := txQueries.UpdatePaypalEmail

	_, err = pg.UpdateOne(ctx, updatePaypalEmail, paypalEmailQuery, toDBUpdatePaypalEmailParams, fromDBPayeeMember)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return enrollment, nil
}

func (s EnrollmentStore) GetEnrollmentByMemberID(ctx context.Context, arg enrollments.GetEnrollmentForFundByMemberID) (*enrollments.Enrollment, error) {
	query := s.queries.GetEnrollmentForFundByMemberId

	return pg.FetchOne(ctx, arg, query, toDBGetEnrollmentForFundByMemberIDParams, fromDBEnrollment)
}

func (s EnrollmentStore) FundEnrollmentExists(ctx context.Context, arg enrollments.FundEnrollmentExists) (*bool, error) {
	query := s.queries.FundEnrollmentExists

	resultIdentity := func(result bool) bool { return result }

	return pg.FetchOne(ctx, arg, query, toDBFundEnrollmentExistsParams, resultIdentity)
}

func (s EnrollmentStore) GetActiveEnrollmentsForFund(ctx context.Context, arg uuid.UUID) ([]enrollments.Enrollment, error) {
	query := s.queries.GetActiveEnrollmentsByFundId

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	return pg.FetchMany(ctx, arg, query, argIdentity, fromDBEnrollment)
}
