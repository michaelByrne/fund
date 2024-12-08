package store

import (
	"boardfund/db"
	"boardfund/service/members"
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
	}
}
