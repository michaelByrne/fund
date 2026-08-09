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

	KindEmailApproved        Kind = "email_approved"
	KindEmailApprovalRemoved Kind = "email_approval_removed"
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

	// SubjectMemberID is whose access changed, when that is somebody with an
	// account.
	//
	// Nil for an approved email address, whose subject cannot be a member yet --
	// approving them is what lets them become one. Exactly one of this and
	// SubjectLabel is set; Record refuses a record with neither, because an
	// access change with no subject describes nothing.
	SubjectMemberID *uuid.UUID

	// SubjectLabel names a subject that is not a member: the email address an
	// approval was granted for.
	//
	// Held in the audit table because an approval that does not say which address
	// answers nothing, and it is the same address already in approved_email in
	// the same database, read by the same admins. Deliberately kept out of the
	// log line -- see Service.Record -- because that goes somewhere else with
	// different retention.
	SubjectLabel string

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
	SubjectMemberID *uuid.UUID
	SubjectName     string
	SubjectLabel    string
	Detail          string
	Created         time.Time
}

// Subject is what to show for who this was about: a member's name, or the label
// for a subject who has no account.
func (e Event) Subject() string {
	if e.SubjectName != "" {
		return e.SubjectName
	}

	return e.SubjectLabel
}

// AboutAMember reports whether the subject is somebody with an account, and so
// whether there is a member page to link to.
func (e Event) AboutAMember() bool {
	return e.SubjectMemberID != nil
}

// Granted reports whether this event handed access out rather than taking it
// away.
func (e Event) Granted() bool {
	return e.Kind == KindAdminGranted || e.Kind == KindEmailApproved
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
	return e.ActorMemberID != nil &&
		e.SubjectMemberID != nil &&
		*e.ActorMemberID == *e.SubjectMemberID
}
