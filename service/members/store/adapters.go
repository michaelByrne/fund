package store

import (
	"boardfund/db"
	"boardfund/service/members"
	"github.com/jackc/pgx/v5/pgtype"
	"net/netip"
)

func toDBMemberUpsertParams(member members.UpsertMember) db.UpsertMemberParams {
	params := db.UpsertMemberParams{
		PaypalEmail: member.MemberProviderEmail,
		IpAddress:   netip.MustParseAddr(member.IPAddress),
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

	return params
}

func fromDBMember(member db.Member) members.Member {
	return members.Member{
		ID:                  member.ID,
		MemberProviderEmail: member.PaypalEmail,
		BCOName:             member.BcoName.String,
		IPAddress:           member.IpAddress.String(),
		FirstName:           member.FirstName.String,
		LastName:            member.LastName.String,
		ProviderPayerID:     member.ProviderPayerID.String,
	}
}
