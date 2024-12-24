package members

import (
	"boardfund/service/donations"
	"context"
	"encoding/gob"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"log/slog"
)

type memberStore interface {
	GetMemberByID(ctx context.Context, id uuid.UUID) (*Member, error)
	UpsertMember(ctx context.Context, member UpsertMember) (*Member, error)
	GetMembers(ctx context.Context) ([]Member, error)
	SetMemberToInactive(ctx context.Context, id uuid.UUID) (*Member, error)
	SetMemberToActive(ctx context.Context, id uuid.UUID) (*Member, error)
	GetActiveMembers(ctx context.Context) ([]Member, error)
	GetMemberWithDonations(ctx context.Context, id uuid.UUID) (*Member, error)
}

type donationStore interface {
	SetDonationsToInactiveByDonorID(ctx context.Context, id uuid.UUID) ([]donations.Donation, error)
	SetDonationsToActive(ctx context.Context, ids []uuid.UUID) ([]donations.Donation, error)
}

//go:generate moq -pkg mocks -out ../mocks/auth_moq.go . AuthProvider
type AuthProvider interface {
	CreateUser(ctx context.Context, username, email string, memberID uuid.UUID) (string, error)
	DeleteUser(ctx context.Context, username string) error
}

type paymentsProvider interface {
	CancelSubscriptions(ctx context.Context, ids []string) ([]string, error)
}

type MemberService struct {
	memberStore      memberStore
	donationStore    donationStore
	authProvider     AuthProvider
	paymentsProvider paymentsProvider

	logger *slog.Logger
}

func NewMemberService(memberStore memberStore, donationStore donationStore, authProvider AuthProvider, paymentsProvider paymentsProvider, logger *slog.Logger) *MemberService {
	gob.Register(Member{})

	return &MemberService{
		memberStore:      memberStore,
		donationStore:    donationStore,
		authProvider:     authProvider,
		paymentsProvider: paymentsProvider,
		logger:           logger,
	}
}

func (s MemberService) GetMemberWithDonations(ctx context.Context, id uuid.UUID) (*Member, error) {
	member, err := s.memberStore.GetMemberWithDonations(ctx, id)
	if err != nil {
		s.logger.Error("failed to get member with donations", slog.String("error", err.Error()))

		return nil, err
	}

	return member, nil
}

func (s MemberService) DeactivateMember(ctx context.Context, id uuid.UUID) (*Member, error) {
	toDeactivate, err := s.memberStore.GetMemberByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get member", slog.String("error", err.Error()))

		return nil, err
	}

	member, err := s.memberStore.SetMemberToInactive(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate member", slog.String("error", err.Error()))

		return nil, err
	}

	deactivatedDonations, err := s.donationStore.SetDonationsToInactiveByDonorID(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate donations", slog.String("error", err.Error()))

		_, err = s.memberStore.SetMemberToActive(ctx, id)
		if err != nil {
			s.logger.Error("failed to reactivate member", slog.String("error", err.Error()))

			return nil, err
		}

		return nil, err
	}

	subscriptionIDs := extractProviderSubscriptionIDs(deactivatedDonations)

	cancelled, err := s.paymentsProvider.CancelSubscriptions(ctx, subscriptionIDs)
	if err != nil {
		s.logger.Error("failed to cancel subscriptions", slog.String("error", err.Error()))

		_, err = s.memberStore.SetMemberToActive(ctx, id)
		if err != nil {
			s.logger.Error("failed to reactivate member", slog.String("error", err.Error()))

			return nil, err
		}

		_, err = s.donationStore.SetDonationsToActive(ctx, extractDonationIDs(deactivatedDonations))
		if err != nil {
			s.logger.Error("failed to reactivate donations", slog.String("error", err.Error()))

			return nil, err
		}

		return nil, err
	}

	if len(cancelled) != len(subscriptionIDs) {
		uncancelled := uncancelledSubscriptions(cancelled, subscriptionIDs)
		s.logger.Error("failed to cancel all subscriptions", slog.String("uncancelled", fmt.Sprintf("%v", uncancelled)))

		return nil, fmt.Errorf("failed to cancel all subscriptions")
	}

	err = s.authProvider.DeleteUser(ctx, toDeactivate.BCOName)
	if err != nil {
		s.logger.Error("failed to delete auth provider user", slog.String("error", err.Error()))

		return nil, err
	}

	return member, nil
}

func (s MemberService) ListActiveMembers(ctx context.Context) ([]Member, error) {
	members, err := s.memberStore.GetActiveMembers(ctx)
	if err != nil {
		s.logger.Error("failed to get active members", slog.String("error", err.Error()))

		return nil, err
	}

	return members, nil
}

func (s MemberService) ListMembers(ctx context.Context) ([]Member, error) {
	members, err := s.memberStore.GetMembers(ctx)
	if err != nil {
		s.logger.Error("failed to get members", slog.String("error", err.Error()))

		return nil, err
	}

	return members, nil
}

func (s MemberService) CreateMember(ctx context.Context, member CreateMember) (*Member, error) {
	newMemberID := uuid.New()

	cognitoID, err := s.authProvider.CreateUser(ctx, member.BCOName, member.Email, newMemberID)
	if err != nil {
		s.logger.Error("failed to create cognito user", slog.String("error", err.Error()))

		return nil, err
	}

	upsertMember := UpsertMember{
		ID:        newMemberID,
		CognitoID: cognitoID,
		Email:     member.Email,
		BCOName:   member.BCOName,
		FirstName: member.FirstName,
		LastName:  member.LastName,
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

func (s MemberService) GetMemberByID(ctx context.Context, id uuid.UUID) (*Member, error) {
	member, err := s.memberStore.GetMemberByID(ctx, id)
	if err != nil {
		s.logger.Error("failed to get member", slog.String("error", err.Error()))

		return nil, err
	}

	return member, nil
}

func extractProviderSubscriptionIDs(donations []donations.Donation) []string {
	var subscriptionIDs []string

	for _, donation := range donations {
		if donation.ProviderSubscriptionID != "" {
			subscriptionIDs = append(subscriptionIDs, donation.ProviderSubscriptionID)
		}
	}

	return subscriptionIDs
}

func extractDonationIDs(donations []donations.Donation) []uuid.UUID {
	var ids []uuid.UUID

	for _, donation := range donations {
		ids = append(ids, donation.ID)
	}

	return ids
}

func uncancelledSubscriptions(cancelled []string, all []string) []string {
	var uncancelled []string

	for _, sub := range all {
		var found bool
		for _, c := range cancelled {
			if sub == c {
				found = true
				break
			}
		}

		if !found {
			uncancelled = append(uncancelled, sub)
		}
	}

	return uncancelled
}
