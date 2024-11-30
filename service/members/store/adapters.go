package store

import (
	"boardfund/db"
	"boardfund/service/members"
	"database/sql"
	"github.com/jackc/pgtype"
	"net"
)

func toDBMemberUpsertParams(member members.UpsertMember) db.UpsertMemberParams {
	ipAddress := net.ParseIP(member.IPAddress)
	netIP := net.IPNet{
		IP: ipAddress,
	}

	params := db.UpsertMemberParams{
		PaypalEmail: member.MemberProviderEmail,
		IpAddress: pgtype.Inet{
			IPNet:  &netIP,
			Status: pgtype.Present,
		},
	}

	if member.BCOName != "" {
		params.BcoName = sql.NullString{
			String: member.BCOName,
			Valid:  true,
		}
	}

	if member.FirstName != "" {
		params.FirstName = sql.NullString{
			String: member.FirstName,
			Valid:  true,
		}
	}

	if member.LastName != "" {
		params.LastName = sql.NullString{
			String: member.LastName,
			Valid:  true,
		}
	}

	if member.ProviderPayerID != "" {
		params.ProviderPayerID = sql.NullString{
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
		IPAddress:           member.IpAddress.IPNet.IP.String(),
		FirstName:           member.FirstName.String,
		LastName:            member.LastName.String,
		ProviderPayerID:     member.ProviderPayerID.String,
	}
}
