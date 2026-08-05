package store

import (
	"boardfund/db"
	"boardfund/service/auth"
)

func fromDBApprovedEmail(email db.ApprovedEmail) auth.ApprovedEmail {
	return auth.ApprovedEmail{
		Email:   email.Email,
		Used:    email.Used,
		Created: email.Created.Time,
		UsedAt:  email.UsedAt.Time,
	}
}
