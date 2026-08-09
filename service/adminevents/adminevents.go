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
		s.logger.Error("refusing to record an admin event with no subject",
			slog.String("kind", string(record.Kind)),
		)

		return
	}

	_, err := s.store.InsertAdminEvent(ctx, record)
	if err != nil {
		s.logger.Error("failed to record admin event",
			slog.String("error", err.Error()),
			slog.String("kind", string(record.Kind)),
			slog.String("subject_member_id", record.SubjectMemberID.String()),
		)
	}
}

func (s Service) GetAdminEvents(ctx context.Context, limit int32) ([]Event, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	events, err := s.store.GetAdminEvents(ctx, limit)
	if err != nil {
		s.logger.Error("failed to get admin events", slog.String("error", err.Error()))

		return nil, err
	}

	return events, nil
}
