package enrollments

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"github.com/google/uuid"
	"time"
)

type InsertEnrollment struct {
	ID              uuid.UUID
	MemberID        uuid.UUID
	MemberBCOName   string
	FundID          uuid.UUID
	PaypalEmail     string
	FirstPayoutDate time.Time
}

type Enrollment struct {
	ID            uuid.UUID
	MemberID      uuid.UUID
	MemberBCOName string
	FundID        uuid.UUID

	// PaypalEmail is where this person's payouts are sent. It was on the
	// database row but dropped by the adapter, so nothing could show or check it
	// -- which is unfortunate for the one field most likely to be wrong.
	PaypalEmail string

	// FirstPayoutDate gates inclusion in a batch: GetActiveEnrollmentsForPayout
	// requires it to have passed, so an enrollee dated in the future is skipped
	// without anything saying so.
	FirstPayoutDate time.Time

	Created time.Time
	Updated time.Time
}

// Payable reports whether this enrollment would be included in a batch planned
// now. PlanBatch skips an enrollee with no PayPal address, and the eligibility
// query excludes one whose first payout date has not arrived -- in both cases
// silently, which is why the list says so instead.
func (e Enrollment) Payable(now time.Time) bool {
	return e.PaypalEmail != "" && !e.FirstPayoutDate.After(now)
}

// Eligible reports whether the first payout date has arrived.
func (e Enrollment) Eligible(now time.Time) bool {
	return !e.FirstPayoutDate.After(now)
}

type CreateEnrollment struct {
	MemberID      uuid.UUID
	FundID        uuid.UUID
	PaypalEmail   string
	MemberBCOName string
}

type PayeeMember struct {
	ID              uuid.UUID
	Email           string `json:"email"`
	BCOName         string `json:"bco_name"`
	IPAddress       string
	CognitoID       string
	FirstName       string               `json:"first_name"`
	LastName        string               `json:"last_name"`
	ProviderPayerID string               `json:"provider_payer_id"`
	PaypalEmail     string               `json:"paypal_email"`
	Roles           []members.MemberRole `json:"role"`
	Active          bool                 `json:"active"`
	Created         time.Time            `json:"created"`
	Updated         time.Time            `json:"updated"`
	Donations       []donations.Donation `json:"donations"`
}

type UpdatePaypalEmail struct {
	MemberID uuid.UUID
	Email    string
}

type GetEnrollmentForFundByMemberID struct {
	FundID   uuid.UUID
	MemberID uuid.UUID
}

type FundEnrollmentExists struct {
	FundID   uuid.UUID
	MemberID uuid.UUID
}

// Recipient is an enrollee as a donor sees them: a name, and nothing else.
//
// A separate type rather than a filtered Enrollment, for the same reason
// fundevents.PublicEvent is one. Enrollment carries PaypalEmail -- the address
// this person's money goes to -- and no template on a page donors read should be
// able to reach it, whether by mistake or by a later edit that looks harmless.
// There is no field here to reach.
type Recipient struct {
	Name string
}

// Recipients projects enrollments for a donor-facing page.
func Recipients(enrollments []Enrollment) []Recipient {
	out := make([]Recipient, 0, len(enrollments))

	for _, enrollment := range enrollments {
		// A member with no bco_name has nothing to show, and a blank row reads as
		// a rendering fault rather than as somebody unnamed.
		if enrollment.MemberBCOName == "" {
			continue
		}

		out = append(out, Recipient{Name: enrollment.MemberBCOName})
	}

	return out
}
