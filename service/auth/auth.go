package auth

import (
	"boardfund/service/members"
	"context"
	"fmt"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"log/slog"
)

type authorizer interface {
	Authorize(ctx context.Context, user, pass string) (*Token, error)
}

type AuthService struct {
	authorizer authorizer

	logger *slog.Logger
}

func NewAuthService(authorizer authorizer, logger *slog.Logger) *AuthService {
	return &AuthService{
		authorizer: authorizer,
		logger:     logger,
	}
}

func (s AuthService) Authenticate(ctx context.Context, username, password string) (*members.Member, *Token, error) {
	token, err := s.authorizer.Authorize(ctx, username, password)
	if err != nil {
		s.logger.Error("failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	parsedToken, err := jwt.ParseString(token.TokenStr)
	if err != nil {
		s.logger.Error("failed to parse token", slog.String("error", err.Error()))

		return nil, nil, err
	}

	fmt.Printf("parsedToken: %+v\n", parsedToken)

	return nil, token, nil
}
