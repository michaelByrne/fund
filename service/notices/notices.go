package notices

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type noticeStore interface {
	InsertNotice(ctx context.Context, body string, actorID *uuid.UUID) (*Notice, error)
	GetActiveNotices(ctx context.Context) ([]Notice, error)
	GetNotices(ctx context.Context) ([]Notice, error)
	SetNoticeActive(ctx context.Context, id uuid.UUID, active bool, actorID *uuid.UUID) (*Notice, error)
}

type Service struct {
	store noticeStore

	logger *slog.Logger
}

func NewService(store noticeStore, logger *slog.Logger) *Service {
	return &Service{store: store, logger: logger}
}

// Create puts a notice up.
//
// Trimmed and bounded here as well as by the column, so the caller gets an error
// it can show rather than a constraint violation it cannot. Counted in runes:
// char_length on the column counts characters too, and counting bytes here would
// refuse a message the database would have accepted.
func (s Service) Create(ctx context.Context, body string, actorID *uuid.UUID) (*Notice, error) {
	trimmed := strings.TrimSpace(body)

	if trimmed == "" {
		return nil, ErrEmptyBody
	}

	if utf8.RuneCountInString(trimmed) > MaxBodyLength {
		return nil, ErrBodyTooLong
	}

	notice, err := s.store.InsertNotice(ctx, trimmed, actorID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to create a notice", slog.String("error", err.Error()))

		return nil, err
	}

	// Every member sees this on their next page load, so it is worth a line
	// saying who put it there. The body is not logged: it is a message to
	// members, not an operational detail, and it is one query away.
	s.logger.InfoContext(ctx, "notice created",
		slog.String("notice_id", notice.ID.String()),
		slog.Int("body_length", utf8.RuneCountInString(trimmed)),
	)

	return notice, nil
}

// Active is what the home page shows.
func (s Service) Active(ctx context.Context) ([]Notice, error) {
	found, err := s.store.GetActiveNotices(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to read active notices", slog.String("error", err.Error()))

		return nil, err
	}

	return found, nil
}

// All is what the admin panel shows, so a notice that has come down can be found
// and put back up.
func (s Service) All(ctx context.Context) ([]Notice, error) {
	found, err := s.store.GetNotices(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to read notices", slog.String("error", err.Error()))

		return nil, err
	}

	return found, nil
}

// SetActive puts a notice up or takes it down.
//
// Takes the state wanted rather than flipping what is there: two admins clicking
// at once both get what they asked for, instead of each undoing the other.
func (s Service) SetActive(ctx context.Context, id uuid.UUID, active bool, actorID *uuid.UUID) (*Notice, error) {
	notice, err := s.store.SetNoticeActive(ctx, id, active, actorID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to change a notice",
			slog.String("error", err.Error()),
			slog.String("notice_id", id.String()),
		)

		return nil, err
	}

	s.logger.InfoContext(ctx, "notice visibility changed",
		slog.String("notice_id", id.String()),
		slog.Bool("active", active),
	)

	return notice, nil
}
