package jwtauth

import (
	"fmt"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"time"
)

// AdminGroup is the Cognito group that grants admin access. It is the single
// source of truth for that decision: the member.roles column plays no part, and
// nothing in the application ever writes ADMIN to it.
const AdminGroup = "bco-admin-group"

// GroupsClaim is where Cognito puts group membership on the ID token.
const GroupsClaim = "cognito:groups"

type Token struct {
	keyset jwk.Set
}

// HasGroup reports whether a parsed token carries the given Cognito group.
func HasGroup(claims map[string]any, group string) bool {
	raw, ok := claims[GroupsClaim]
	if !ok {
		return false
	}

	groups, ok := raw.([]any)
	if !ok {
		return false
	}

	for _, g := range groups {
		if name, ok := g.(string); ok && name == group {
			return true
		}
	}

	return false
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

	if !HasGroup(parsedToken.PrivateClaims(), AdminGroup) {
		return nil, fmt.Errorf("not an admin")
	}

	return parsedToken, nil
}
