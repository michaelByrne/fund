package store

import (
	"context"
	"time"

	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/payouts"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PayoutStore struct {
	queries *db.Queries
	conn    *pgxpool.Pool
}

func NewPayoutStore(conn *pgxpool.Pool) PayoutStore {
	return PayoutStore{
		queries: db.New(conn),
		conn:    conn,
	}
}

func uuidIdentity(id uuid.UUID) uuid.UUID { return id }

// CreateBatchWithPayouts writes the batch and every one of its payout rows in a
// single transaction. A partially-written batch would be worse than no batch: the
// submitter would send fewer items than the batch total claims, and the difference
// would look like a provider failure rather than our own.
func (s PayoutStore) CreateBatchWithPayouts(ctx context.Context, batch payouts.InsertBatch, items []payouts.InsertPayout) (*payouts.Batch, []payouts.Payout, error) {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	created, err := pg.CreateOne(ctx, batch, txQueries.InsertBatchPayout, toDBInsertBatchParams, fromDBBatch)
	if err != nil {
		return nil, nil, err
	}

	written := make([]payouts.Payout, 0, len(items))
	for _, item := range items {
		payout, errInner := pg.CreateOne(ctx, item, txQueries.InsertPayout, toDBInsertPayoutParams, fromDBPayout)
		if errInner != nil {
			return nil, nil, errInner
		}

		written = append(written, *payout)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, nil, err
	}

	return created, written, nil
}

func (s PayoutStore) GetBatchByID(ctx context.Context, id uuid.UUID) (*payouts.Batch, error) {
	return pg.FetchOne(ctx, id, s.queries.GetBatchPayoutById, uuidIdentity, fromDBBatch)
}

func (s PayoutStore) GetBatchBySenderBatchID(ctx context.Context, id uuid.UUID) (*payouts.Batch, error) {
	return pg.FetchOne(ctx, id, s.queries.GetBatchPayoutBySenderBatchId, uuidIdentity, fromDBBatch)
}

func (s PayoutStore) GetBatchesForFund(ctx context.Context, fundID uuid.UUID) ([]payouts.Batch, error) {
	return pg.FetchMany(ctx, fundID, s.queries.GetBatchPayoutsByFundId, uuidIdentity, fromDBBatch)
}

func (s PayoutStore) GetBatchesByStatus(ctx context.Context, status payouts.Status) ([]payouts.Batch, error) {
	argIn := func(s payouts.Status) db.PayoutStatus { return db.PayoutStatus(s) }

	return pg.FetchMany(ctx, status, s.queries.GetBatchPayoutsByStatus, argIn, fromDBBatch)
}

func (s PayoutStore) GetPayoutsForBatch(ctx context.Context, batchID uuid.UUID) ([]payouts.Payout, error) {
	return pg.FetchMany(ctx, batchID, s.queries.GetPayoutsByBatchId, uuidIdentity, fromDBPayout)
}

func (s PayoutStore) IsFundActive(ctx context.Context, fundID uuid.UUID) (bool, error) {
	return s.queries.IsFundActive(ctx, fundID)
}

func (s PayoutStore) GetEnrollmentsForPayout(ctx context.Context, fundID uuid.UUID) ([]payouts.PayoutEnrollment, error) {
	return pg.FetchMany(ctx, fundID, s.queries.GetActiveEnrollmentsForPayout, uuidIdentity, fromDBPayoutEnrollment)
}

func (s PayoutStore) ApproveBatch(ctx context.Context, arg payouts.ApproveBatch) (*payouts.Batch, error) {
	return pg.UpdateOne(ctx, arg, s.queries.ApproveBatchPayout, toDBApproveBatchParams, fromDBBatch)
}

func (s PayoutStore) RejectBatch(ctx context.Context, arg payouts.RejectBatch) (*payouts.Batch, error) {
	return pg.UpdateOne(ctx, arg, s.queries.RejectBatchPayout, toDBRejectBatchParams, fromDBBatch)
}

func (s PayoutStore) CancelExpiredBatches(ctx context.Context) ([]payouts.Batch, error) {
	dbBatches, err := s.queries.CancelExpiredBatchPayouts(ctx)
	if err != nil {
		return nil, err
	}

	cancelled := make([]payouts.Batch, len(dbBatches))
	for i, b := range dbBatches {
		cancelled[i] = fromDBBatch(b)
	}

	return cancelled, nil
}

func (s PayoutStore) GetBatchesNeedingReminder(ctx context.Context, within time.Duration) ([]payouts.Batch, error) {
	argIn := func(d time.Duration) pgtype.Interval {
		return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
	}

	return pg.FetchMany(ctx, within, s.queries.GetBatchPayoutsNeedingReminder, argIn, fromDBBatch)
}

func (s PayoutStore) MarkReminderSent(ctx context.Context, batchID uuid.UUID) (*payouts.Batch, error) {
	return pg.UpdateOne(ctx, batchID, s.queries.MarkBatchPayoutReminderSent, uuidIdentity, fromDBBatch)
}

func (s PayoutStore) SetBatchSubmitted(ctx context.Context, arg payouts.SetBatchSubmitted) (*payouts.Batch, error) {
	return pg.UpdateOne(ctx, arg, s.queries.SetBatchPayoutSubmitted, toDBSetBatchSubmittedParams, fromDBBatch)
}

func (s PayoutStore) SetBatchStatus(ctx context.Context, arg payouts.SetBatchStatus) (*payouts.Batch, error) {
	return pg.UpdateOne(ctx, arg, s.queries.SetBatchPayoutStatus, toDBSetBatchStatusParams, fromDBBatch)
}

func (s PayoutStore) SetPayoutProviderItemID(ctx context.Context, arg payouts.SetPayoutProviderItem) (*payouts.Payout, error) {
	return pg.UpdateOne(ctx, arg, s.queries.SetPayoutProviderItemId, toDBSetPayoutProviderItemParams, fromDBPayout)
}

func (s PayoutStore) SetPayoutResult(ctx context.Context, arg payouts.SetPayoutResult) (*payouts.Payout, error) {
	return pg.UpdateOne(ctx, arg, s.queries.SetPayoutResultById, toDBSetPayoutResultParams, fromDBPayout)
}

func (s PayoutStore) SetPayoutStatusByProviderItemID(ctx context.Context, arg payouts.SetPayoutStatusByItem) (*payouts.Payout, error) {
	return pg.UpdateOne(ctx, arg, s.queries.SetPayoutStatusByProviderItemId, toDBSetPayoutStatusByItemParams, fromDBPayout)
}
