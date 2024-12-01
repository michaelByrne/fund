package store

import (
	"boardfund/db"
	"boardfund/service/donations"
	"github.com/jackc/pgx/v5/pgtype"
)

func toDBDonationPlanInsertParams(plan donations.InsertDonationPlan) db.InsertDonationPlanParams {
	planParams := db.InsertDonationPlanParams{
		Name:          plan.Name,
		AmountCents:   plan.AmountCents,
		IntervalUnit:  db.IntervalUnit(plan.IntervalUnit),
		IntervalCount: plan.IntervalCount,
		Active:        plan.Active,
	}

	if plan.ProviderPlanID != "" {
		planParams.PaypalPlanID = pgtype.Text{
			String: plan.ProviderPlanID,
			Valid:  true,
		}
	}

	return planParams
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
		Created:        plan.Created.Time,
		Updated:        plan.Updated.Time,
	}
}

func toDBDonationPlanUpdateParams(plan donations.UpdateDonationPlan) db.UpdateDonationPlanParams {
	return db.UpdateDonationPlanParams{
		ID:            plan.ID,
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
		Created:        donation.Created.Time,
		Updated:        donation.Updated.Time,
	}
}

func toDBDonationInsertParams(donation donations.InsertDonation) db.InsertDonationParams {
	return db.InsertDonationParams{
		DonorID:        donation.DonorID,
		DonationPlanID: donation.DonationPlanID,
	}
}

func toDBDonationUpdateParams(donation donations.UpdateDonation) db.UpdateDonationParams {
	return db.UpdateDonationParams{
		ID:             donation.ID,
		DonorID:        donation.DonorID,
		DonationPlanID: donation.DonationPlanID,
	}
}

func fromDBDonationRow(donation db.GetDonationsByMemberPaypalEmailRow) donations.Donation {
	return donations.Donation{
		ID:             donation.ID,
		DonorID:        donation.DonorID,
		DonationPlanID: donation.DonationPlanID,
		Created:        donation.Created.Time,
		Updated:        donation.Updated.Time,
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
		DonationID:      payment.DonationID,
		PaypalPaymentID: payment.ProviderPaymentID,
		AmountCents:     payment.AmountCents,
	}
}
