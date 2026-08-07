package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/donations"
	"boardfund/service/finance"
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// SetPaymentReconciliation records what reconciliation saw at the provider.
func (s DonationStore) SetPaymentReconciliation(ctx context.Context, arg donations.SetPaymentReconciliation) error {
	_, err := s.queries.SetPaymentReconciliation(ctx, db.SetPaymentReconciliationParams{
		ID:                  arg.PaymentID,
		ProviderStatus:      nullText(arg.ProviderStatus),
		ProviderAmountCents: nullInt32(arg.ProviderAmountCents),
		ProviderFeeCents:    nullInt32(arg.ProviderFeeCents),
	})

	return err
}

// GetFundPaymentsForAudit is what the audit page shows.
func (s DonationStore) GetFundPaymentsForAudit(ctx context.Context, fundID uuid.UUID) ([]finance.AuditPayment, error) {
	rows, err := s.queries.GetFundPaymentsForAudit(ctx, fundID)
	if err != nil {
		return nil, err
	}

	out := make([]finance.AuditPayment, 0, len(rows))
	for _, row := range rows {
		out = append(out, finance.AuditPayment{
			DonationID:          row.DonationID,
			PaymentID:           row.ID,
			ProviderPaymentID:   row.PaypalPaymentID,
			DonorName:           row.DonorName.String,
			Recurring:           row.Recurring,
			AmountCents:         row.AmountCents,
			RefundedCents:       row.RefundedCents,
			FeeAmountCents:      row.ProviderFeeCents,
			ProviderStatus:      row.ProviderStatus.String,
			ProviderAmountCents: row.ProviderAmountCents.Int32,
			ReconciledAt:        reconciledAt(row.ReconciledAt),
			Created:             row.Created.Time,
		})
	}

	return out, nil
}

func nullText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}

	return pgtype.Text{String: *s, Valid: true}
}

func nullInt32(n *int32) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}

	return pgtype.Int4{Int32: *n, Valid: true}
}

// reconciledAt distinguishes "never checked" from "checked", which is the whole
// point of the column.
func reconciledAt(t db.NullDBTime) *time.Time {
	if !t.Valid {
		return nil
	}

	out := t.DBTime.Time

	return &out
}

func (s DonationStore) MemberHasGivenToFund(ctx context.Context, fundID, memberID uuid.UUID) (bool, error) {
	return s.queries.MemberHasGivenToFund(ctx, db.MemberHasGivenToFundParams{
		FundID:  fundID,
		DonorID: memberID,
	})
}

func (s DonationStore) UpsertFundNote(ctx context.Context, arg donations.UpsertFundNote) (*donations.FundNote, error) {
	row, err := s.queries.UpsertFundNote(ctx, db.UpsertFundNoteParams{
		ID:        uuid.New(),
		FundID:    arg.FundID,
		MemberID:  arg.MemberID,
		Body:      arg.Body,
		Anonymous: arg.Anonymous,
	})
	if err != nil {
		return nil, err
	}

	// No author name: the caller already knows whose it is, and reading it back
	// would mean a join this does not need.
	return &donations.FundNote{
		ID:        row.ID,
		FundID:    row.FundID,
		MemberID:  row.MemberID,
		Body:      row.Body,
		Anonymous: row.Anonymous,
		Created:   row.Created.Time,
		Updated:   row.Updated.Time,
	}, nil
}

func (s DonationStore) GetFundNotes(ctx context.Context, fundID uuid.UUID) ([]donations.FundNote, error) {
	rows, err := s.queries.GetFundNotes(ctx, fundID)
	if err != nil {
		return nil, err
	}

	notes := make([]donations.FundNote, 0, len(rows))
	for _, row := range rows {
		note := donations.FundNote{
			ID:        row.ID,
			FundID:    row.FundID,
			MemberID:  row.MemberID,
			Body:      row.Body,
			Anonymous: row.Anonymous,
			Created:   row.Created.Time,
			Updated:   row.Updated.Time,
		}

		// Withheld here rather than in the template. A name left on the struct is
		// a name one careless render away from being shown.
		if !row.Anonymous {
			note.AuthorName = row.AuthorName.String
		}

		notes = append(notes, note)
	}

	return notes, nil
}

// GetFundNotesForMember is every note this member has up, keyed by the fund it
// is on, so a page showing many donations can draw them all from one query.
func (s DonationStore) GetFundNotesForMember(ctx context.Context, memberID uuid.UUID) (map[uuid.UUID]donations.FundNote, error) {
	rows, err := s.queries.GetFundNotesForMember(ctx, memberID)
	if err != nil {
		return nil, err
	}

	notes := make(map[uuid.UUID]donations.FundNote, len(rows))
	for _, row := range rows {
		notes[row.FundID] = donations.FundNote{
			ID:        row.ID,
			FundID:    row.FundID,
			MemberID:  row.MemberID,
			Body:      row.Body,
			Anonymous: row.Anonymous,
			Created:   row.Created.Time,
			Updated:   row.Updated.Time,
		}
	}

	return notes, nil
}

func (s DonationStore) GetFundNoteForMember(ctx context.Context, fundID, memberID uuid.UUID) (*donations.FundNote, error) {
	row, err := s.queries.GetFundNoteForMember(ctx, db.GetFundNoteForMemberParams{
		FundID:   fundID,
		MemberID: memberID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No note yet, which is the ordinary state of most donors.
			return nil, nil
		}

		return nil, err
	}

	// A removed note is not shown back to its author either: editing it would
	// otherwise be a way to find out it had been taken down.
	if row.RemovedAt.Valid {
		return nil, nil
	}

	return &donations.FundNote{
		ID:        row.ID,
		FundID:    row.FundID,
		MemberID:  row.MemberID,
		Body:      row.Body,
		Anonymous: row.Anonymous,
		Created:   row.Created.Time,
		Updated:   row.Updated.Time,
	}, nil
}

// RemoveOwnFundNote takes down the note this member wrote on this fund.
//
// Scoped by member in the statement itself rather than by checking ownership
// first and then deleting: there is no window between the two, and no way for a
// caller to name a note that is not theirs.
func (s DonationStore) RemoveOwnFundNote(ctx context.Context, fundID, memberID uuid.UUID) error {
	_, err := s.queries.RemoveOwnFundNote(ctx, db.RemoveOwnFundNoteParams{
		FundID:   fundID,
		MemberID: memberID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// No note, or already down. Either way the donor asked for it gone and it
		// is gone.
		return nil
	}

	return err
}

func (s DonationStore) RemoveFundNote(ctx context.Context, noteID, actorID uuid.UUID) error {
	_, err := s.queries.RemoveFundNote(ctx, db.RemoveFundNoteParams{
		ID:        noteID,
		RemovedBy: uuid.NullUUID{UUID: actorID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Already removed. The moderator asked for it gone and it is gone.
		return nil
	}

	return err
}
