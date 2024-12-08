package auth

import "time"

type Token struct {
	AccessTokenStr string
	IDTokenStr     string
	Expires        time.Time
}

type AuthResponse struct {
	Token         *Token
	ResetPassword bool
}
