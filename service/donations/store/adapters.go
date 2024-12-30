package store

import (
	"boardfund/db"
	"boardfund/service/donations"
	"github.com/jackc/pgx/v5/pgtype"
)

func fromDBFundRow(fund db.GetActiveFundsRow) donations.Fund {
	fundOut := donations.Fund{
		ID:              fund.ID,
		Name:            fund.Name,
		Description:     fund.Description,
		ProviderName:    fund.ProviderName,
		ProviderID:      fund.ProviderID,
		Active:          fund.Active,
		PayoutFrequency: donations.PayoutFrequency(fund.PayoutFrequency),
		NextPayment:     fund.NextPayment.Time,
		GoalCents:       fund.GoalCents.Int32,
		Created:         fund.Created.Time,
		Updated:         fund.Updated.Time,
		Principal:       fund.Principal,
		Stats: donations.FundStats{
			TotalDonated:    fund.TotalDonated.Int32,
			TotalDonations:  int32(fund.TotalDonations.Int64),
			AverageDonation: fund.AverageDonation.Int32,
			TotalDonors:     int32(fund.TotalDonors.Int64),
		},
	}

	if !fund.Expires.Time.IsZero() {
		fundOut.Expires = &fund.Expires.Time
	}

	return fundOut
}

func fromDBFundByID(fund db.GetFundByIdRow) donations.Fund {
	fundOut := donations.Fund{
		ID:              fund.ID,
		Name:            fund.Name,
		Description:     fund.Description,
		ProviderName:    fund.ProviderName,
		ProviderID:      fund.ProviderID,
		Active:          fund.Active,
		PayoutFrequency: donations.PayoutFrequency(fund.PayoutFrequency),
		NextPayment:     fund.NextPayment.Time,
		GoalCents:       fund.GoalCents.Int32,
		Created:         fund.Created.Time,
		Updated:         fund.Updated.Time,
		Principal:       fund.Principal,
		Stats: donations.FundStats{
			TotalDonated:    fund.TotalDonated.Int32,
			TotalDonations:  int32(fund.TotalDonations.Int64),
			AverageDonation: fund.AverageDonation.Int32,
			TotalDonors:     int32(fund.TotalDonors.Int64),
		},
	}

	if !fund.Expires.Time.IsZero() {
		fundOut.Expires = &fund.Expires.Time
	}

	return fundOut
}

func fromDBFund(fund db.Fund) donations.Fund {
	fundOut := donations.Fund{
		ID:              fund.ID,
		Name:            fund.Name,
		Description:     fund.Description,
		ProviderName:    fund.ProviderName,
		ProviderID:      fund.ProviderID,
		Active:          fund.Active,
		PayoutFrequency: donations.PayoutFrequency(fund.PayoutFrequency),
		NextPayment:     fund.NextPayment.Time,
		GoalCents:       fund.GoalCents.Int32,
		Created:         fund.Created.Time,
		Updated:         fund.Updated.Time,
		Principal:       fund.Principal,
	}

	if !fund.Expires.Time.IsZero() {
		fundOut.Expires = &fund.Expires.Time
	}

	return fundOut
}

func fromDBMonthlyDonationTotal(total db.GetMonthlyTotalsByFundRow) donations.MonthTotal {
	return donations.MonthTotal{
		MonthYear:    total.MonthYear,
		TotalCents:   int32(total.Total),
		UniqueDonors: int32(total.UniqueDonors),
	}
}

func toDBFundInsertParams(fund donations.InsertFund) db.InsertFundParams {
	insertFund := db.InsertFundParams{
		ID:           fund.ID,
		Name:         fund.Name,
		Description:  fund.Description,
		Active:       fund.Active,
		ProviderID:   fund.ProviderID,
		ProviderName: fund.ProviderName,
		GoalCents: pgtype.Int4{
			Int32: fund.GoalCents,
			Valid: true,
		},
		PayoutFrequency: db.PayoutFrequency(fund.PayoutFrequency),
		Principal:       fund.Principal,
	}

	if fund.Expires != nil {
		insertFund.Expires = db.NullDBTime{
			DBTime: db.DBTime{
				Time: *fund.Expires,
			},
			Valid: true,
		}
	}

	return insertFund
}

func toDBFundUpdateParams(fund donations.UpdateFund) db.UpdateFundParams {
	updateFund := db.UpdateFundParams{
		ID:          fund.ID,
		Name:        fund.Name,
		Description: fund.Description,
		Active:      fund.Active,
		GoalCents: pgtype.Int4{
			Int32: fund.GoalCents,
			Valid: true,
		},
		PayoutFrequency: db.PayoutFrequency(fund.PayoutFrequency),
		Principal:       fund.Principal,
	}

	if fund.Expires != nil {
		updateFund.Expires = db.NullDBTime{
			DBTime: db.DBTime{
				Time: *fund.Expires,
			},
			Valid: true,
		}
	}

	return updateFund
}

func fromDBDonationPlan(plan db.DonationPlan) donations.DonationPlan {
	return donations.DonationPlan{
		ID:             plan.ID,
		Name:           plan.Name,
		ProviderPlanID: plan.PaypalPlanID.String,
		AmountCents:    plan.AmountCents,
		IntervalUnit:   donations.IntervalUnit(plan.IntervalUnit),
		IntervalCount:  plan.IntervalCount,
		Active:         plan.Active,
		FundID:         plan.FundID,
		Created:        plan.Created.Time,
		Updated:        plan.Updated.Time,
	}
}

func toDBDonationPlanUpsertParams(plan donations.UpsertDonationPlan) db.UpsertDonationPlanParams {
	return db.UpsertDonationPlanParams{
		PaypalPlanID: pgtype.Text{
			String: plan.ProviderPlanID,
			Valid:  true,
		},
		ID:            plan.ID,
		FundID:        plan.FundID,
		Name:          plan.Name,
		AmountCents:   plan.AmountCents,
		IntervalUnit:  db.IntervalUnit(plan.IntervalUnit),
		IntervalCount: plan.IntervalCount,
		Active:        plan.Active,
	}
}

func fromDBDonation(donation db.Donation) donations.Donation {
	donationOut := donations.Donation{
		ID:                     donation.ID,
		DonorID:                donation.DonorID,
		DonationPlanID:         donation.DonationPlanID,
		FundID:                 donation.FundID,
		Recurring:              donation.Recurring,
		Active:                 donation.Active,
		Created:                donation.Created.Time,
		Updated:                donation.Updated.Time,
		ProviderOrderID:        donation.ProviderOrderID,
		ProviderSubscriptionID: donation.ProviderSubscriptionID.String,
	}

	return donationOut
}

func toDBDonationInsertParams(donation donations.InsertDonation) db.InsertDonationParams {
	insertDonation := db.InsertDonationParams{
		ID:              donation.ID,
		DonorID:         donation.DonorID,
		Recurring:       donation.Recurring,
		FundID:          donation.FundID,
		ProviderOrderID: donation.ProviderOrderID,
		DonationPlanID:  donation.PlanID,
	}

	if donation.ProviderSubscriptionID == "" {
		insertDonation.ProviderSubscriptionID = pgtype.Text{
			Valid: false,
		}
	} else {
		insertDonation.ProviderSubscriptionID = pgtype.Text{
			String: donation.ProviderSubscriptionID,
			Valid:  true,
		}
	}

	return insertDonation
}

func fromDBDonationPayment(payment db.DonationPayment) donations.DonationPayment {
	return donations.DonationPayment{
		ID:                payment.ID,
		DonationID:        payment.DonationID,
		ProviderPaymentID: payment.PaypalPaymentID,
		AmountCents:       payment.AmountCents,
		ProviderFeeCents:  payment.ProviderFeeCents,
		Created:           payment.Created.Time,
		Updated:           payment.Updated.Time,
	}
}

func toDBDonationPaymentInsertParams(payment donations.InsertDonationPayment) db.InsertDonationPaymentParams {
	return db.InsertDonationPaymentParams{
		ID:               payment.ID,
		DonationID:       payment.DonationID,
		PaypalPaymentID:  payment.ProviderPaymentID,
		AmountCents:      payment.AmountCents,
		ProviderFeeCents: payment.ProviderFeeCents,
	}
}

func toDBSetDonationToInactiveBySubscriptionIDParams(arg donations.DeactivateDonationBySubscription) db.SetDonationToInactiveBySubscriptionIdParams {
	return db.SetDonationToInactiveBySubscriptionIdParams{
		ProviderSubscriptionID: pgtype.Text{
			String: arg.SubscriptionID,
			Valid:  true,
		},
		InactiveReason: pgtype.Text{
			String: arg.Reason,
			Valid:  true,
		},
	}
}

func toDBSetDonationToInactive(arg donations.DeactivateDonation) db.SetDonationToInactiveParams {
	return db.SetDonationToInactiveParams{
		ID: arg.ID,
		InactiveReason: pgtype.Text{
			String: arg.Reason,
			Valid:  true,
		},
	}
}

func toDBGetRecurringDonationsForFundParams(arg donations.GetRecurringDonationsForFundRequest) db.GetRecurringDonationsForFundParams {
	return db.GetRecurringDonationsForFundParams{
		ID:     arg.FundID,
		Active: arg.Active,
	}
}

func toDBGetOneTimeDonationsForFundParams(arg donations.GetOneTimeDonationsForFundRequest) db.GetOneTimeDonationsForFundParams {
	return db.GetOneTimeDonationsForFundParams{
		ID:     arg.FundID,
		Active: arg.Active,
	}
}

func toDBUpdatePaymentPaypalFeeParams(arg donations.UpdatePaymentPaypalFee) db.UpdateDonationPaymentPaypalFeeParams {
	return db.UpdateDonationPaymentPaypalFeeParams{
		ID:               arg.ID,
		ProviderFeeCents: arg.ProviderFeeCents,
	}
}
