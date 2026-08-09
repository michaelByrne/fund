package fundevents

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// DefaultLimit is how many events the fund page shows.
const DefaultLimit = 50

type eventStore interface {
	InsertFundEvent(ctx context.Context, arg Record) (*Event, error)
	GetFundEvents(ctx context.Context, fundID uuid.UUID, limit int32) ([]Event, error)
}

type Service struct {
	store eventStore

	logger *slog.Logger
}

func NewService(store eventStore, logger *slog.Logger) *Service {
	return &Service{store: store, logger: logger}
}

// Record writes an event, and logs that it did.
//
// The log line happens here rather than at the nineteen call sites, which is
// what makes it an invariant instead of a habit: anything that records a fund
// event is, by construction, also visible to whoever is on call. Twelve of those
// sites had no log line at all -- a completed donation wrote a row and returned
// nil -- and a rule kept by remembering would have grown a thirteenth.
//

// It deliberately never returns an error to its caller, and callers are expected
// to ignore it as a failure path. Recording runs after the operation it
// describes has already committed, so returning an error would present the
// caller with a choice it cannot act on: the donation is taken, the batch is
// approved, and failing now would report a lie.
//
// The trade-off is that a failed insert loses an audit line. That is logged at
// error level, and the domain rows still carry their own timestamps, so a gap is
// detectable and reconstructible. Blocking a payment on the audit log being
// available would be the worse failure.
func (s Service) Record(ctx context.Context, record Record) {
	if record.FundID == uuid.Nil {
		s.logger.ErrorContext(ctx, "refusing to record a fund event with no fund",
			slog.String("kind", string(record.Kind)),
		)

		return
	}

	event, err := s.store.InsertFundEvent(ctx, record)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to record fund event",
			slog.String("error", err.Error()),
			slog.String("kind", string(record.Kind)),
			slog.String("fund_id", record.FundID.String()),
		)

		return
	}

	// No row means the dedupe key was already present, so this is a redelivered
	// webhook describing something already recorded. Logged at debug rather than
	// info: it is the ordinary outcome of at-least-once delivery, and writing
	// "payment received" a second time for one payment would misreport it as two.
	if event == nil {
		s.logger.DebugContext(ctx, "fund event already recorded",
			slog.String("kind", string(record.Kind)),
			slog.String("fund_id", record.FundID.String()),
			slog.String("dedupe_key", record.DedupeKey),
		)

		return
	}

	s.logger.InfoContext(ctx, "fund event", logAttrs(record)...)
}

// logAttrs describes an event for the operational log.
//
// Every field here is an id, an amount or a kind. The Record itself carries no
// names -- the actor and subject are uuids, resolved to names only by the query
// that renders a page -- so this cannot copy a member's details out of the
// database and into a stream with different retention, whatever is added to it
// later.
//
// Detail is included even though it is free text, and on a rejected batch it is
// whatever the treasurer typed. It goes to the same people who can already read
// it in the admin section, and dropping it would remove the most informative
// field on the line: "4 payees", "approval window expired", "goal $500.00 to
// $750.00".
func logAttrs(record Record) []any {
	attrs := []any{
		slog.String("kind", string(record.Kind)),
		slog.String("fund_id", record.FundID.String()),
	}

	if record.AmountCents != nil {
		attrs = append(attrs, slog.Int("amount_cents", int(*record.AmountCents)))
	}

	if record.ActorMemberID != nil {
		attrs = append(attrs, slog.String("actor_member_id", record.ActorMemberID.String()))
	}

	if record.SubjectMemberID != nil {
		attrs = append(attrs, slog.String("subject_member_id", record.SubjectMemberID.String()))
	}

	if record.ReferenceID != nil {
		attrs = append(attrs, slog.String("reference_id", record.ReferenceID.String()))
	}

	if record.Detail != "" {
		attrs = append(attrs, slog.String("detail", record.Detail))
	}

	return attrs
}

func (s Service) GetFundEvents(ctx context.Context, fundID uuid.UUID, limit int32) ([]Event, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	events, err := s.store.GetFundEvents(ctx, fundID, limit)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get fund events",
			slog.String("error", err.Error()),
			slog.String("fund_id", fundID.String()),
		)

		return nil, err
	}

	return events, nil
}

// GetPublicFundEvents is the same feed with the private half removed.
//
// The filtering happens here rather than in SQL so that Kind.Public is the only
// place the decision lives -- a query with its own list of kinds would be a
// second copy to keep in step, and the copy that drifts is the one that leaks.
//
// The limit applies to what is read, not to what survives, so a fund with many
// donations shows a short timeline rather than a slow one. Read one page deeper
// than asked for, because the private events are the numerous ones.
func (s Service) GetPublicFundEvents(ctx context.Context, fundID uuid.UUID, limit int32) ([]PublicEvent, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	events, err := s.GetFundEvents(ctx, fundID, limit*publicReadFactor)
	if err != nil {
		return nil, err
	}

	public := make([]PublicEvent, 0, len(events))
	for _, event := range events {
		if !event.Kind.Public() {
			continue
		}

		public = append(public, event.Publish())

		if int32(len(public)) == limit {
			break
		}
	}

	return public, nil
}

// publicReadFactor is how much wider to read than the caller asked for. A fund's
// feed is mostly per-donor events, so reading exactly `limit` rows would often
// yield a handful of public ones from a fund with a long history.
const publicReadFactor = 10
