package enrollments

import (
	"context"
	"github.com/google/uuid"
	"log/slog"
)

type enrollmentStore interface {
	InsertEnrollment(ctx context.Context, arg InsertEnrollment) (*Enrollment, error)
	InsertEnrollmentWithPaypalEmail(ctx context.Context, insertEnrollment InsertEnrollment, updatePaypalEmail UpdatePaypalEmail) (*Enrollment, error)
	GetEnrollmentByMemberID(ctx context.Context, arg GetEnrollmentForFundByMemberID) (*Enrollment, error)
	FundEnrollmentExists(ctx context.Context, arg FundEnrollmentExists) (*bool, error)
	GetActiveEnrollmentsForFund(ctx context.Context, arg uuid.UUID) ([]Enrollment, error)
	DeactivateEnrollment(ctx context.Context, arg uuid.UUID) (*Enrollment, error)
}

type EnrollmentsService struct {
	enrollmentStore enrollmentStore

	logger *slog.Logger
}

func NewEnrollmentsService(enrollmentStore enrollmentStore, logger *slog.Logger) *EnrollmentsService {
	return &EnrollmentsService{
		enrollmentStore: enrollmentStore,
		logger:          logger,
	}
}

func (s EnrollmentsService) DeactivateEnrollment(ctx context.Context, enrollmentID uuid.UUID) (*Enrollment, error) {
	enrollment, err := s.enrollmentStore.DeactivateEnrollment(ctx, enrollmentID)
	if err != nil {
		s.logger.Error("failed to deactivate enrollment", slog.String("error", err.Error()))

		return nil, err
	}

	return enrollment, nil
}

func (s EnrollmentsService) CreateEnrollment(ctx context.Context, createEnrollment CreateEnrollment) (*Enrollment, error) {
	insert := InsertEnrollment{
		MemberID:      createEnrollment.MemberID,
		FundID:        createEnrollment.FundID,
		ID:            uuid.New(),
		PaypalEmail:   createEnrollment.PaypalEmail,
		MemberBCOName: createEnrollment.MemberBCOName,
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
