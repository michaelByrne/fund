package stats

type FundStats struct {
	TotalDonated    int32
	TotalDonations  int32
	AverageDonation int32
	TotalDonors     int32
	Monthly         []MonthTotal
}

type MonthTotal struct {
	MonthYear  string `json:"month"`
	TotalCents int32  `json:"amount"`
}
