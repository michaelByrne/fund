package store

import (
	"context"
	"time"

	"boardfund/db"
	"boardfund/service/fundevents"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventStore struct {
	queries *db.Queries
	conn    *pgxpool.Pool
}

func NewEventStore(conn *pgxpool.Pool) EventStore {
	return EventStore{
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

func nullInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}

	return pgtype.Int4{Int32: *v, Valid: true}
}

func int32Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}

	out := v.Int32

	return &out
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// occurredAt leaves the column unset when the caller did not supply a time, so
// the query's COALESCE falls back to now(). Recording "when we heard about it"
// as "when it happened" would quietly misdate every webhook.
func occurredAt(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}

	return pgtype.Timestamptz{Time: t, Valid: true}
}

func (s EventStore) InsertFundEvent(ctx context.Context, arg fundevents.Record) (*fundevents.Event, error) {
	inserted, err := s.queries.InsertFundEvent(ctx, db.InsertFundEventParams{
		ID:              uuid.New(),
		FundID:          arg.FundID,
		Kind:            db.FundEventKind(arg.Kind),
		OccurredAt:      occurredAt(arg.OccurredAt),
		ActorMemberID:   nullUUID(arg.ActorMemberID),
		SubjectMemberID: nullUUID(arg.SubjectMemberID),
		AmountCents:     nullInt4(arg.AmountCents),
		Detail:          text(arg.Detail),
		ReferenceID:     nullUUID(arg.ReferenceID),
	})
	if err != nil {
		return nil, err
	}

	event := fromDB(inserted, "", "")

	return &event, nil
}

func (s EventStore) GetFundEvents(ctx context.Context, fundID uuid.UUID, limit int32) ([]fundevents.Event, error) {
	rows, err := s.queries.GetFundEvents(ctx, db.GetFundEventsParams{
		FundID: fundID,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	events := make([]fundevents.Event, len(rows))
	for i, row := range rows {
		events[i] = fromDB(row.FundEvent, row.ActorName.String, row.SubjectName.String)
	}

	return events, nil
}

func fromDB(e db.FundEvent, actorName, subjectName string) fundevents.Event {
	return fundevents.Event{
		ID:              e.ID,
		FundID:          e.FundID,
		Kind:            fundevents.Kind(e.Kind),
		OccurredAt:      e.OccurredAt.Time,
		ActorMemberID:   uuidPtr(e.ActorMemberID),
		ActorName:       actorName,
		SubjectMemberID: uuidPtr(e.SubjectMemberID),
		SubjectName:     subjectName,
		AmountCents:     int32Ptr(e.AmountCents),
		Detail:          e.Detail.String,
		ReferenceID:     uuidPtr(e.ReferenceID),
		Created:         e.Created.Time,
	}
}
