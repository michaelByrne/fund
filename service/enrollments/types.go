package enrollments

import (
	"boardfund/service/donations"
	"boardfund/service/members"
	"github.com/google/uuid"
	"time"
)

type InsertEnrollment struct {
	ID            uuid.UUID
	MemberID      uuid.UUID
	MemberBCOName string
	FundID        uuid.UUID
}

type Enrollment struct {
	ID              uuid.UUID
	MemberID        uuid.UUID
	MemberBCOName   string
	FundID          uuid.UUID
	FirstPayoutDate time.Time
	Created         time.Time
	Updated         time.Time
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
