package store

import (
	"boardfund/db"
	"boardfund/service/stats"
)

func fromDBFundStatsRow(row db.GetFundStatsRow) stats.FundStats {
	return stats.FundStats{
		TotalDonated:    row.TotalDonated,
		TotalDonations:  int32(row.TotalDonations),
		AverageDonation: row.AverageDonation,
		TotalDonors:     int32(row.TotalDonors),
	}
}

func fromDBMonthlyTotalsRow(row db.GetMonthlyTotalsByFundRow) stats.MonthTotal {
	return stats.MonthTotal{
		MonthYear:  row.MonthYear,
		TotalCents: int32(row.Total),
	}
}
