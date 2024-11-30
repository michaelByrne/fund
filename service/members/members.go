package members

type Member struct {
	ID                  int32
	MemberProviderEmail string
	BCOName             string
	IPAddress           string
	FirstName           string
	LastName            string
	ProviderPayerID     string
}

type InsertMember struct {
	MemberProviderEmail string
	BCOName             string
	IPAddress           string
}

type UpsertMember struct {
	MemberProviderEmail string
	BCOName             string
	IPAddress           string
	FirstName           string
	LastName            string
	ProviderPayerID     string
}
