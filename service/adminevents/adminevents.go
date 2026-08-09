package adminevents

import (
	"context"
	"log/slog"
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
	// Exactly one subject, matching the check constraint on the table. Neither
	// describes nothing; both would leave a reader to guess which one the event
	// was about.
	if (record.SubjectMemberID == nil) == (record.SubjectLabel == "") {
		s.logger.ErrorContext(ctx, "refusing to record an admin event without exactly one subject",
			slog.String("kind", string(record.Kind)),
		)

		return
	}

	// Built before the write, so the failure path can say the same things about
	// the event as the success path. An error line naming only the kind leaves
	// the one question worth asking -- whose access failed to be recorded --
	// answerable only by guessing.
	attrs := logAttrs(record)

	_, err := s.store.InsertAdminEvent(ctx, record)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to record admin event",
			append(attrs, slog.String("error", err.Error()))...)

		return
	}

	// Same invariant as fundevents: recorded means logged. A privilege change is
	// the line most worth finding in a hurry, and the table it goes in is only
	// readable by the people whose access it describes.
	s.logger.InfoContext(ctx, "admin event", attrs...)
}

// logAttrs describes an event for the operational log.
//
// SubjectLabel is deliberately absent, and only its presence is reported. It
// holds an email address, and the rule for this stream is ids only: the address
// lives in the audit table, where the people who can already read it in the
// admin section will find it, rather than in a log with different retention.
func logAttrs(record Record) []any {
	attrs := []any{slog.String("kind", string(record.Kind))}

	if record.SubjectMemberID != nil {
		attrs = append(attrs, slog.String("subject_member_id", record.SubjectMemberID.String()))
	}

	if record.SubjectLabel != "" {
		attrs = append(attrs, slog.Bool("subject_not_a_member", true))
	}

	if record.ActorMemberID != nil {
		attrs = append(attrs, slog.String("actor_member_id", record.ActorMemberID.String()))
	}

	if record.Detail != "" {
		attrs = append(attrs, slog.String("detail", record.Detail))
	}

	return attrs
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
