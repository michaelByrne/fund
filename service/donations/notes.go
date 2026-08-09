package donations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// MaxNoteLength is how much a donor gets to say.
//
// Long enough for a real message, short enough that fifty of them still render on
// one page.
const MaxNoteLength = 500

// ErrNotADonor means the member has not given money to this fund, so has nothing
// to attach a note to.
var ErrNotADonor = errors.New("only donors to this fund can leave a note")

// ErrNoteTooLong means the message exceeds MaxNoteLength.
var ErrNoteTooLong = fmt.Errorf("a note can be at most %d characters", MaxNoteLength)

// ErrNoteEmpty means there is nothing to save.
var ErrNoteEmpty = errors.New("a note needs something in it")

// FundNote is a message a donor attached to a fund.
type FundNote struct {
	ID       uuid.UUID
	FundID   uuid.UUID
	MemberID uuid.UUID

	Body      string
	Anonymous bool

	// AuthorName is empty when the note is anonymous. Blanked here rather than in
	// the template so that no caller can render it by reaching past the check.
	AuthorName string

	Created time.Time
	Updated time.Time
}

// Edited reports whether the note has been changed since it was written, so the
// page can say so rather than showing a date that is not when it was said.
func (n FundNote) Edited() bool {
	return n.Updated.After(n.Created.Add(time.Second))
}

// SaveFundNote writes or replaces a donor's note on a fund.
//
// Eligibility is money that survived refunds, or a subscription still running.
// A full refund is not a donation. A monthly gift set up a minute ago is one,
// even though PayPal has not charged it yet -- and the thank-you screen, which is
// where we ask, comes before the first payment exists.
//
// Checked here rather than at the form: the member comes from the session, the
// fund from the request, and this is the last place that is true for both.
func (s DonationService) SaveFundNote(ctx context.Context, fundID, memberID uuid.UUID, body string, anonymous bool) (*FundNote, error) {
	trimmed := strings.TrimSpace(body)

	if trimmed == "" {
		return nil, ErrNoteEmpty
	}

	// Counted in runes. Counting bytes would cut a donor off early for writing in
	// a language that does not fit in one byte per character.
	if utf8.RuneCountInString(trimmed) > MaxNoteLength {
		return nil, ErrNoteTooLong
	}

	given, err := s.donationStore.MemberHasGivenToFund(ctx, fundID, memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to check whether member has given to fund",
			slog.String("error", err.Error()))

		return nil, err
	}

	if !given {
		return nil, ErrNotADonor
	}

	note, err := s.donationStore.UpsertFundNote(ctx, UpsertFundNote{
		FundID:    fundID,
		MemberID:  memberID,
		Body:      trimmed,
		Anonymous: anonymous,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to save fund note", slog.String("error", err.Error()))

		return nil, err
	}

	return note, nil
}

// MemberHasGivenToFund reports whether the member may leave a note.
//
// The page asks so it can offer the form only to somebody the server would accept
// one from, rather than showing a box and refusing what is typed into it.
func (s DonationService) MemberHasGivenToFund(ctx context.Context, fundID, memberID uuid.UUID) (bool, error) {
	given, err := s.donationStore.MemberHasGivenToFund(ctx, fundID, memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to check whether member has given to fund",
			slog.String("error", err.Error()))

		return false, err
	}

	return given, nil
}

// ListFundNotes is what a visitor sees on a fund.
func (s DonationService) ListFundNotes(ctx context.Context, fundID uuid.UUID) ([]FundNote, error) {
	notes, err := s.donationStore.GetFundNotes(ctx, fundID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to list fund notes", slog.String("error", err.Error()))

		return nil, err
	}

	return notes, nil
}

// GetFundNoteForMember is a donor's own note, so the form can show what they said
// last time rather than an empty box that silently overwrites it.
func (s DonationService) GetFundNoteForMember(ctx context.Context, fundID, memberID uuid.UUID) (*FundNote, error) {
	note, err := s.donationStore.GetFundNoteForMember(ctx, fundID, memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get fund note for member", slog.String("error", err.Error()))

		return nil, err
	}

	return note, nil
}

// RemoveFundNote takes a note down.
//
// Soft: the row stays, with who removed it and when. After taking something down
// that is exactly what you want to still have.
func (s DonationService) RemoveFundNote(ctx context.Context, noteID, actorID uuid.UUID) error {
	note, err := s.donationStore.RemoveFundNote(ctx, noteID, actorID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to remove fund note",
			slog.String("note_id", noteID.String()),
			slog.String("error", err.Error()),
		)

		return err
	}

	// An admin taking down a member's words is worth a line in the fund's
	// history. fund_note.removed_by already says who, but only until the next
	// removal of that note overwrites it.
	//
	// The note's text is not carried. The event says a note was removed and by
	// whom; what it said is still in the row, which is the point of the removal
	// being soft.
	s.recordNoteRemoval(ctx, note, &actorID)

	return nil
}

// recordNoteRemoval writes the event, or nothing when there was no note to take
// down.
//
// A second click on a note already removed changes no row, and recording it
// would put a removal in the feed for something that was already gone.
func (s DonationService) recordNoteRemoval(ctx context.Context, note *FundNote, actorID *uuid.UUID) {
	if note == nil {
		return
	}

	subject := note.MemberID

	s.events.Record(ctx, fundevents.Record{
		FundID:          note.FundID,
		Kind:            fundevents.KindFundNoteRemoved,
		ActorMemberID:   actorID,
		SubjectMemberID: &subject,
		ReferenceID:     &note.ID,
	})
}

// RemoveOwnFundNote is a donor taking their own words down.
//
// Separate from RemoveFundNote rather than sharing it with a permission check:
// this one cannot name a note at all, only a fund, so there is no id to get wrong
// and none to guess.
func (s DonationService) RemoveOwnFundNote(ctx context.Context, fundID, memberID uuid.UUID) error {
	note, err := s.donationStore.RemoveOwnFundNote(ctx, fundID, memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to remove own fund note",
			slog.String("fund_id", fundID.String()),
			slog.String("error", err.Error()),
		)

		return err
	}

	// The actor and the subject are the same person here, which the feed already
	// knows how to say without repeating the name twice.
	s.recordNoteRemoval(ctx, note, &memberID)

	return nil
}

// ListFundNotesForMember is every note this member has up, keyed by fund.
//
// The my-donations page draws one editor per donation, and each needs to know
// what is already there. A failure is not fatal to that page: the donations are
// what it is for, and an editor that opens empty is a smaller wrong than a page
// that will not load.
func (s DonationService) ListFundNotesForMember(ctx context.Context, memberID uuid.UUID) (map[uuid.UUID]FundNote, error) {
	notes, err := s.donationStore.GetFundNotesForMember(ctx, memberID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to list a member's fund notes", slog.String("error", err.Error()))

		return nil, err
	}

	return notes, nil
}
