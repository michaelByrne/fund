package payouts

import (
	"boardfund/service/fundevents"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNoEnrollments is returned when a fund has no one eligible to be paid. It is
	// not a failure: a fund with no active enrollees simply has no batch this period.
	ErrNoEnrollments = errors.New("no active enrollments eligible for payout")

	// ErrNotApprovable is returned when a batch could not be approved or rejected
	// because it was no longer awaiting approval — already approved, already
	// submitted, or swept away by the expiry sweep.
	ErrNotApprovable = errors.New("batch is not awaiting approval")

	// ErrNotSubmittable is returned when a batch is not in 'ready', which means it
	// was never approved or has already been sent.
	ErrNotSubmittable = errors.New("batch is not ready for submission")
)

// DefaultApprovalWindow is how long a treasurer has to approve a batch before it
// is cancelled. Deliberately shorter than a payout period so an expired batch can
// be re-planned without slipping the schedule.
const DefaultApprovalWindow = 72 * time.Hour

// DefaultReminderWindow is how close to the deadline a batch must be before a
// reminder goes out.
const DefaultReminderWindow = 24 * time.Hour

type payoutStore interface {
	CreateBatchWithPayouts(ctx context.Context, batch InsertBatch, items []InsertPayout) (*Batch, []Payout, error)
	GetBatchByID(ctx context.Context, id uuid.UUID) (*Batch, error)
	GetBatchBySenderBatchID(ctx context.Context, id uuid.UUID) (*Batch, error)
	GetBatchesForFund(ctx context.Context, fundID uuid.UUID) ([]Batch, error)
	GetBatchesByStatus(ctx context.Context, status Status) ([]Batch, error)
	GetPayoutsForBatch(ctx context.Context, batchID uuid.UUID) ([]Payout, error)
	GetEnrollmentsForPayout(ctx context.Context, fundID uuid.UUID) ([]PayoutEnrollment, error)
	ApproveBatch(ctx context.Context, arg ApproveBatch) (*Batch, error)
	RejectBatch(ctx context.Context, arg RejectBatch) (*Batch, error)
	CancelExpiredBatches(ctx context.Context) ([]Batch, error)
	GetBatchesNeedingReminder(ctx context.Context, within time.Duration) ([]Batch, error)
	MarkReminderSent(ctx context.Context, batchID uuid.UUID) (*Batch, error)
	SetBatchSubmitted(ctx context.Context, arg SetBatchSubmitted) (*Batch, error)
	SetBatchStatus(ctx context.Context, arg SetBatchStatus) (*Batch, error)
	SetPayoutResult(ctx context.Context, arg SetPayoutResult) (*Payout, error)
	SetPayoutStatusByProviderItemID(ctx context.Context, arg SetPayoutStatusByItem) (*Payout, error)
}

// PayoutsProvider is the payments provider's batch payout surface. SubmitBatch must
// treat senderBatchID as an idempotency key: calling it twice with the same value
// must not move money twice.
type PayoutsProvider interface {
	SubmitBatch(ctx context.Context, senderBatchID uuid.UUID, note string, items []ProviderPayoutItem) (*ProviderBatchResult, error)
	GetBatchStatus(ctx context.Context, providerBatchID string) (*ProviderBatchResult, error)
}

// eventRecorder writes the fund activity feed. Record does not return an error:
// it runs after the operation it describes has committed.
type eventRecorder interface {
	Record(ctx context.Context, record fundevents.Record)
}

// approvalNotifier is told when a batch needs a treasurer's attention. Kept narrow
// so the service does not depend on how the message is delivered.
type approvalNotifier interface {
	NotifyApprovalRequired(ctx context.Context, batch Batch) error
	NotifyApprovalExpiring(ctx context.Context, batch Batch) error
}

type PayoutService struct {
	payoutStore payoutStore
	provider    PayoutsProvider
	notifier    approvalNotifier
	events      eventRecorder

	approvalWindow time.Duration
	reminderWindow time.Duration

	logger *slog.Logger
}

func NewPayoutService(payoutStore payoutStore, provider PayoutsProvider, notifier approvalNotifier, events eventRecorder, approvalWindow, reminderWindow time.Duration, logger *slog.Logger) *PayoutService {
	if approvalWindow <= 0 {
		approvalWindow = DefaultApprovalWindow
	}

	if reminderWindow <= 0 {
		reminderWindow = DefaultReminderWindow
	}

	return &PayoutService{
		payoutStore:    payoutStore,
		provider:       provider,
		notifier:       notifier,
		events:         events,
		approvalWindow: approvalWindow,
		reminderWindow: reminderWindow,
		logger:         logger,
	}
}

// PlanBatch builds a batch for a fund and leaves it awaiting approval. It writes
// nothing to the provider: planning is a local, reversible act, and no money can
// move until ApproveBatch and SubmitBatch have both run.
func (s PayoutService) PlanBatch(ctx context.Context, req PlanBatch) (*Batch, error) {
	logger := s.logger.With(slog.String("fund_id", req.FundID.String()))

	enrollments, err := s.payoutStore.GetEnrollmentsForPayout(ctx, req.FundID)
	if err != nil {
		logger.Error("failed to get enrollments for payout", slog.String("error", err.Error()))

		return nil, err
	}

	payable := make([]PayoutEnrollment, 0, len(enrollments))
	for _, enrollment := range enrollments {
		if enrollment.PaypalEmail == "" {
			// Nowhere to send it. Skipped rather than failed so one unconfigured
			// member cannot block everyone else's payout.
			logger.Warn("enrollment has no paypal email, skipping",
				slog.String("enrollment_id", enrollment.ID.String()),
				slog.String("member_id", enrollment.MemberID.String()),
			)

			continue
		}

		payable = append(payable, enrollment)
	}

	if len(payable) == 0 {
		return nil, ErrNoEnrollments
	}

	batchID := uuid.New()

	status := StatusAwaitingApproval
	var deadline *time.Time
	if req.RequireApproval {
		window := req.ApprovalWindow
		if window <= 0 {
			window = s.approvalWindow
		}

		expires := time.Now().Add(window)
		deadline = &expires
	} else {
		status = StatusReady
	}

	items := make([]InsertPayout, 0, len(payable))
	for _, enrollment := range payable {
		items = append(items, InsertPayout{
			ID:               uuid.New(),
			FundEnrollmentID: enrollment.ID,
			BatchID:          batchID,
			AmountCents:      req.AmountCents,
			Status:           StatusPlanned,
			Description:      req.Description,
			PayoutDate:       req.PayoutDate,
			DestinationEmail: enrollment.PaypalEmail,
		})
	}

	insert := InsertBatch{
		ID:               batchID,
		FundID:           req.FundID,
		SenderBatchID:    uuid.New(),
		AmountCents:      req.AmountCents * int32(len(payable)),
		NumEnrollments:   int32(len(payable)),
		Status:           status,
		Description:      req.Description,
		Notes:            req.Notes,
		PayoutDate:       req.PayoutDate,
		ApprovalDeadline: deadline,
	}

	batch, _, err := s.payoutStore.CreateBatchWithPayouts(ctx, insert, items)
	if err != nil {
		logger.Error("failed to create batch with payouts", slog.String("error", err.Error()))

		return nil, err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:      batch.FundID,
		Kind:        fundevents.KindBatchPlanned,
		AmountCents: &batch.AmountCents,
		Detail:      fmt.Sprintf("%d payees", batch.NumEnrollments),
		ReferenceID: &batch.ID,
	})

	logger.Info("planned payout batch",
		slog.String("batch_id", batch.ID.String()),
		slog.Int("num_enrollments", int(batch.NumEnrollments)),
		slog.Int("amount_cents", int(batch.AmountCents)),
		slog.String("status", string(batch.Status)),
	)

	if batch.AwaitingApproval() && s.notifier != nil {
		if errNotify := s.notifier.NotifyApprovalRequired(ctx, *batch); errNotify != nil {
			// The batch exists and the deadline is running; a failed notification is
			// worth surfacing but must not undo the batch.
			logger.Error("failed to send approval notification", slog.String("error", errNotify.Error()))
		}
	}

	return batch, nil
}

// ApproveBatch records a treasurer's approval and moves the batch to 'ready'.
// The underlying update is a compare-and-set on 'awaiting_approval', so a batch
// cancelled by the sweep between read and write cannot be revived.
func (s PayoutService) ApproveBatch(ctx context.Context, batchID, approvedBy uuid.UUID) (*Batch, error) {
	batch, err := s.payoutStore.ApproveBatch(ctx, ApproveBatch{
		BatchID:    batchID,
		ApprovedBy: approvedBy,
	})
	if err != nil {
		s.logger.Error("failed to approve batch",
			slog.String("error", err.Error()),
			slog.String("batch_id", batchID.String()),
		)

		return nil, fmt.Errorf("%w: %w", ErrNotApprovable, err)
	}

	// The one event with a real actor throughout: a person authorised money to
	// leave the fund, and the feed should say who.
	s.events.Record(ctx, fundevents.Record{
		FundID:        batch.FundID,
		Kind:          fundevents.KindBatchApproved,
		ActorMemberID: &approvedBy,
		AmountCents:   &batch.AmountCents,
		ReferenceID:   &batch.ID,
	})

	s.logger.Info("batch approved",
		slog.String("batch_id", batch.ID.String()),
		slog.String("approved_by", approvedBy.String()),
	)

	return batch, nil
}

func (s PayoutService) RejectBatch(ctx context.Context, batchID uuid.UUID, reason string) (*Batch, error) {
	if reason == "" {
		reason = "rejected by treasurer"
	}

	batch, err := s.payoutStore.RejectBatch(ctx, RejectBatch{
		BatchID: batchID,
		Reason:  reason,
	})
	if err != nil {
		s.logger.Error("failed to reject batch",
			slog.String("error", err.Error()),
			slog.String("batch_id", batchID.String()),
		)

		return nil, fmt.Errorf("%w: %w", ErrNotApprovable, err)
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:      batch.FundID,
		Kind:        fundevents.KindBatchRejected,
		AmountCents: &batch.AmountCents,
		Detail:      reason,
		ReferenceID: &batch.ID,
	})

	s.logger.Info("batch rejected", slog.String("batch_id", batch.ID.String()), slog.String("reason", reason))

	return batch, nil
}

// SubmitBatch sends an approved batch to the provider. The sender batch ID is
// already persisted, so a submission that times out can be retried: the provider
// rejects the duplicate rather than paying twice.
func (s PayoutService) SubmitBatch(ctx context.Context, batchID uuid.UUID) (*Batch, error) {
	logger := s.logger.With(slog.String("batch_id", batchID.String()))

	batch, err := s.payoutStore.GetBatchByID(ctx, batchID)
	if err != nil {
		logger.Error("failed to get batch", slog.String("error", err.Error()))

		return nil, err
	}

	if batch.Status != StatusReady {
		return nil, fmt.Errorf("%w: status is %q", ErrNotSubmittable, batch.Status)
	}

	items, err := s.payoutStore.GetPayoutsForBatch(ctx, batch.ID)
	if err != nil {
		logger.Error("failed to get payouts for batch", slog.String("error", err.Error()))

		return nil, err
	}

	providerItems := make([]ProviderPayoutItem, 0, len(items))
	for _, item := range items {
		providerItems = append(providerItems, ProviderPayoutItem{
			PayoutID:      item.ID,
			ReceiverEmail: item.DestinationEmail,
			AmountCents:   item.AmountCents,
			Note:          item.Description,
		})
	}

	result, err := s.provider.SubmitBatch(ctx, batch.SenderBatchID, batch.Description, providerItems)
	if err != nil {
		logger.Error("failed to submit batch to provider", slog.String("error", err.Error()))

		// Deliberately not marked failed. The request may have reached the provider,
		// and a batch marked failed here would be re-planned and paid twice. Leave it
		// 'ready' so the reconciler can look it up by sender batch ID and settle it.
		return nil, err
	}

	submitted, err := s.payoutStore.SetBatchSubmitted(ctx, SetBatchSubmitted{
		BatchID:         batch.ID,
		ProviderBatchID: result.ProviderBatchID,
	})
	if err != nil {
		logger.Error("failed to record batch submission",
			slog.String("error", err.Error()),
			slog.String("provider_batch_id", result.ProviderBatchID),
		)

		return nil, err
	}

	// Per-item IDs are deliberately not read here: PayPal's create-batch response
	// carries only the batch header. Item IDs and their statuses arrive via
	// ReconcileBatch, matched back through sender_item_id.
	s.events.Record(ctx, fundevents.Record{
		FundID:      batch.FundID,
		Kind:        fundevents.KindBatchSubmitted,
		AmountCents: &batch.AmountCents,
		Detail:      fmt.Sprintf("%d payees", batch.NumEnrollments),
		ReferenceID: &batch.ID,
	})

	logger.Info("batch submitted",
		slog.String("provider_batch_id", result.ProviderBatchID),
		slog.Int("num_items", len(providerItems)),
	)

	return submitted, nil
}

// RunApprovalSweep cancels batches whose approval window closed and sends reminders
// for those approaching it. Safe to run repeatedly: cancellation is bounded by
// status, and each batch is reminded at most once.
func (s PayoutService) RunApprovalSweep(ctx context.Context) error {
	expired, err := s.payoutStore.CancelExpiredBatches(ctx)
	if err != nil {
		s.logger.Error("failed to cancel expired batches", slog.String("error", err.Error()))

		return err
	}

	for _, batch := range expired {
		s.logger.Warn("batch cancelled: approval window expired",
			slog.String("batch_id", batch.ID.String()),
			slog.String("fund_id", batch.FundID.String()),
			slog.Int("amount_cents", int(batch.AmountCents)),
		)
	}

	needReminder, err := s.payoutStore.GetBatchesNeedingReminder(ctx, s.reminderWindow)
	if err != nil {
		s.logger.Error("failed to get batches needing reminder", slog.String("error", err.Error()))

		return err
	}

	for _, batch := range needReminder {
		if s.notifier != nil {
			if errNotify := s.notifier.NotifyApprovalExpiring(ctx, batch); errNotify != nil {
				s.logger.Error("failed to send expiry reminder",
					slog.String("error", errNotify.Error()),
					slog.String("batch_id", batch.ID.String()),
				)

				// Leave reminder_sent_at unset so the next sweep tries again.
				continue
			}
		}

		_, errMark := s.payoutStore.MarkReminderSent(ctx, batch.ID)
		if errMark != nil {
			s.logger.Error("failed to mark reminder sent",
				slog.String("error", errMark.Error()),
				slog.String("batch_id", batch.ID.String()),
			)
		}
	}

	s.logger.Info("approval sweep complete",
		slog.Int("cancelled", len(expired)),
		slog.Int("reminded", len(needReminder)),
	)

	return nil
}

// ReconcileBatch polls the provider for a submitted batch and writes back per-item
// status. This is authoritative: webhooks are an optimisation, and a dropped one
// must not leave a payout stranded.
func (s PayoutService) ReconcileBatch(ctx context.Context, batchID uuid.UUID) error {
	logger := s.logger.With(slog.String("batch_id", batchID.String()))

	batch, err := s.payoutStore.GetBatchByID(ctx, batchID)
	if err != nil {
		logger.Error("failed to get batch", slog.String("error", err.Error()))

		return err
	}

	if batch.ProviderBatchID == "" {
		logger.Info("batch has not been submitted, nothing to reconcile")

		return nil
	}

	result, err := s.provider.GetBatchStatus(ctx, batch.ProviderBatchID)
	if err != nil {
		logger.Error("failed to get batch status from provider", slog.String("error", err.Error()))

		return err
	}

	for _, item := range result.Items {
		if item.PayoutID == uuid.Nil {
			// sender_item_id did not round-trip as one of our payout IDs. Refusing to
			// guess: a mis-attributed status here would mark the wrong person paid.
			logger.Error("provider item could not be matched to a payout",
				slog.String("provider_item_id", item.ProviderPayoutItemID),
			)

			continue
		}

		_, errItem := s.payoutStore.SetPayoutResult(ctx, SetPayoutResult{
			PayoutID:             item.PayoutID,
			ProviderPayoutItemID: item.ProviderPayoutItemID,
			Status:               ProviderStatusToStatus(item.Status),
			ProviderFeeCents:     item.FeeCents,
		})
		if errItem != nil {
			logger.Error("failed to update payout result",
				slog.String("error", errItem.Error()),
				slog.String("payout_id", item.PayoutID.String()),
			)
		}
	}

	settled := ProviderStatusToStatus(result.Status)

	_, err = s.payoutStore.SetBatchStatus(ctx, SetBatchStatus{
		BatchID: batch.ID,
		Status:  settled,
	})
	if err != nil {
		logger.Error("failed to update batch status", slog.String("error", err.Error()))

		return err
	}

	// Only once it stops moving. Recording every poll would bury the feed under
	// "still pending".
	if settled.Terminal() && settled != batch.Status {
		s.events.Record(ctx, fundevents.Record{
			FundID:      batch.FundID,
			Kind:        fundevents.KindBatchSettled,
			AmountCents: &batch.AmountCents,
			Detail:      string(settled),
			ReferenceID: &batch.ID,
		})
	}

	return nil
}

func (s PayoutService) GetBatchesForFund(ctx context.Context, fundID uuid.UUID) ([]Batch, error) {
	return s.payoutStore.GetBatchesForFund(ctx, fundID)
}

func (s PayoutService) GetBatchesAwaitingApproval(ctx context.Context) ([]Batch, error) {
	return s.payoutStore.GetBatchesByStatus(ctx, StatusAwaitingApproval)
}

func (s PayoutService) GetBatchByID(ctx context.Context, id uuid.UUID) (*Batch, error) {
	return s.payoutStore.GetBatchByID(ctx, id)
}

func (s PayoutService) GetPayoutsForBatch(ctx context.Context, batchID uuid.UUID) ([]Payout, error) {
	return s.payoutStore.GetPayoutsForBatch(ctx, batchID)
}
