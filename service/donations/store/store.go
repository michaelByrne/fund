package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/donations"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DonationStore struct {
	queries *db.Queries
	conn    *pgxpool.Pool
}

func NewDonationStore(conn *pgxpool.Pool) DonationStore {
	return DonationStore{
		queries: db.New(conn),
		conn:    conn,
	}
}

func (s DonationStore) GetFunds(ctx context.Context) ([]donations.Fund, error) {
	query := s.queries.GetFunds

	return pg.FetchAll(ctx, query, fromDBFund)
}

func (s DonationStore) GetFundByID(ctx context.Context, id uuid.UUID) (*donations.Fund, error) {
	query := s.queries.GetFundById

	return pg.FetchOne(ctx, id, query, fromDBFund)
}

func (s DonationStore) UpdateFund(ctx context.Context, fund donations.UpdateFund) (*donations.Fund, error) {
	query := s.queries.UpdateFund

	return pg.UpsertOne(ctx, fund, query, toDBFundUpdateParams, fromDBFund)
}

func (s DonationStore) InsertFund(ctx context.Context, fund donations.InsertFund) (*donations.Fund, error) {
	query := s.queries.InsertFund

	return pg.CreateOne(ctx, fund, query, toDBFundInsertParams, fromDBFund)
}

func (s DonationStore) UpsertDonationPlan(ctx context.Context, plan donations.UpsertDonationPlan) (*donations.DonationPlan, error) {
	query := s.queries.UpsertDonationPlan

	return pg.UpsertOne(ctx, plan, query, toDBDonationPlanUpsertParams, fromDBDonationPlan)
}

func (s DonationStore) InsertDonationWithPayment(ctx context.Context, donation donations.InsertDonation, payment donations.InsertDonationPayment) (*donations.Donation, error) {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	donationOut, err := pg.CreateOne(ctx, donation, txQueries.InsertDonation, toDBDonationInsertParams, fromDBDonation)
	if err != nil {
		return nil, err
	}

	paymentOut, err := pg.CreateOne(ctx, payment, txQueries.InsertDonationPayment, toDBDonationPaymentInsertParams, fromDBDonationPayment)
	if err != nil {
		return nil, err
	}

	donationOut.Payment = paymentOut

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return donationOut, nil
}

func (s DonationStore) GetDonationPlanByID(ctx context.Context, id uuid.UUID) (*donations.DonationPlan, error) {
	query := s.queries.GetDonationPlanById

	return pg.FetchOne(ctx, id, query, fromDBDonationPlan)
}

func (s DonationStore) GetDonationByID(ctx context.Context, id uuid.UUID) (*donations.Donation, error) {
	query := s.queries.GetDonationById

	return pg.FetchOne(ctx, id, query, fromDBDonation)
}

func (s DonationStore) InsertDonation(ctx context.Context, donation donations.InsertDonation) (*donations.Donation, error) {
	query := s.queries.InsertDonation

	return pg.CreateOne(ctx, donation, query, toDBDonationInsertParams, fromDBDonation)
}

func (s DonationStore) GetDonationsByDonorID(ctx context.Context, donorID uuid.UUID) ([]donations.Donation, error) {
	query := s.queries.GetDonationsByDonorId

	return pg.FetchMany(ctx, donorID, query, fromDBDonation)
}

func (s DonationStore) GetDonationsByMemberPaypalEmail(ctx context.Context, email string) ([]donations.Donation, error) {
	query := s.queries.GetDonationsByMemberPaypalEmail

	return pg.FetchMany(ctx, email, query, fromDBDonation)
}

func (s DonationStore) InsertDonationPayment(ctx context.Context, payment donations.InsertDonationPayment) (*donations.DonationPayment, error) {
	query := s.queries.InsertDonationPayment

	return pg.CreateOne(ctx, payment, query, toDBDonationPaymentInsertParams, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentByID(ctx context.Context, id uuid.UUID) (*donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentById

	return pg.FetchOne(ctx, id, query, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentsByDonationID(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentsByDonationId

	return pg.FetchMany(ctx, donationID, query, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentsByMemberPaypalEmail(ctx context.Context, email string) ([]donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentsByMemberPaypalEmail

	return pg.FetchMany(ctx, email, query, fromDBDonationPayment)
}
