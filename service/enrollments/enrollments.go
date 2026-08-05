package enrollments

import (
	"boardfund/service/donations"
	"boardfund/service/fundevents"
	"context"
	"github.com/google/uuid"
	"log/slog"
	"time"
)

// fundStore supplies the payout schedule. An enrollment's first payout is a
// property of the fund it joins, so the service reads it rather than having the
// handler compute a domain date and pass it down.
type fundStore interface {
	GetFundByID(ctx context.Context, id uuid.UUID) (*donations.Fund, error)
}

type enrollmentStore interface {
	InsertEnrollment(ctx context.Context, arg InsertEnrollment) (*Enrollment, error)
	InsertEnrollmentWithPaypalEmail(ctx context.Context, insertEnrollment InsertEnrollment, updatePaypalEmail UpdatePaypalEmail) (*Enrollment, error)
	GetEnrollmentByMemberID(ctx context.Context, arg GetEnrollmentForFundByMemberID) (*Enrollment, error)
	FundEnrollmentExists(ctx context.Context, arg FundEnrollmentExists) (*bool, error)
	GetActiveEnrollmentsForFund(ctx context.Context, arg uuid.UUID) ([]Enrollment, error)
	DeactivateEnrollment(ctx context.Context, arg uuid.UUID) (*Enrollment, error)
}

// eventRecorder writes the fund activity feed. Record does not return an error:
// it runs after the operation it describes has committed.
type eventRecorder interface {
	Record(ctx context.Context, record fundevents.Record)
}

type EnrollmentsService struct {
	enrollmentStore enrollmentStore
	fundStore       fundStore
	events          eventRecorder

	logger *slog.Logger
}

func NewEnrollmentsService(enrollmentStore enrollmentStore, fundStore fundStore, events eventRecorder, logger *slog.Logger) *EnrollmentsService {
	return &EnrollmentsService{
		enrollmentStore: enrollmentStore,
		fundStore:       fundStore,
		events:          events,
		logger:          logger,
	}
}

func (s EnrollmentsService) DeactivateEnrollment(ctx context.Context, enrollmentID uuid.UUID) (*Enrollment, error) {
	enrollment, err := s.enrollmentStore.DeactivateEnrollment(ctx, enrollmentID)
	if err != nil {
		s.logger.Error("failed to deactivate enrollment", slog.String("error", err.Error()))

		return nil, err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:          enrollment.FundID,
		Kind:            fundevents.KindEnrollmentCancelled,
		SubjectMemberID: &enrollment.MemberID,
		ReferenceID:     &enrollment.ID,
	})

	return enrollment, nil
}

func (s EnrollmentsService) CreateEnrollment(ctx context.Context, createEnrollment CreateEnrollment) (*Enrollment, error) {
	fund, err := s.fundStore.GetFundByID(ctx, createEnrollment.FundID)
	if err != nil {
		s.logger.Error("failed to get fund for enrollment", slog.String("error", err.Error()))

		return nil, err
	}

	insert := InsertEnrollment{
		MemberID:      createEnrollment.MemberID,
		FundID:        createEnrollment.FundID,
		ID:            uuid.New(),
		PaypalEmail:   createEnrollment.PaypalEmail,
		MemberBCOName: createEnrollment.MemberBCOName,

		// The fund's next scheduled payout: no waiting period, but a real date
		// rather than the instant they enrolled.
		FirstPayoutDate: fund.NextPaymentAfter(time.Now()),
	}

	updatePaypal := UpdatePaypalEmail{
		MemberID: createEnrollment.MemberID,
		Email:    createEnrollment.PaypalEmail,
	}

	enrollment, err := s.enrollmentStore.InsertEnrollmentWithPaypalEmail(ctx, insert, updatePaypal)
	if err != nil {
		s.logger.Error("failed to create enrollment", slog.String("error", err.Error()))

		return nil, err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:          enrollment.FundID,
		Kind:            fundevents.KindMemberEnrolled,
		SubjectMemberID: &enrollment.MemberID,
		ReferenceID:     &enrollment.ID,
	})

	return enrollment, nil
}

func (s EnrollmentsService) GetEnrollmentForFundByMemberID(ctx context.Context, fundID, memberID uuid.UUID) (*Enrollment, error) {
	arg := GetEnrollmentForFundByMemberID{
		FundID:   fundID,
		MemberID: memberID,
	}

	enrollment, err := s.enrollmentStore.GetEnrollmentByMemberID(ctx, arg)
	if err != nil {
		s.logger.Error("failed to get enrollment", slog.String("error", err.Error()))

		return nil, err
	}

	return enrollment, nil
}

func (s EnrollmentsService) FundEnrollmentExists(ctx context.Context, fundID, memberID uuid.UUID) (bool, error) {
	enrollment, err := s.enrollmentStore.FundEnrollmentExists(ctx, FundEnrollmentExists{
		FundID:   fundID,
		MemberID: memberID,
	})
	if err != nil {
		s.logger.Error("failed to check if enrollment exists", slog.String("error", err.Error()))

		return false, err
	}

	return *enrollment, nil
}

func (s EnrollmentsService) GetActiveEnrollmentsForFund(ctx context.Context, fundID uuid.UUID) ([]Enrollment, error) {
	enrollments, err := s.enrollmentStore.GetActiveEnrollmentsForFund(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to get active enrollments", slog.String("error", err.Error()))

		return nil, err
	}

	return enrollments, nil
}
