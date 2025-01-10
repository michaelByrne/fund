package auth

import "github.com/go-webauthn/webauthn/webauthn"

type PasskeyUser struct {
	ID      []byte
	BCOName string
	Email   string
	Creds   []webauthn.Credential
}

type InsertPasskeyUser struct {
	BCOName string
	Email   string
	ID      []byte
	Creds   []byte
}

type UpdatePasskeyUserCredentials struct {
	BCOName string
	Creds   []byte
}

func (o *PasskeyUser) WebAuthnID() []byte {
	return o.ID
}

func (o *PasskeyUser) WebAuthnName() string {
	return o.Email
}

func (o *PasskeyUser) WebAuthnDisplayName() string {
	return o.BCOName
}

func (o *PasskeyUser) WebAuthnIcon() string {
	return ""
}

func (o *PasskeyUser) WebAuthnCredentials() []webauthn.Credential {
	return o.Creds
}

func (o *PasskeyUser) AddCredential(credential *webauthn.Credential) {
	o.Creds = append(o.Creds, *credential)
}

func (o *PasskeyUser) UpdateCredential(credential *webauthn.Credential) {
	for i, c := range o.Creds {
		if string(c.ID) == string(credential.ID) {
			o.Creds[i] = *credential
		}
	}
}
