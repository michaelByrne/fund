package store

import (
	"boardfund/db"
	"boardfund/service/enrollments"
	"boardfund/service/members"
	"github.com/jackc/pgx/v5/pgtype"
)

func toDBEnrollmentParams(arg enrollments.InsertEnrollment) db.InsertEnrollmentParams {
	return db.InsertEnrollmentParams{
		ID:          arg.ID,
		MemberID:    arg.MemberID,
		FundID:      arg.FundID,
		PaypalEmail: arg.PaypalEmail,
		MemberBcoName: pgtype.Text{
			String: arg.MemberBCOName,
			Valid:  true,
		},
	}
}

func fromDBEnrollment(dbEnrollment db.FundEnrollment) enrollments.Enrollment {
	return enrollments.Enrollment{
		ID:              dbEnrollment.ID,
		MemberID:        dbEnrollment.MemberID,
		MemberBCOName:   dbEnrollment.MemberBcoName.String,
		FundID:          dbEnrollment.FundID,
		FirstPayoutDate: dbEnrollment.FirstPayoutDate.Time,
		Created:         dbEnrollment.Created.Time,
		Updated:         dbEnrollment.Updated.Time,
	}
}

func toDBUpdatePaypalEmailParams(arg enrollments.UpdatePaypalEmail) db.UpdatePaypalEmailParams {
	return db.UpdatePaypalEmailParams{
		ID:          arg.MemberID,
		PaypalEmail: pgtype.Text{String: arg.Email, Valid: true},
	}
}

func fromDBPayeeMember(dbMember db.Member) enrollments.PayeeMember {
	var roles []members.MemberRole
	for _, role := range dbMember.Roles {
		roles = append(roles, members.MemberRole(role))
	}

	return enrollments.PayeeMember{
		ID:              dbMember.ID,
		Email:           dbMember.Email,
		BCOName:         dbMember.BcoName.String,
		CognitoID:       dbMember.CognitoID.String,
		FirstName:       dbMember.FirstName.String,
		LastName:        dbMember.LastName.String,
		ProviderPayerID: dbMember.ProviderPayerID.String,
		PaypalEmail:     dbMember.PaypalEmail.String,
		Active:          dbMember.Active,
		Roles:           roles,
		Created:         dbMember.Created.Time,
		Updated:         dbMember.Updated.Time,
	}
}

func toDBGetEnrollmentForFundByMemberIDParams(arg enrollments.GetEnrollmentForFundByMemberID) db.GetEnrollmentForFundByMemberIdParams {
	return db.GetEnrollmentForFundByMemberIdParams{
		FundID:   arg.FundID,
		MemberID: arg.MemberID,
	}
}

func toDBFundEnrollmentExistsParams(arg enrollments.FundEnrollmentExists) db.FundEnrollmentExistsParams {
	return db.FundEnrollmentExistsParams{
		FundID:   arg.FundID,
		MemberID: arg.MemberID,
	}
}
