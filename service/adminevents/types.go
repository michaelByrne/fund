// Package adminevents records privilege changes, append-only.
//
// It is the companion to fundevents for things that are not about a fund. Admin
// access is granted by Cognito group membership and by nothing else, which makes
// the group the entire authorisation model -- and a model with no record of who
// changed it cannot answer the one question asked after an incident.
package adminevents

import (
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindAdminGranted Kind = "admin_granted"
	KindAdminRevoked Kind = "admin_revoked"
)

// Record is one privilege change.
type Record struct {
	Kind Kind

	// OccurredAt is when it happened. Zero means now, which is the ordinary case
	// here: unlike a fund event, nothing reports these to us after the fact.
	OccurredAt time.Time

	// ActorMemberID is who made the change. Nil when it was made outside the app
	// -- the first admin, or a future command-line tool. Left nil rather than
	// filled with a stand-in so that "we do not know" stays distinguishable.
	ActorMemberID *uuid.UUID

	// SubjectMemberID is whose privileges changed. Always set: the store rejects
	// a record without one, because a privilege change with no subject describes
	// nothing.
	SubjectMemberID uuid.UUID

	// Detail is free text for the reader, such as how the change was made.
	Detail string
}

// Event is a recorded Record, with the names resolved.
type Event struct {
	ID              uuid.UUID
	Kind            Kind
	OccurredAt      time.Time
	ActorMemberID   *uuid.UUID
	ActorName       string
	SubjectMemberID uuid.UUID
	SubjectName     string
	Detail          string
	Created         time.Time
}

// Granted reports whether this event added admin access rather than removing it.
func (e Event) Granted() bool {
	return e.Kind == KindAdminGranted
}

// ByProvider reports whether this happened without a person the app knows about
// -- a change made in the Cognito console, or before this table existed. Said
// explicitly, because "nobody is listed" and "nobody did it" are different
// claims.
func (e Event) ByProvider() bool {
	return e.ActorMemberID == nil
}

// SelfInflicted reports whether the actor changed their own access. Worth
// showing: it is the shape of both a bootstrap and a privilege escalation.
func (e Event) SelfInflicted() bool {
	return e.ActorMemberID != nil && *e.ActorMemberID == e.SubjectMemberID
}
