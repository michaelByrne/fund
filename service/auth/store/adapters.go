package store

import (
	"boardfund/db"
	"boardfund/service/auth"
	"encoding/json"
	"github.com/go-webauthn/webauthn/webauthn"
)

func toDBInsertPasskeyUserParams(user auth.InsertPasskeyUser) db.InsertPasskeyUserParams {
	return db.InsertPasskeyUserParams{
		BcoName: user.BCOName,
		Email:   user.Email,
		ID:      user.ID,
		Creds:   user.Creds,
	}
}

func fromDBPasskeyUser(user db.PasskeyUser) auth.PasskeyUser {
	var creds webauthn.Credential
	_ = json.Unmarshal(user.Creds, &creds)

	return auth.PasskeyUser{
		ID:      user.ID,
		BCOName: user.BcoName,
		Email:   user.Email,
		Creds:   []webauthn.Credential{creds},
	}
}

func fromDBApprovedEmail(email db.ApprovedEmail) auth.ApprovedEmail {
	return auth.ApprovedEmail{
		Email:   email.Email,
		Used:    email.Used,
		Created: email.Created.Time,
		UsedAt:  email.UsedAt.Time,
	}
}

func toDBUpdatePasskeyUserCredentialsParams(params auth.UpdatePasskeyUserCredentials) db.UpdatePasskeyUserCredentialsParams {
	return db.UpdatePasskeyUserCredentialsParams{
		BcoName: params.BCOName,
		Creds:   params.Creds,
	}
}
