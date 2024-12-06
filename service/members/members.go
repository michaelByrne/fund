package members

import "github.com/google/uuid"

type Member struct {
	ID                  uuid.UUID
	MemberProviderEmail string
	BCOName             string
	IPAddress           string
	FirstName           string
	LastName            string
	ProviderPayerID     string
}

type InsertMember struct {
	ID                  uuid.UUID
	MemberProviderEmail string
	BCOName             string
	IPAddress           string
}

type UpsertMember struct {
	ID                  uuid.UUID
	MemberProviderEmail string
	BCOName             string
	IPAddress           string
	FirstName           string
	LastName            string
	ProviderPayerID     string
}
