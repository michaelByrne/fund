package adminevents

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// DefaultLimit is how many events the audit page shows.
const DefaultLimit = 100

type eventStore interface {
	InsertAdminEvent(ctx context.Context, arg Record) (*Event, error)
	GetAdminEvents(ctx context.Context, limit int32) ([]Event, error)
}

type Service struct {
	store eventStore

	logger *slog.Logger
}

func NewService(store eventStore, logger *slog.Logger) *Service {
	return &Service{store: store, logger: logger}
}

// Record writes an event.
//
// Like fundevents.Record it returns nothing, for the same reason: the group
// write to Cognito has already happened by the time this runs, so an error here
// offers the caller no choice it can act on. Failing the request would report
// that the grant did not happen when it did, which is worse than a missing audit
// line -- and the missing line is logged at error level.
func (s Service) Record(ctx context.Context, record Record) {
	if record.SubjectMemberID == uuid.Nil {
		s.logger.ErrorContext(ctx, "refusing to record an admin event with no subject",
			slog.String("kind", string(record.Kind)),
		)

		return
	}

	_, err := s.store.InsertAdminEvent(ctx, record)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to record admin event",
			slog.String("error", err.Error()),
			slog.String("kind", string(record.Kind)),
			slog.String("subject_member_id", record.SubjectMemberID.String()),
		)

		return
	}

	// Same invariant as fundevents: recorded means logged. A privilege change is
	// the line most worth finding in a hurry, and the table it goes in is only
	// readable by the people whose access it describes.
	attrs := []any{
		slog.String("kind", string(record.Kind)),
		slog.String("subject_member_id", record.SubjectMemberID.String()),
	}

	if record.ActorMemberID != nil {
		attrs = append(attrs, slog.String("actor_member_id", record.ActorMemberID.String()))
	}

	if record.Detail != "" {
		attrs = append(attrs, slog.String("detail", record.Detail))
	}

	s.logger.InfoContext(ctx, "admin event", attrs...)
}

func (s Service) GetAdminEvents(ctx context.Context, limit int32) ([]Event, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	events, err := s.store.GetAdminEvents(ctx, limit)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get admin events", slog.String("error", err.Error()))

		return nil, err
	}

	return events, nil
}
