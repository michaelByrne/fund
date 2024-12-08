package jwtauth

import (
	"fmt"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"time"
)

type Token struct {
	keyset jwk.Set
}

func NewToken(keyset jwk.Set) *Token {
	return &Token{keyset: keyset}
}

func (t *Token) Verify(tokenStr string) (jwt.Token, error) {
	parsedToken, err := jwt.ParseString(
		tokenStr,
		jwt.WithKeySet(t.keyset),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(time.Second*5),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	return parsedToken, nil
}

func (t *Token) VerifyAdmin(tokenStr string) (jwt.Token, error) {
	parsedToken, err := jwt.ParseString(
		tokenStr,
		jwt.WithKeySet(t.keyset),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(time.Second*5),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims := parsedToken.PrivateClaims()
	if claims["cognito:groups"] == nil {
		return nil, fmt.Errorf("no groups claim found")
	}

	groups := claims["cognito:groups"].([]interface{})
	if len(groups) == 0 {
		return nil, fmt.Errorf("no groups found")
	}

	for _, group := range groups {
		groupStr, ok := group.(string)
		if !ok {
			continue
		}

		if groupStr == "bco-admin-group" {
			return parsedToken, nil
		}
	}

	return nil, fmt.Errorf("not an admin")
}
