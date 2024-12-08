package auth

import (
	"boardfund/service/members"
	"context"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"log/slog"
)

type authorizer interface {
	Authorize(ctx context.Context, user, pass string) (*AuthResponse, error)
	SetPassword(ctx context.Context, user, old, new string) error
}

type memberStore interface {
	GetMemberByID(ctx context.Context, id uuid.UUID) (*members.Member, error)
}

type AuthService struct {
	authorizer  authorizer
	memberStore memberStore

	logger *slog.Logger
}

func NewAuthService(authorizer authorizer, memberStore memberStore, logger *slog.Logger) *AuthService {
	return &AuthService{
		authorizer:  authorizer,
		memberStore: memberStore,
		logger:      logger,
	}
}

func (s AuthService) Authenticate(ctx context.Context, username, password string) (*members.Member, *AuthResponse, error) {
	resp, err := s.authorizer.Authorize(ctx, username, password)
	if err != nil {
		s.logger.Error("failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	if resp.ResetPassword {
		return nil, resp, nil
	}

	parsedToken, err := jwt.ParseString(resp.Token.IDTokenStr, jwt.WithVerify(false))
	if err != nil {
		s.logger.Error("failed to parse token", slog.String("error", err.Error()))

		return nil, nil, err
	}

	claims := parsedToken.PrivateClaims()
	memberID := claims["custom:member_id"].(string)

	memberUUID, err := uuid.Parse(memberID)
	if err != nil {
		s.logger.Error("failed to parse member id", slog.String("error", err.Error()))

		return nil, nil, err
	}

	member, err := s.memberStore.GetMemberByID(ctx, memberUUID)
	if err != nil {
		s.logger.Error("failed to get member by id", slog.String("error", err.Error()))

		return nil, nil, err
	}

	return member, resp, nil
}

func (s AuthService) ResetPassword(ctx context.Context, username, password, newPassword string) (*members.Member, *AuthResponse, error) {
	err := s.authorizer.SetPassword(ctx, username, password, newPassword)
	if err != nil {
		s.logger.Error("failed to reset password", slog.String("error", err.Error()))

		return nil, nil, err
	}

	member, autResp, err := s.Authenticate(ctx, username, newPassword)
	if err != nil {
		s.logger.Error("failed to authenticate", slog.String("error", err.Error()))

		return nil, nil, err
	}

	return member, autResp, nil
}
