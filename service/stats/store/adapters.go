package store

import (
	"boardfund/db"
	"boardfund/service/stats"
)

func fromDBFundStatsRow(row db.GetFundStatsRow) stats.FundStats {
	return stats.FundStats{
		TotalDonated:    int32(row.TotalDonated),
		TotalDonations:  int32(row.TotalDonations),
		AverageDonation: row.AverageDonation,
		TotalDonors:     int32(row.TotalDonors),
	}
}
