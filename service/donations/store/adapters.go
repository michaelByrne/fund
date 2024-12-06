package store

import (
	"boardfund/db"
	"boardfund/service/donations"
	"github.com/jackc/pgx/v5/pgtype"
)

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
	}

	if !fund.Expires.Time.IsZero() {
		fundOut.Expires = &fund.Expires.Time
	}

	return fundOut
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
	}

	if fund.Expires != nil {
		insertFund.Expires = pgtype.Timestamptz{
			Time:  *fund.Expires,
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
	}

	if fund.Expires != nil {
		updateFund.Expires = pgtype.Timestamptz{
			Time:  *fund.Expires,
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
	return donations.Donation{
		ID:             donation.ID,
		DonorID:        donation.DonorID,
		DonationPlanID: donation.DonationPlanID,
		FundID:         donation.FundID,
		Recurring:      donation.Recurring,
		Created:        donation.Created.Time,
		Updated:        donation.Updated.Time,
	}
}

func toDBDonationInsertParams(donation donations.InsertDonation) db.InsertDonationParams {
	return db.InsertDonationParams{
		ID:        donation.ID,
		DonorID:   donation.DonorID,
		Recurring: false,
		FundID:    donation.FundID,
	}
}

func fromDBDonationPayment(payment db.DonationPayment) donations.DonationPayment {
	return donations.DonationPayment{
		ID:                payment.ID,
		DonationID:        payment.DonationID,
		ProviderPaymentID: payment.PaypalPaymentID,
		AmountCents:       payment.AmountCents,
		Created:           payment.Created.Time,
		Updated:           payment.Updated.Time,
	}
}

func toDBDonationPaymentInsertParams(payment donations.InsertDonationPayment) db.InsertDonationPaymentParams {
	return db.InsertDonationPaymentParams{
		ID:              payment.ID,
		DonationID:      payment.DonationID,
		PaypalPaymentID: payment.ProviderPaymentID,
		AmountCents:     payment.AmountCents,
	}
}
