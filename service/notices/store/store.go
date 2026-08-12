package store

import (
	"context"

	"boardfund/db"
	"boardfund/service/notices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NoticeStore struct {
	queries *db.Queries
	conn    *pgxpool.Pool
}

func NewNoticeStore(conn *pgxpool.Pool) NoticeStore {
	return NoticeStore{
		queries: db.New(conn),
		conn:    conn,
	}
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}

	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}

	out := id.UUID

	return &out
}

func (s NoticeStore) InsertNotice(ctx context.Context, body string, actorID *uuid.UUID) (*notices.Notice, error) {
	row, err := s.queries.InsertNotice(ctx, db.InsertNoticeParams{
		ID:        uuid.New(),
		Body:      body,
		CreatedBy: nullUUID(actorID),
	})
	if err != nil {
		return nil, err
	}

	notice := fromDB(row)

	return &notice, nil
}

func (s NoticeStore) GetActiveNotices(ctx context.Context) ([]notices.Notice, error) {
	rows, err := s.queries.GetActiveNotices(ctx)
	if err != nil {
		return nil, err
	}

	return allFromDB(rows), nil
}

func (s NoticeStore) GetNotices(ctx context.Context) ([]notices.Notice, error) {
	rows, err := s.queries.GetNotices(ctx)
	if err != nil {
		return nil, err
	}

	return allFromDB(rows), nil
}

func (s NoticeStore) SetNoticeActive(
	ctx context.Context,
	id uuid.UUID,
	active bool,
	actorID *uuid.UUID,
) (*notices.Notice, error) {
	row, err := s.queries.SetNoticeActive(ctx, db.SetNoticeActiveParams{
		ID:        id,
		Active:    active,
		UpdatedBy: nullUUID(actorID),
	})
	if err != nil {
		return nil, err
	}

	notice := fromDB(row)

	return &notice, nil
}

func allFromDB(rows []db.Notice) []notices.Notice {
	out := make([]notices.Notice, len(rows))
	for i, row := range rows {
		out[i] = fromDB(row)
	}

	return out
}

func fromDB(row db.Notice) notices.Notice {
	return notices.Notice{
		ID:        row.ID,
		Body:      row.Body,
		Active:    row.Active,
		CreatedBy: uuidPtr(row.CreatedBy),
		UpdatedBy: uuidPtr(row.UpdatedBy),
		Created:   row.Created.Time,
		Updated:   row.Updated.Time,
	}
}
