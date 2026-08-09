package store

import (
	"context"
	"time"

	"boardfund/db"
	"boardfund/service/adminevents"

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

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// occurredAt leaves the column unset when the caller did not supply a time, so
// the query's COALESCE falls back to now().
func occurredAt(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}

	return pgtype.Timestamptz{Time: t, Valid: true}
}

func (s EventStore) InsertAdminEvent(ctx context.Context, arg adminevents.Record) (*adminevents.Event, error) {
	inserted, err := s.queries.InsertAdminEvent(ctx, db.InsertAdminEventParams{
		ID:              uuid.New(),
		Kind:            db.AdminEventKind(arg.Kind),
		OccurredAt:      occurredAt(arg.OccurredAt),
		ActorMemberID:   nullUUID(arg.ActorMemberID),
		SubjectMemberID: nullUUID(arg.SubjectMemberID),
		SubjectLabel:    text(arg.SubjectLabel),
		Detail:          text(arg.Detail),
	})
	if err != nil {
		return nil, err
	}

	event := fromDB(inserted, "", "")

	return &event, nil
}

func (s EventStore) GetAdminEvents(ctx context.Context, limit int32) ([]adminevents.Event, error) {
	rows, err := s.queries.GetAdminEvents(ctx, limit)
	if err != nil {
		return nil, err
	}

	events := make([]adminevents.Event, len(rows))
	for i, row := range rows {
		events[i] = fromDB(row.AdminEvent, row.ActorName.String, row.SubjectName.String)
	}

	return events, nil
}

func fromDB(e db.AdminEvent, actorName, subjectName string) adminevents.Event {
	return adminevents.Event{
		ID:              e.ID,
		Kind:            adminevents.Kind(e.Kind),
		OccurredAt:      e.OccurredAt.Time,
		ActorMemberID:   uuidPtr(e.ActorMemberID),
		ActorName:       actorName,
		SubjectMemberID: uuidPtr(e.SubjectMemberID),
		SubjectName:     subjectName,
		SubjectLabel:    e.SubjectLabel.String,
		Detail:          e.Detail.String,
		Created:         e.Created.Time,
	}
}
