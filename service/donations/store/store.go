package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/donations"
	"context"
	"github.com/jackc/pgx/v4/pgxpool"
)

type DonationStore struct {
	queries *db.Queries
	conn    *pgxpool.Conn
}

func NewDonationStore(conn *pgxpool.Conn) DonationStore {
	return DonationStore{
		queries: db.New(conn),
		conn:    conn,
	}
}

func (s DonationStore) CreateDonationWithPayment(ctx context.Context, donation donations.InsertDonation, payment donations.InsertDonationPayment) (*donations.Donation, error) {
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

	payment.DonationID = donationOut.ID

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

func (s DonationStore) GetDonationPlanByID(ctx context.Context, id int32) (*donations.DonationPlan, error) {
	query := s.queries.GetDonationPlanById

	return pg.FetchOne(ctx, id, query, fromDBDonationPlan)
}

func (s DonationStore) CreateDonationPlan(ctx context.Context, plan donations.InsertDonationPlan) (*donations.DonationPlan, error) {
	query := s.queries.InsertDonationPlan

	return pg.CreateOne(ctx, plan, query, toDBDonationPlanInsertParams, fromDBDonationPlan)
}

func (s DonationStore) UpdateDonationPlan(ctx context.Context, plan donations.UpdateDonationPlan) (*donations.DonationPlan, error) {
	query := s.queries.UpdateDonationPlan

	return pg.UpdateOne(ctx, plan, query, toDBDonationPlanUpdateParams, fromDBDonationPlan)
}

func (s DonationStore) GetDonationByID(ctx context.Context, id int32) (*donations.Donation, error) {
	query := s.queries.GetDonationById

	return pg.FetchOne(ctx, id, query, fromDBDonation)
}

func (s DonationStore) CreateDonation(ctx context.Context, donation donations.InsertDonation) (*donations.Donation, error) {
	query := s.queries.InsertDonation

	return pg.CreateOne(ctx, donation, query, toDBDonationInsertParams, fromDBDonation)
}

func (s DonationStore) UpdateDonation(ctx context.Context, donation donations.UpdateDonation) (*donations.Donation, error) {
	query := s.queries.UpdateDonation

	return pg.UpdateOne(ctx, donation, query, toDBDonationUpdateParams, fromDBDonation)
}

func (s DonationStore) GetDonationsByDonorID(ctx context.Context, donorID int32) ([]donations.Donation, error) {
	query := s.queries.GetDonationsByDonorId

	return pg.FetchMany(ctx, donorID, query, fromDBDonation)
}

func (s DonationStore) GetDonationsByMemberPaypalEmail(ctx context.Context, email string) ([]donations.Donation, error) {
	query := s.queries.GetDonationsByMemberPaypalEmail

	return pg.FetchMany(ctx, email, query, fromDBDonationRow)
}

func (s DonationStore) CreateDonationPayment(ctx context.Context, payment donations.InsertDonationPayment) (*donations.DonationPayment, error) {
	query := s.queries.InsertDonationPayment

	return pg.CreateOne(ctx, payment, query, toDBDonationPaymentInsertParams, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentByID(ctx context.Context, id int32) (*donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentById

	return pg.FetchOne(ctx, id, query, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentsByDonationID(ctx context.Context, donationID int32) ([]donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentsByDonationId

	return pg.FetchMany(ctx, donationID, query, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentsByMemberPaypalEmail(ctx context.Context, email string) ([]donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentsByMemberPaypalEmail

	return pg.FetchMany(ctx, email, query, fromDBDonationPayment)
}
