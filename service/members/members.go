package members

import (
	"boardfund/service/donations"
	"boardfund/service/fundevents"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/google/uuid"
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
	SearchMembersByUsername(ctx context.Context, arg string) ([]MemberSearchResult, error)
	GetMemberByUsername(ctx context.Context, username string) (*Member, error)
}

type donationStore interface {
	GetDonationsByDonorID(ctx context.Context, donorID uuid.UUID) ([]donations.Donation, error)
	SetDonationsToInactiveByDonorID(ctx context.Context, id uuid.UUID) ([]donations.Donation, error)
}

// eventRecorder writes the fund activity feed. Record does not return an error:
// it runs after the operation it describes has committed.
type eventRecorder interface {
	Record(ctx context.Context, record fundevents.Record)
}

type paymentsProvider interface {
	CancelSubscriptions(ctx context.Context, ids []string) ([]string, error)
}

type MemberService struct {
	memberStore      memberStore
	donationStore    donationStore
	paymentsProvider paymentsProvider
	events           eventRecorder

	logger *slog.Logger
}

func NewMemberService(memberStore memberStore, donationStore donationStore, paymentsProvider paymentsProvider, events eventRecorder, logger *slog.Logger) *MemberService {
	gob.Register(Member{})

	return &MemberService{
		memberStore:      memberStore,
		donationStore:    donationStore,
		paymentsProvider: paymentsProvider,
		events:           events,
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

// ErrSubscriptionsNotCancelled means the provider would not cancel every
// subscription, so the member was left active.
var ErrSubscriptionsNotCancelled = errors.New("could not cancel all subscriptions at the provider")

// DeactivateMember closes a member's account and cancels their recurring
// donations at the provider.
//
// The provider is called before anything is written. The previous order
// deactivated the member and their donations first, then cancelled; on a partial
// cancellation it returned an error and reactivated nothing, leaving a member
// and their donations closed locally while some subscriptions carried on
// charging them. Nothing retried it.
//
// Partial cancellation is now refused with everything left as it was, which is a
// state an admin can act on by trying again.
func (s MemberService) DeactivateMember(ctx context.Context, id uuid.UUID) (*Member, error) {
	existing, err := s.donationStore.GetDonationsByDonorID(ctx, id)
	if err != nil {
		s.logger.Error("failed to read donations before deactivating member",
			slog.String("error", err.Error()))

		return nil, err
	}

	var active []donations.Donation
	for _, donation := range existing {
		if donation.Active {
			active = append(active, donation)
		}
	}

	toCancel := extractProviderSubscriptionIDs(active)

	if len(toCancel) > 0 {
		cancelled, errCancel := s.paymentsProvider.CancelSubscriptions(ctx, toCancel)
		if errCancel != nil {
			s.logger.Error("failed to cancel subscriptions, member left active",
				slog.String("error", errCancel.Error()),
				slog.String("member_id", id.String()),
			)

			return nil, errCancel
		}

		// Compared by membership, not by count. Equal lengths would also be true
		// of a provider that returned duplicates or a different set of ids, and
		// the question here is whether anything we asked for is still running.
		uncancelled := uncancelledSubscriptions(cancelled, toCancel)
		if len(uncancelled) > 0 {
			s.logger.Error("could not cancel every subscription, member left active",
				slog.String("member_id", id.String()),
				slog.String("uncancelled", fmt.Sprintf("%v", uncancelled)),
			)

			return nil, fmt.Errorf("%w: %d of %d remain", ErrSubscriptionsNotCancelled,
				len(uncancelled), len(toCancel))
		}
	}

	member, err := s.memberStore.SetMemberToInactive(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate member", slog.String("error", err.Error()))

		return nil, err
	}

	deactivated, err := s.donationStore.SetDonationsToInactiveByDonorID(ctx, id)
	if err != nil {
		s.logger.Error("failed to deactivate donations", slog.String("error", err.Error()))

		// The subscriptions are already cancelled, so leaving the member closed
		// would strand donations that look active but can never be charged. Put
		// the member back and report the failure.
		if _, errRestore := s.memberStore.SetMemberToActive(ctx, id); errRestore != nil {
			s.logger.Error("failed to reactivate member after donations failed",
				slog.String("error", errRestore.Error()))
		}

		return nil, err
	}

	// Their donations belong to funds, and those funds' histories should say why
	// they stopped. Nothing recorded this before.
	for _, cancelled := range deactivated {
		s.events.Record(ctx, fundevents.Record{
			FundID:          cancelled.FundID,
			Kind:            fundevents.KindDonationCancelled,
			SubjectMemberID: &cancelled.DonorID,
			Detail:          "member deactivated",
			ReferenceID:     &cancelled.ID,
		})
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

	upsertMember := UpsertMember{
		ID:      newMemberID,
		Email:   member.Email,
		BCOName: member.BCOName,
	}

	newMember, err := s.memberStore.UpsertMember(ctx, upsertMember)
	if err != nil {
		s.logger.Error("failed to create member", slog.String("error", err.Error()))

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

func (s MemberService) SearchMembersByUsername(ctx context.Context, arg string) ([]MemberSearchResult, error) {
	members, err := s.memberStore.SearchMembersByUsername(ctx, arg)
	if err != nil {
		s.logger.Error("failed to search members", slog.String("error", err.Error()))

		return nil, err
	}

	return members, nil
}

func (s MemberService) GetMemberByUsername(ctx context.Context, username string) (*Member, error) {
	member, err := s.memberStore.GetMemberByUsername(ctx, username)
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
