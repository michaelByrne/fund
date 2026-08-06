package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/donations"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
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

func uuidIdentity(id uuid.UUID) uuid.UUID { return id }

func (s DonationStore) GetTotalDonatedByMemberID(ctx context.Context, id uuid.UUID) (int64, error) {
	query := s.queries.GetTotalDonatedByMember

	resultIdentity := func(amount int64) int64 { return amount }

	return pg.FetchScalar(ctx, id, query, resultIdentity)
}

func (s DonationStore) GetActiveFunds(ctx context.Context, arg string) ([]donations.Fund, error) {
	query := s.queries.GetActiveFunds

	argIn := func(freq string) db.PayoutFrequency { return db.PayoutFrequency(freq) }

	return pg.FetchMany(ctx, arg, query, argIn, fromDBFundRow)
}

func (s DonationStore) GetMonthlyDonationTotalsForFund(ctx context.Context, id uuid.UUID) ([]donations.MonthTotal, error) {
	query := s.queries.GetMonthlyTotalsByFund

	return pg.FetchMany(ctx, id, query, uuidIdentity, fromDBMonthlyDonationTotal)
}

func (s DonationStore) SetDonationToActiveBySubscriptionID(ctx context.Context, id string) (*donations.Donation, error) {
	query := s.queries.SetDonationsToActiveBySubscriptionId

	argIdentity := func(id string) pgtype.Text {
		return pgtype.Text{
			String: id,
			Valid:  true,
		}
	}

	return pg.UpdateOne(ctx, id, query, argIdentity, fromDBDonation)
}

func (s DonationStore) SetFundAndDonationsToInactive(ctx context.Context, id uuid.UUID) ([]donations.Donation, error) {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	fundQuery := txQueries.SetFundToInactive

	_, err = pg.UpdateOne(ctx, id, fundQuery, argIdentity, fromDBFund)
	if err != nil {
		return nil, err
	}

	donationQuery := txQueries.SetDonationsToInactiveByFundId

	updated, err := pg.UpdateMany(ctx, id, donationQuery, argIdentity, fromDBDonation)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s DonationStore) SetFundAndDonationsToActive(ctx context.Context, id uuid.UUID) ([]donations.Donation, error) {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	fundQuery := txQueries.SetFundToActive

	_, err = pg.UpdateOne(ctx, id, fundQuery, argIdentity, fromDBFund)
	if err != nil {
		return nil, err
	}

	donationQuery := txQueries.SetDonationsToActiveByFundId

	updated, err := pg.UpdateMany(ctx, id, donationQuery, argIdentity, fromDBDonation)
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s DonationStore) SetDonationsToInactiveByDonorID(ctx context.Context, donorID uuid.UUID) ([]donations.Donation, error) {
	query := s.queries.SetDonationsToInactiveByDonorId

	return pg.UpdateMany(ctx, donorID, query, uuidIdentity, fromDBDonation)
}

func (s DonationStore) SetDonationsToActive(ctx context.Context, ids []uuid.UUID) ([]donations.Donation, error) {
	query := s.queries.SetDonationsToActive

	argListIdentity := func(ids []uuid.UUID) []uuid.UUID { return ids }

	return pg.UpdateMany(ctx, ids, query, argListIdentity, fromDBDonation)
}

func (s DonationStore) SetDonationToInactive(ctx context.Context, arg donations.DeactivateDonation) (*donations.Donation, error) {
	query := s.queries.SetDonationToInactive

	return pg.UpdateOne(ctx, arg, query, toDBSetDonationToInactive, fromDBDonation)
}

func (s DonationStore) SetDonationToInactiveBySubscriptionID(ctx context.Context, arg donations.DeactivateDonationBySubscription) (*donations.Donation, error) {
	query := s.queries.SetDonationToInactiveBySubscriptionId

	return pg.UpdateOne(ctx, arg, query, toDBSetDonationToInactiveBySubscriptionIDParams, fromDBDonation)
}

func (s DonationStore) GetFunds(ctx context.Context) ([]donations.Fund, error) {
	query := s.queries.GetFunds

	return pg.FetchAll(ctx, query, fromDBFund)
}

func (s DonationStore) GetFundByID(ctx context.Context, id uuid.UUID) (*donations.Fund, error) {
	query := s.queries.GetFundById

	return pg.FetchOne(ctx, id, query, uuidIdentity, fromDBFundByID)
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

	paymentOut, err := pg.CreateOneIfNew(ctx, payment, txQueries.InsertDonationPayment, toDBDonationPaymentInsertParams, fromDBDonationPayment)
	if err != nil {
		return nil, err
	}

	// The provider payment is already on record, so this is a repeat of a
	// completion we have already handled -- a double-submitted form, or a retry.
	// Rolling back discards the donation row this transaction would otherwise
	// have added alongside it.
	if paymentOut == nil {
		return nil, donations.ErrPaymentAlreadyRecorded
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

	return pg.FetchOne(ctx, id, query, uuidIdentity, fromDBDonationPlan)
}

func (s DonationStore) GetDonationByID(ctx context.Context, id uuid.UUID) (*donations.Donation, error) {
	query := s.queries.GetDonationById

	argIdentity := func(id uuid.UUID) uuid.UUID { return id }

	return pg.FetchOne(ctx, id, query, argIdentity, fromDBDonation)
}

func (s DonationStore) GetTotalDonatedByFundID(ctx context.Context, id uuid.UUID) (int64, error) {
	query := s.queries.GetTotalDonatedByFund

	resultIdentity := func(amount int64) int64 { return amount }

	return pg.FetchScalar(ctx, id, query, resultIdentity)
}

func (s DonationStore) InsertDonation(ctx context.Context, donation donations.InsertDonation) (*donations.Donation, error) {
	query := s.queries.InsertDonation

	return pg.CreateOne(ctx, donation, query, toDBDonationInsertParams, fromDBDonation)
}

func (s DonationStore) GetDonationsForDonor(ctx context.Context, donorID uuid.UUID) ([]donations.MemberDonation, error) {
	rows, err := s.queries.GetDonationsForDonor(ctx, donorID)
	if err != nil {
		return nil, err
	}

	out := make([]donations.MemberDonation, 0, len(rows))
	for _, row := range rows {
		out = append(out, donations.NewMemberDonation(donations.MemberDonationRow{
			ID:              row.ID,
			FundID:          row.FundID,
			FundName:        row.FundName,
			FundActive:      row.FundActive,
			Recurring:       row.Recurring,
			Active:          row.Active,
			InactiveReason:  row.InactiveReason.String,
			HasSubscription: row.ProviderSubscriptionID.Valid && row.ProviderSubscriptionID.String != "",
			TotalGivenCents: row.TotalGivenCents,
			PlanAmountCents: row.PlanAmountCents.Int32,
			PlanInterval:    string(row.PlanIntervalUnit.IntervalUnit),
			Started:         row.Created.Time,
			LastPayment:     timePtr(row.LastPaymentAt),
		}))
	}

	return out, nil
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}

	out := t.Time

	return &out
}

func (s DonationStore) GetDonationsByDonorID(ctx context.Context, donorID uuid.UUID) ([]donations.Donation, error) {
	query := s.queries.GetDonationsByDonorId

	return pg.FetchMany(ctx, donorID, query, uuidIdentity, fromDBDonation)
}

// ReactivateSuspendedDonation returns the donation it brought back, or nil when
// there was nothing to bring back -- the subscription is unknown, already active,
// or was deactivated for a reason a payment must not overturn.
func (s DonationStore) ReactivateSuspendedDonation(ctx context.Context, subscriptionID string) (*donations.Donation, error) {
	rows, err := s.queries.ReactivateSuspendedDonationBySubscriptionId(ctx,
		pgtype.Text{String: subscriptionID, Valid: true})
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}

	donation := fromDBDonation(rows[0])

	return &donation, nil
}

// SetDonationPaymentRefunded records money returned for a payment. Returns nil
// when the payment is unknown, or already carries this refunded total -- which is
// what a redelivered refund webhook looks like.
func (s DonationStore) SetDonationPaymentRefunded(ctx context.Context, providerPaymentID string, refundedCents int32) (*donations.RefundedPayment, error) {
	rows, err := s.queries.SetDonationPaymentRefunded(ctx, db.SetDonationPaymentRefundedParams{
		PaypalPaymentID: providerPaymentID,
		RefundedCents:   refundedCents,
	})
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}

	row := rows[0]

	return &donations.RefundedPayment{
		PaymentID:               row.PaymentID,
		DonationID:              row.DonationID,
		FundID:                  row.FundID,
		DonorID:                 row.DonorID,
		AmountCents:             row.AmountCents,
		RefundedCents:           row.RefundedCents,
		PreviouslyRefundedCents: row.PreviouslyRefundedCents,
	}, nil
}

func (s DonationStore) InsertDonationPayment(ctx context.Context, payment donations.InsertDonationPayment) (*donations.DonationPayment, error) {
	query := s.queries.InsertDonationPayment

	// Returns nil, nil when the provider payment is already recorded. Callers
	// must treat that as "nothing more to do" rather than as a payment.
	return pg.CreateOneIfNew(ctx, payment, query, toDBDonationPaymentInsertParams, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentByID(ctx context.Context, id uuid.UUID) (*donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentById

	return pg.FetchOne(ctx, id, query, uuidIdentity, fromDBDonationPayment)
}

func (s DonationStore) GetDonationPaymentsByDonationID(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error) {
	query := s.queries.GetDonationPaymentsByDonationId

	return pg.FetchMany(ctx, donationID, query, uuidIdentity, fromDBDonationPayment)
}

func (s DonationStore) GetDonationByProviderSubscriptionID(ctx context.Context, id string) (*donations.Donation, error) {
	query := s.queries.GetDonationByProviderSubscriptionId

	argTransform := func(id string) pgtype.Text {
		return pgtype.Text{
			String: id,
			Valid:  true,
		}
	}

	return pg.FetchOne(ctx, id, query, argTransform, fromDBDonation)
}

func (s DonationStore) GetRecurringDonationsForFund(ctx context.Context, arg donations.GetRecurringDonationsForFundRequest) ([]donations.Donation, error) {
	query := s.queries.GetRecurringDonationsForFund

	return pg.FetchMany(ctx, arg, query, toDBGetRecurringDonationsForFundParams, fromDBDonation)
}

func (s DonationStore) GetPaymentsForDonation(ctx context.Context, donationID uuid.UUID) ([]donations.DonationPayment, error) {
	query := s.queries.GetPaymentsForDonation

	return pg.FetchMany(ctx, donationID, query, uuidIdentity, fromDBDonationPayment)
}

func (s DonationStore) GetOneTimeDonationsForFund(ctx context.Context, arg donations.GetOneTimeDonationsForFundRequest) ([]donations.Donation, error) {
	query := s.queries.GetOneTimeDonationsForFund

	return pg.FetchMany(ctx, arg, query, toDBGetOneTimeDonationsForFundParams, fromDBDonation)
}

func (s DonationStore) UpdatePaymentPaypalFee(ctx context.Context, arg donations.UpdatePaymentPaypalFee) (*donations.DonationPayment, error) {
	query := s.queries.UpdateDonationPaymentPaypalFee

	return pg.UpdateOne(ctx, arg, query, toDBUpdatePaymentPaypalFeeParams, fromDBDonationPayment)
}

func (s DonationStore) GetAllFundsWithStats(ctx context.Context) ([]donations.Fund, error) {
	return pg.FetchAll(ctx, s.queries.GetAllFundsWithStats, fromDBAllFundsRow)
}

func (s DonationStore) GetClosedFundsWithStats(ctx context.Context) ([]donations.ClosedFund, error) {
	return pg.FetchAll(ctx, s.queries.GetClosedFundsWithStats, fromDBClosedFundRow)
}

func (s DonationStore) GetExpiredActiveFunds(ctx context.Context) ([]donations.Fund, error) {
	return pg.FetchAll(ctx, s.queries.GetExpiredActiveFunds, fromDBExpiredFund)
}

func (s DonationStore) GetFundPayoutStats(ctx context.Context, fundID uuid.UUID) (donations.PayoutStats, error) {
	return pg.FetchScalar(ctx, fundID, s.queries.GetFundPayoutStats, fromDBPayoutStats)
}
