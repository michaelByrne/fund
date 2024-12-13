package members

import (
	"boardfund/db"
	"boardfund/service/donations"
	"github.com/google/uuid"
	"time"
)

type MemberRole string

const (
	AdminRole MemberRole = "ADMIN"
	DonorRole MemberRole = "DONOR"
	PayeeRole MemberRole = "PAYEE"
)

type MemberDonation struct {
	ID                     uuid.UUID           `json:"id"`
	DonorID                uuid.UUID           `json:"donor_id"`
	DonationPlanID         uuid.NullUUID       `json:"donation_plan_id"`
	FundID                 uuid.UUID           `json:"fund_id"`
	FundName               string              `json:"fund_name"`
	Recurring              bool                `json:"recurring"`
	ProviderOrderID        string              `json:"provider_order_id"`
	ProviderSubscriptionID string              `json:"provider_subscription_id"`
	Payments               []MemberPayment     `json:"payments"`
	Created                db.DBTime           `json:"created"`
	Updated                db.NullDBTime       `json:"updated"`
	Plan                   *MemberDonationPlan `json:"plan"`
}

type MemberDonationPlan struct {
	ID            uuid.UUID     `json:"id"`
	AmountCents   int64         `json:"amount_cents"`
	IntervalCount int32         `json:"interval_count"`
	IntervalUnit  string        `json:"interval_unit"`
	Created       db.DBTime     `json:"created"`
	Updated       db.NullDBTime `json:"updated"`
}

type MemberPayment struct {
	ID          uuid.UUID     `json:"id"`
	DonationID  uuid.UUID     `json:"donation_id"`
	AmountCents int32         `json:"amount_cents"`
	Created     db.DBTime     `json:"created"`
	Updated     db.NullDBTime `json:"updated"`
}

type Member struct {
	ID              uuid.UUID
	Email           string `json:"email"`
	BCOName         string `json:"bco_name"`
	IPAddress       string
	CognitoID       string
	FirstName       string               `json:"first_name"`
	LastName        string               `json:"last_name"`
	ProviderPayerID string               `json:"provider_payer_id"`
	Roles           []MemberRole         `json:"role"`
	Active          bool                 `json:"active"`
	Created         time.Time            `json:"created"`
	Updated         time.Time            `json:"updated"`
	Donations       []donations.Donation `json:"donations"`
}

func (m Member) GetTotalDonatedCents() int32 {
	var total int32
	for _, donation := range m.Donations {
		for _, payment := range donation.Payments {
			total += payment.AmountCents
		}
	}

	return total
}

func (m Member) IsAdmin() bool {
	for _, role := range m.Roles {
		if role == AdminRole {
			return true
		}
	}

	return false
}

type UpsertMember struct {
	ID              uuid.UUID
	Email           string
	BCOName         string
	IPAddress       string
	CognitoID       string
	FirstName       string
	LastName        string
	ProviderPayerID string
	Roles           []MemberRole
}

type CreateMember struct {
	Email     string
	BCOName   string
	FirstName string
	LastName  string
}
