package store

import (
	"boardfund/db"
	"boardfund/service/donations"
	"boardfund/service/members"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
)

func toDBMemberUpsertParams(member members.UpsertMember) db.UpsertMemberParams {
	params := db.UpsertMemberParams{
		ID:    member.ID,
		Email: member.Email,
		Roles: convertRolesToDB(member.Roles),
	}

	if member.BCOName != "" {
		params.BcoName = pgtype.Text{
			String: member.BCOName,
			Valid:  true,
		}
	}

	if member.FirstName != "" {
		params.FirstName = pgtype.Text{
			String: member.FirstName,
			Valid:  true,
		}
	}

	if member.LastName != "" {
		params.LastName = pgtype.Text{
			String: member.LastName,
			Valid:  true,
		}
	}

	if member.ProviderPayerID != "" {
		params.ProviderPayerID = pgtype.Text{
			String: member.ProviderPayerID,
			Valid:  true,
		}
	}

	if member.CognitoID != "" {
		params.CognitoID = pgtype.Text{
			String: member.CognitoID,
			Valid:  true,
		}
	}

	return params
}

func convertRolesFromDB(roles []db.Role) []members.MemberRole {
	var convertedRoles []members.MemberRole
	for _, role := range roles {
		convertedRoles = append(convertedRoles, members.MemberRole(role))
	}

	return convertedRoles
}

func convertRolesToDB(roles []members.MemberRole) []db.Role {
	var convertedRoles []db.Role
	for _, role := range roles {
		convertedRoles = append(convertedRoles, db.Role(role))
	}

	return convertedRoles
}

func fromDBMember(member db.Member) members.Member {
	var ip string
	if member.IpAddress != nil {
		ip = member.IpAddress.String()
	}

	return members.Member{
		ID:              member.ID,
		Email:           member.Email,
		BCOName:         member.BcoName.String,
		IPAddress:       ip,
		FirstName:       member.FirstName.String,
		LastName:        member.LastName.String,
		ProviderPayerID: member.ProviderPayerID.String,
		CognitoID:       member.CognitoID.String,
		Roles:           convertRolesFromDB(member.Roles),
		Active:          member.Active,
		Created:         member.Created.Time,
		Updated:         member.Updated.Time,
	}
}

func fromDBMemberWithDonations(member db.GetMemberWithDonationsRow) (*members.Member, error) {
	memberOut := &members.Member{
		ID:              member.ID,
		Email:           member.Email,
		BCOName:         member.BcoName.String,
		CognitoID:       member.CognitoID.String,
		FirstName:       member.FirstName.String,
		LastName:        member.LastName.String,
		ProviderPayerID: member.ProviderPayerID.String,
		Active:          member.Active,
		Created:         member.Created.Time,
		Updated:         member.Updated.Time,
	}

	for _, role := range member.Roles {
		memberOut.Roles = append(memberOut.Roles, members.MemberRole(role))
	}

	donationsBytes, err := json.Marshal(member.Donations)
	if err != nil {
		return nil, err
	}

	var dbDonations []members.MemberDonation
	err = json.Unmarshal(donationsBytes, &dbDonations)
	if err != nil {
		return nil, err
	}

	for _, donation := range dbDonations {
		var paymentsOut []donations.DonationPayment
		for _, payment := range donation.Payments {
			paymentOut := donations.DonationPayment{
				ID:          payment.ID,
				DonationID:  payment.DonationID,
				AmountCents: payment.AmountCents,
				Created:     payment.Created.Time,
				Updated:     payment.Updated.Time,
			}

			paymentsOut = append(paymentsOut, paymentOut)
		}

		donationOut := donations.Donation{
			ID:                     donation.ID,
			DonorID:                donation.DonorID,
			FundID:                 donation.FundID,
			FundName:               donation.FundName,
			Recurring:              donation.Recurring,
			Created:                donation.Created.Time,
			Updated:                donation.Updated.Time,
			ProviderOrderID:        donation.ProviderOrderID,
			Payments:               paymentsOut,
			ProviderSubscriptionID: donation.ProviderSubscriptionID,
		}

		if donation.Plan != nil {
			donationOut.Plan = &donations.DonationPlan{
				ID:            donation.Plan.ID,
				AmountCents:   int32(donation.Plan.AmountCents),
				IntervalCount: donation.Plan.IntervalCount,
				IntervalUnit:  donations.IntervalUnit(donation.Plan.IntervalUnit),
				Created:       donation.Plan.Created.Time,
				Updated:       donation.Plan.Updated.Time,
			}
		}

		memberOut.Donations = append(memberOut.Donations, donationOut)
	}

	return memberOut, nil
}
