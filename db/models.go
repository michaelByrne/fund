// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0

package db

import (
	"database/sql/driver"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type IntervalUnit string

const (
	IntervalUnitWEEK  IntervalUnit = "WEEK"
	IntervalUnitMONTH IntervalUnit = "MONTH"
)

func (e *IntervalUnit) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = IntervalUnit(s)
	case string:
		*e = IntervalUnit(s)
	default:
		return fmt.Errorf("unsupported scan type for IntervalUnit: %T", src)
	}
	return nil
}

type NullIntervalUnit struct {
	IntervalUnit IntervalUnit
	Valid        bool // Valid is true if IntervalUnit is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullIntervalUnit) Scan(value interface{}) error {
	if value == nil {
		ns.IntervalUnit, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.IntervalUnit.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullIntervalUnit) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.IntervalUnit), nil
}

type PayoutFrequency string

const (
	PayoutFrequencyMonthly PayoutFrequency = "monthly"
	PayoutFrequencyOnce    PayoutFrequency = "once"
)

func (e *PayoutFrequency) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = PayoutFrequency(s)
	case string:
		*e = PayoutFrequency(s)
	default:
		return fmt.Errorf("unsupported scan type for PayoutFrequency: %T", src)
	}
	return nil
}

type NullPayoutFrequency struct {
	PayoutFrequency PayoutFrequency
	Valid           bool // Valid is true if PayoutFrequency is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullPayoutFrequency) Scan(value interface{}) error {
	if value == nil {
		ns.PayoutFrequency, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.PayoutFrequency.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullPayoutFrequency) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.PayoutFrequency), nil
}

type Role string

const (
	RoleADMIN Role = "ADMIN"
	RoleDONOR Role = "DONOR"
	RolePAYEE Role = "PAYEE"
)

func (e *Role) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = Role(s)
	case string:
		*e = Role(s)
	default:
		return fmt.Errorf("unsupported scan type for Role: %T", src)
	}
	return nil
}

type NullRole struct {
	Role  Role
	Valid bool // Valid is true if Role is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullRole) Scan(value interface{}) error {
	if value == nil {
		ns.Role, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.Role.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullRole) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.Role), nil
}

type Donation struct {
	ID                     uuid.UUID
	Recurring              bool
	DonorID                uuid.UUID
	DonationPlanID         uuid.NullUUID
	ProviderOrderID        string
	Created                pgtype.Timestamptz
	Updated                pgtype.Timestamptz
	FundID                 uuid.UUID
	Active                 bool
	ProviderSubscriptionID pgtype.Text
}

type DonationPayment struct {
	ID              uuid.UUID
	DonationID      uuid.UUID
	PaypalPaymentID string
	AmountCents     int32
	Created         pgtype.Timestamptz
	Updated         pgtype.Timestamptz
}

type DonationPlan struct {
	ID            uuid.UUID
	Name          string
	PaypalPlanID  pgtype.Text
	AmountCents   int32
	IntervalUnit  IntervalUnit
	IntervalCount int32
	Active        bool
	Created       pgtype.Timestamptz
	Updated       pgtype.Timestamptz
	FundID        uuid.UUID
}

type Fund struct {
	ID              uuid.UUID
	Name            string
	Description     string
	ProviderID      string
	ProviderName    string
	GoalCents       pgtype.Int4
	PayoutFrequency PayoutFrequency
	Active          bool
	Principal       uuid.NullUUID
	Expires         NullDBTime
	NextPayment     DBTime
	Created         pgtype.Timestamptz
	Updated         pgtype.Timestamptz
}

type Member struct {
	ID              uuid.UUID
	FirstName       pgtype.Text
	LastName        pgtype.Text
	BcoName         pgtype.Text
	Roles           []Role
	Email           string
	IpAddress       *netip.Addr
	LastLogin       NullDBTime
	CognitoID       pgtype.Text
	PaypalEmail     pgtype.Text
	PostalCode      pgtype.Text
	Created         pgtype.Timestamptz
	Updated         pgtype.Timestamptz
	ProviderPayerID pgtype.Text
	Active          bool
}

type Session struct {
	Token  string
	Data   []byte
	Expiry pgtype.Timestamptz
}
