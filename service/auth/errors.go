package auth

import "errors"

type AuthError struct {
	Err error
}

func (e AuthError) Error() string {
	return e.Err.Error()
}

// ErrEmailNotApproved means registration was attempted with an address no admin
// has added to approved_email. A sentinel so callers can distinguish "we turned
// this away" from "the lookup failed" -- previously both surfaced as a 500, which
// made a routine rejection look like an outage in the logs and to the user.
var ErrEmailNotApproved = errors.New("email not approved")

// ErrEmailAlreadyApproved means the address is already on the approved list.
var ErrEmailAlreadyApproved = errors.New("email is already approved")
