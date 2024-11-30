package donations

import "time"

type IntervalUnit string

const (
	IntervalUnitWeek  IntervalUnit = "WEEK"
	IntervalUnitMonth IntervalUnit = "MONTH"
)

type Donation struct {
	ID             int32
	DonorID        int32
	DonationPlanID int32
	ProviderID     string
	Payment        *DonationPayment
	Created        time.Time
	Updated        time.Time
}

type DonationPayment struct {
	ID                  int32
	DonationID          int32
	ProviderPaymentID   string
	AmountCents         int32
	MemberProviderEmail string
	Created             time.Time
	Updated             time.Time
}

type DonationOrderCapture struct {
	ProviderOrderID     string
	PlanID              int32
	MemberProviderEmail string
	ProviderPaymentID   string
	AmountCents         int32
}

type CreateCapture struct {
	ProviderOrderID    string
	PlanID             int32
	ProviderPlanID     string
	ProviderDonationID string
	IPAddress          string
	PayerEmail         string
	AmountCents        int32
	PayerFirstName     string
	PayerLastName      string
	PayerID            string
	BCOName            string
}

type DonationPlan struct {
	ID             int32
	Name           string
	ProviderPlanID string
	AmountCents    int32
	IntervalUnit   IntervalUnit
	IntervalCount  int32
	Active         bool
	Created        time.Time
	Updated        time.Time
}

type CreatePlan struct {
	Name          string
	ProductID     string
	IntervalUnit  IntervalUnit
	IntervalCount int32
	AmountCents   int32
}

type InsertDonationPlan struct {
	Name           string
	AmountCents    int32
	IntervalUnit   string
	IntervalCount  int32
	ProviderPlanID string
	Active         bool
}

type UpdateDonationPlan struct {
	ID            int32
	Name          string
	AmountCents   int32
	IntervalUnit  string
	IntervalCount int32
	Active        bool
}

type InsertDonation struct {
	DonorID        int32
	DonationPlanID int32
}

type UpdateDonation struct {
	ID             int32
	DonorID        int32
	DonationPlanID int32
}

type InsertDonationPayment struct {
	DonationID        int32
	ProviderPaymentID string
	AmountCents       int32
}
