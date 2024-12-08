package members

import (
	"context"
	"encoding/gob"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"log/slog"
)

type MemberRole string

const (
	AdminRole MemberRole = "ADMIN"
	DonorRole MemberRole = "DONOR"
	PayeeRole MemberRole = "PAYEE"
)

type Member struct {
	ID              uuid.UUID
	Email           string `json:"email"`
	BCOName         string `json:"bco_name"`
	IPAddress       string
	CognitoID       string
	FirstName       string       `json:"first_name"`
	LastName        string       `json:"last_name"`
	ProviderPayerID string       `json:"provider_payer_id"`
	Roles           []MemberRole `json:"role"`
}

type UpsertMember struct {
	ID              uuid.UUID
	Email           string
	BCOName         string
	IPAddress       string
	CognitoID       string
	FirstName       string
	LastName        string
	ProviderPayerID string
	Roles           []MemberRole
}

type memberStore interface {
	GetMemberByID(ctx context.Context, id uuid.UUID) (*Member, error)
	UpsertMember(ctx context.Context, member UpsertMember) (*Member, error)
}

type authProvider interface {
	CreateUser(ctx context.Context, username, email string, memberID uuid.UUID) (string, error)
	DeleteUser(ctx context.Context, username string) error
}

type MemberService struct {
	memberStore  memberStore
	authProvider authProvider

	logger *slog.Logger
}

func NewMemberService(memberStore memberStore, authProvider authProvider, logger *slog.Logger) *MemberService {
	gob.Register(Member{})

	return &MemberService{
		memberStore:  memberStore,
		authProvider: authProvider,
		logger:       logger,
	}
}

func (s MemberService) CreateMember(ctx context.Context, member Member) (*Member, error) {
	newMemberID := uuid.New()

	cognitoID, err := s.authProvider.CreateUser(ctx, member.BCOName, member.Email, newMemberID)
	if err != nil {
		s.logger.Error("failed to create cognito user", slog.String("error", err.Error()))

		return nil, err
	}

	upsertMember := UpsertMember{
		ID:              newMemberID,
		CognitoID:       cognitoID,
		Email:           member.Email,
		BCOName:         member.BCOName,
		IPAddress:       member.IPAddress,
		FirstName:       member.FirstName,
		LastName:        member.LastName,
		ProviderPayerID: member.ProviderPayerID,
	}

	newMember, err := s.memberStore.UpsertMember(ctx, upsertMember)
	if err != nil {
		s.logger.Error("failed to create member", slog.String("error", err.Error()))

		deleteErr := s.authProvider.DeleteUser(ctx, member.BCOName)
		if deleteErr != nil {
			s.logger.Error("failed to delete cognito user", slog.String("error", deleteErr.Error()))

			return nil, errors.Wrap(err, deleteErr.Error())
		}

		return nil, err
	}

	return newMember, nil
}
