package auth

import (
	"boardfund/service/members"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"log/slog"
)

type memberStore interface {
	GetMemberByID(ctx context.Context, id uuid.UUID) (*members.Member, error)
	UpsertMember(ctx context.Context, upsert members.UpsertMember) (*members.Member, error)
}

type authStore interface {
	GetPasskeyUser(ctx context.Context, arg string) (*PasskeyUser, error)
	InsertPasskeyUser(ctx context.Context, arg InsertPasskeyUser) (*PasskeyUser, error)
	UpdatePasskeyUserCredentials(ctx context.Context, credentials UpdatePasskeyUserCredentials) (*PasskeyUser, error)
	GetPasskeyUserByID(ctx context.Context, arg uuid.UUID) (*PasskeyUser, error)
	InsertApprovedEmail(ctx context.Context, arg string) (*ApprovedEmail, error)
	GetApprovedEmail(ctx context.Context, arg string) (*ApprovedEmail, error)
	MarkEmailAsUsed(ctx context.Context, email string) (*ApprovedEmail, error)
	PasskeyEmailExists(ctx context.Context, email string) (bool, error)
	PasskeyUsernameExists(ctx context.Context, bcoName string) (bool, error)
	GetApprovedEmails(ctx context.Context) ([]ApprovedEmail, error)
	DeleteApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error)
}

type authorizer interface {
	Authorize(ctx context.Context, user, pass string) (*AuthResponse, error)
	SetPassword(ctx context.Context, user, old, new string) error
	CreateUser(ctx context.Context, username, email string, memberID uuid.UUID) (string, error)
}

type AuthService struct {
	memberStore memberStore
	authStore   authStore
	authorizer  authorizer

	logger *slog.Logger
}

func NewAuthService(memberStore memberStore, authStore authStore, authorizer authorizer, logger *slog.Logger) *AuthService {
	return &AuthService{
		memberStore: memberStore,
		authStore:   authStore,
		authorizer:  authorizer,
		logger:      logger,
	}
}

func (s AuthService) Register(ctx context.Context, username, email string) (*members.Member, error) {
	memberID := uuid.New()

	cognitoID, err := s.authorizer.CreateUser(ctx, username, email, memberID)
	if err != nil {
		s.logger.Error("failed to create user", slog.String("error", err.Error()))

		return nil, err
	}

	upsert := members.UpsertMember{
		ID:        memberID,
		CognitoID: cognitoID,
		Email:     email,
		BCOName:   username,
	}

	member, err := s.memberStore.UpsertMember(ctx, upsert)
	if err != nil {
		s.logger.Error("failed to upsert member", slog.String("error", err.Error()))

		return nil, err
	}

	return member, nil
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

func (s AuthService) GetApprovedEmails(ctx context.Context) ([]ApprovedEmail, error) {
	emails, err := s.authStore.GetApprovedEmails(ctx)
	if err != nil {
		s.logger.Error("failed to get approved emails", slog.String("error", err.Error()))

		return nil, err
	}

	return emails, nil
}

func (s AuthService) CreatePasskeyUser(ctx context.Context, bcoName, email string) (*PasskeyUser, error) {
	insert := InsertPasskeyUser{
		BCOName: bcoName,
		Email:   email,
		ID:      []byte(uuid.New().String()),
	}

	user, err := s.authStore.InsertPasskeyUser(ctx, insert)
	if err != nil {
		s.logger.Error("failed to insert passkey user", slog.String("error", err.Error()))

		return nil, err
	}

	return user, nil
}

func (s AuthService) GetOrCreatePasskeyUser(ctx context.Context, bcoName string) (*PasskeyUser, error) {
	user, err := s.authStore.GetPasskeyUser(ctx, bcoName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			insert := InsertPasskeyUser{
				BCOName: bcoName,
				ID:      []byte(uuid.New().String()),
			}

			user, err = s.authStore.InsertPasskeyUser(ctx, insert)
			if err != nil {
				s.logger.Error("failed to insert passkey user", slog.String("error", err.Error()))

				return nil, err
			}

			return user, nil
		}

		s.logger.Error("failed to get passkey user", slog.String("error", err.Error()))

		return nil, err
	}

	return user, nil
}

func (s AuthService) GetPasskeyUserByID(ctx context.Context, id uuid.UUID) (*PasskeyUser, error) {
	user, err := s.authStore.GetPasskeyUserByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get passkey user by id", slog.String("error", err.Error()))

		return nil, err
	}

	return user, nil
}

func (s AuthService) UpdatePasskeyUserCredentials(ctx context.Context, bcoName string, creds []byte) (*PasskeyUser, error) {
	credentials := UpdatePasskeyUserCredentials{
		BCOName: bcoName,
		Creds:   creds,
	}

	user, err := s.authStore.UpdatePasskeyUserCredentials(ctx, credentials)
	if err != nil {
		s.logger.Error("failed to update passkey user credentials", slog.String("error", err.Error()))

		return nil, err
	}

	return user, nil
}

func (s AuthService) GetApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.GetApprovedEmail(ctx, email)
	if err != nil {
		s.logger.Error("failed to get approved email", slog.String("error", err.Error()))

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("email not approved")
		}

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) MarkEmailAsUsed(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.MarkEmailAsUsed(ctx, email)
	if err != nil {
		s.logger.Error("failed to mark email as used", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) InsertApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.InsertApprovedEmail(ctx, email)
	if err != nil {
		s.logger.Error("failed to insert approved email", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
}

func (s AuthService) ValidateNewPasskeyUser(ctx context.Context, bcoName, email string) error {
	if exists, err := s.authStore.PasskeyUsernameExists(ctx, bcoName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("username already exists")
	}

	if exists, err := s.authStore.PasskeyEmailExists(ctx, email); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("email already exists")
	}

	return nil
}

func (s AuthService) DeleteApprovedEmail(ctx context.Context, email string) (*ApprovedEmail, error) {
	approvedEmail, err := s.authStore.DeleteApprovedEmail(ctx, email)
	if err != nil {
		s.logger.Error("failed to delete approved email", slog.String("error", err.Error()))

		return nil, err
	}

	return approvedEmail, nil
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
