package auth

import (
	"errors"
	"time"
)

type ApprovedEmail struct {
	Email   string
	Used    bool
	Created time.Time
	UsedAt  time.Time
}

type Token struct {
	AccessTokenStr string
	IDTokenStr     string
	Expires        time.Time
}

type AuthResponse struct {
	Token         *Token
	ResetPassword bool
}

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAuthenticateOther  = errors.New("unable to authenticate")
	ErrNewUserOther       = errors.New("unable to create new user")
	ErrUsernameExists     = errors.New("username already exists")
	ErrInvalidPassword    = errors.New("invalid password")
)
