// Package notices is short administrative messages shown to every member.
//
// Distinct from fund notes, which are text a donor wrote about one fund. These
// go the other way: an admin writing to everyone, attached to no fund. The two
// share a word in English and nothing else.
package notices

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// MaxBodyLength matches the check constraint on the column. Long enough for a
// real message, short enough that several still leave room for the funds
// underneath.
const MaxBodyLength = 500

var (
	// ErrEmptyBody means there was nothing to say. Its own error because an
	// accidental submit is the likeliest way to reach it, and "that request was
	// not valid" is a poor answer to it.
	ErrEmptyBody = errors.New("a notice needs something in it")

	// ErrBodyTooLong is the other end of the same check.
	ErrBodyTooLong = errors.New("that notice is too long")
)

// Notice is one message.
type Notice struct {
	ID   uuid.UUID
	Body string

	// Active is what the home page filters on.
	Active bool

	// CreatedBy is who put it up, UpdatedBy who last changed it. Nil when
	// nobody the application knows about did -- see the column comments.
	CreatedBy *uuid.UUID
	UpdatedBy *uuid.UUID

	Created time.Time
	Updated time.Time
}
