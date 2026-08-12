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

	// ErrFundInactive is returned when planning against a closed fund.
	// Deactivating a fund cancels its donations but leaves enrollments intact,
	// so without this a fund that stopped taking money could still send it.
	ErrFundInactive = errors.New("fund is not active")

	// ErrFundNotFound is returned when the fund does not exist at all. Kept
	// distinct from ErrFundInactive because a mistyped id and a closed fund need
	// different things done about them, and distinct from the raw pgx error,
	// which reaches a CLI user as "no rows in result set".
	ErrFundNotFound = errors.New("fund not found")
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
	GetDetailedBatchesByStatus(ctx context.Context, status Status) ([]BatchDetail, error)
	GetPayoutsForBatch(ctx context.Context, batchID uuid.UUID) ([]Payout, error)
	GetEnrollmentsForPayout(ctx context.Context, fundID uuid.UUID) ([]PayoutEnrollment, error)
	GetEnrollmentsInUnsentBatches(ctx context.Context, fundID uuid.UUID) ([]uuid.UUID, error)
	RequeueOneTimeFundPayout(ctx context.Context, fundID uuid.UUID) (bool, error)
	GetFundsDueForPayout(ctx context.Context) ([]DueFund, error)
	GetFundBalanceCents(ctx context.Context, fundID uuid.UUID) (int64, error)
	AdvanceFundNextPayment(ctx context.Context, fundID uuid.UUID) error
	IsFundActive(ctx context.Context, fundID uuid.UUID) (bool, error)
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

	// Checked before the enrollment query so a closed fund reports why, rather
	// than reporting that nobody is eligible -- which is what the filtering in
	// that query would otherwise produce, and reads like a different problem.
	active, err := s.payoutStore.IsFundActive(ctx, req.FundID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check whether the fund is active", slog.String("error", err.Error()))

		return nil, err
	}

	if !active {
		return nil, ErrFundInactive
	}

	enrollments, err := s.payoutStore.GetEnrollmentsForPayout(ctx, req.FundID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get enrollments for payout", slog.String("error", err.Error()))

		return nil, err
	}

	payable := make([]PayoutEnrollment, 0, len(enrollments))
	for _, enrollment := range enrollments {
		if enrollment.PaypalEmail == "" {
			// Nowhere to send it. Skipped rather than failed so one unconfigured
			// member cannot block everyone else's payout.
			logger.WarnContext(ctx, "enrollment has no paypal email, skipping",
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
		logger.ErrorContext(ctx, "failed to create batch with payouts", slog.String("error", err.Error()))

		return nil, err
	}

	s.events.Record(ctx, fundevents.Record{
		FundID:      batch.FundID,
		Kind:        fundevents.KindBatchPlanned,
		AmountCents: &batch.AmountCents,
		Detail:      fmt.Sprintf("%d payees", batch.NumEnrollments),
		ReferenceID: &batch.ID,
	})

	logger.InfoContext(ctx, "planned payout batch",
		slog.String("batch_id", batch.ID.String()),
		slog.Int("num_enrollments", int(batch.NumEnrollments)),
		slog.Int("amount_cents", int(batch.AmountCents)),
		slog.String("status", string(batch.Status)),
	)

	if batch.AwaitingApproval() && s.notifier != nil {
		if errNotify := s.notifier.NotifyApprovalRequired(ctx, *batch); errNotify != nil {
			// The batch exists and the deadline is running; a failed notification is
			// worth surfacing but must not undo the batch.
			logger.ErrorContext(ctx, "failed to send approval notification", slog.String("error", errNotify.Error()))
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
		s.logger.ErrorContext(ctx, "failed to approve batch",
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

	s.logger.InfoContext(ctx, "batch approved",
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
		s.logger.ErrorContext(ctx, "failed to reject batch",
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

	s.logger.InfoContext(ctx, "batch rejected", slog.String("batch_id", batch.ID.String()), slog.String("reason", reason))

	s.requeueOneTimePayout(ctx, batch.FundID, "rejected")

	return batch, nil
}

// requeueOneTimePayout puts a one-off fund's payout back on the schedule after
// its batch came to nothing.
//
// The planner clears next_payment as soon as a batch exists, before any money
// moves, so a rejected or expired batch otherwise leaves a 'once' fund with no
// anchor -- unplannable, and readable by the expiry job as a payout already
// dealt with, which closes the fund on its balance.
//
// Not fatal, and deliberately after the status change: the rejection is the
// decision and it has been recorded. A failure here leaves the fund needing a
// hand, which is what the error line is for.
func (s PayoutService) requeueOneTimePayout(ctx context.Context, fundID uuid.UUID, why string) {
	requeued, err := s.payoutStore.RequeueOneTimeFundPayout(ctx, fundID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to requeue a one-time fund's payout",
			slog.String("error", err.Error()),
			slog.String("fund_id", fundID.String()),
			slog.String("reason", why),
		)

		return
	}

	if requeued {
		s.logger.InfoContext(ctx, "one-time fund payout requeued",
			slog.String("fund_id", fundID.String()),
			slog.String("reason", why),
		)
	}
}

// SubmitBatch sends an approved batch to the provider. The sender batch ID is
// already persisted, so a submission that times out can be retried: the provider
// rejects the duplicate rather than paying twice.
func (s PayoutService) SubmitBatch(ctx context.Context, batchID uuid.UUID) (*Batch, error) {
	logger := s.logger.With(slog.String("batch_id", batchID.String()))

	batch, err := s.payoutStore.GetBatchByID(ctx, batchID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get batch", slog.String("error", err.Error()))

		return nil, err
	}

	if batch.Status != StatusReady {
		return nil, fmt.Errorf("%w: status is %q", ErrNotSubmittable, batch.Status)
	}

	items, err := s.payoutStore.GetPayoutsForBatch(ctx, batch.ID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get payouts for batch", slog.String("error", err.Error()))

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
		logger.ErrorContext(ctx, "failed to submit batch to provider", slog.String("error", err.Error()))

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
		logger.ErrorContext(ctx, "failed to record batch submission",
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

	logger.InfoContext(ctx, "batch submitted",
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
		s.logger.ErrorContext(ctx, "failed to cancel expired batches", slog.String("error", err.Error()))

		return err
	}

	for _, batch := range expired {
		// No actor: nobody decided this, which is the whole reason it is worth
		// recording. A batch that quietly stopped existing between one glance at
		// the admin page and the next is exactly what the feed is for.
		s.events.Record(ctx, fundevents.Record{
			FundID:      batch.FundID,
			Kind:        fundevents.KindBatchExpired,
			AmountCents: &batch.AmountCents,
			Detail:      "approval window expired",
			ReferenceID: &batch.ID,
		})

		s.logger.WarnContext(ctx, "batch cancelled: approval window expired",
			slog.String("batch_id", batch.ID.String()),
			slog.String("fund_id", batch.FundID.String()),
			slog.Int("amount_cents", int(batch.AmountCents)),
		)

		s.requeueOneTimePayout(ctx, batch.FundID, "approval window expired")
	}

	needReminder, err := s.payoutStore.GetBatchesNeedingReminder(ctx, s.reminderWindow)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get batches needing reminder", slog.String("error", err.Error()))

		return err
	}

	for _, batch := range needReminder {
		if s.notifier != nil {
			if errNotify := s.notifier.NotifyApprovalExpiring(ctx, batch); errNotify != nil {
				s.logger.ErrorContext(ctx, "failed to send expiry reminder",
					slog.String("error", errNotify.Error()),
					slog.String("batch_id", batch.ID.String()),
				)

				// Leave reminder_sent_at unset so the next sweep tries again.
				continue
			}
		}

		_, errMark := s.payoutStore.MarkReminderSent(ctx, batch.ID)
		if errMark != nil {
			s.logger.ErrorContext(ctx, "failed to mark reminder sent",
				slog.String("error", errMark.Error()),
				slog.String("batch_id", batch.ID.String()),
			)
		}
	}

	s.logger.InfoContext(ctx, "approval sweep complete",
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
		logger.ErrorContext(ctx, "failed to get batch", slog.String("error", err.Error()))

		return err
	}

	if batch.ProviderBatchID == "" {
		logger.InfoContext(ctx, "batch has not been submitted, nothing to reconcile")

		return nil
	}

	result, err := s.provider.GetBatchStatus(ctx, batch.ProviderBatchID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get batch status from provider", slog.String("error", err.Error()))

		return err
	}

	for _, item := range result.Items {
		if item.PayoutID == uuid.Nil {
			// sender_item_id did not round-trip as one of our payout IDs. Refusing to
			// guess: a mis-attributed status here would mark the wrong person paid.
			logger.ErrorContext(ctx, "provider item could not be matched to a payout",
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
			logger.ErrorContext(ctx, "failed to update payout result",
				slog.String("error", errItem.Error()),
				slog.String("payout_id", item.PayoutID.String()),
			)
		}
	}

	items, err := s.payoutStore.GetPayoutsForBatch(ctx, batch.ID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to read back payouts for batch", slog.String("error", err.Error()))

		return err
	}

	settled := batchStatusFrom(ProviderStatusToStatus(result.Status), items)

	_, err = s.payoutStore.SetBatchStatus(ctx, SetBatchStatus{
		BatchID: batch.ID,
		Status:  settled,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to update batch status", slog.String("error", err.Error()))

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

// batchStatusFrom decides a batch's status from what happened to its items, not
// from what the provider says about the batch.
//
// PayPal reports SUCCESS on a batch it has accepted and processed, which is not
// the same as everyone having been paid: a batch of one UNCLAIMED item reports
// SUCCESS while the money sits undelivered and auto-returns after 30 days.
// Taking the provider's word produced a batch reading "paid" that had paid
// nobody, which is the one thing this page must not get wrong.
//
// The provider's own verdict still wins when it rejected the batch outright,
// since then there are no item outcomes to reason about.
func batchStatusFrom(providerStatus Status, items []Payout) Status {
	switch providerStatus {
	case StatusFailed, StatusCancelled, StatusBlocked:
		return providerStatus
	}

	if len(items) == 0 {
		return providerStatus
	}

	allPaid := true
	anyUnresolved := false

	for _, item := range items {
		if item.Status != StatusPaid {
			allPaid = false
		}

		if !item.Status.Terminal() {
			anyUnresolved = true
		}
	}

	switch {
	case anyUnresolved:
		// Something is still in flight, including anything unclaimed -- money
		// nobody has taken has not finished moving.
		return StatusPending
	case allPaid:
		return StatusPaid
	default:
		// Everything has stopped, and not all of it arrived.
		return StatusFailed
	}
}

func (s PayoutService) GetBatchesForFund(ctx context.Context, fundID uuid.UUID) ([]Batch, error) {
	return s.payoutStore.GetBatchesForFund(ctx, fundID)
}

func (s PayoutService) GetBatchesAwaitingApproval(ctx context.Context) ([]Batch, error) {
	return s.payoutStore.GetBatchesByStatus(ctx, StatusAwaitingApproval)
}

// GetDetailedBatchesAwaitingApproval is for the approval page. The jobs use
// GetBatchesAwaitingApproval, which does not pay for a join to every payee.
func (s PayoutService) GetDetailedBatchesAwaitingApproval(ctx context.Context) ([]BatchDetail, error) {
	return s.payoutStore.GetDetailedBatchesByStatus(ctx, StatusAwaitingApproval)
}

func (s PayoutService) GetBatchByID(ctx context.Context, id uuid.UUID) (*Batch, error) {
	return s.payoutStore.GetBatchByID(ctx, id)
}

func (s PayoutService) GetPayoutsForBatch(ctx context.Context, batchID uuid.UUID) ([]Payout, error) {
	return s.payoutStore.GetPayoutsForBatch(ctx, batchID)
}

// PlanResult is what one run of the planner did, for the caller to print.
type PlanResult struct {
	Planned int
	Skipped int
}

// PlanDueBatches builds a batch for every fund whose payout date has arrived.
//
// The amount is the fund's available balance divided evenly among eligible
// enrollees, floored: the remainder stays in the fund and rolls into the next
// payout rather than being handed to whoever sorts first. Nothing here decides
// to send money -- every batch lands awaiting a treasurer, which is the point of
// planning being separate from submitting.
//
// One fund's failure never stops the others. A cron that abandons the remaining
// funds because the first had no PayPal address on file would turn a single
// misconfigured member into a fund-wide outage.
func (s PayoutService) PlanDueBatches(ctx context.Context) (PlanResult, error) {
	var result PlanResult

	due, err := s.payoutStore.GetFundsDueForPayout(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get funds due for payout", slog.String("error", err.Error()))

		return result, err
	}

	for _, fund := range due {
		logger := s.logger.With(
			slog.String("fund_id", fund.ID.String()),
			slog.String("fund", fund.Name),
		)

		planned, errPlan := s.planOneDueFund(ctx, fund, logger)
		if errPlan != nil {
			// Logged where it happened, with the fund named. Counted as skipped so
			// the run's summary line does not claim a clean sweep.
			result.Skipped++

			continue
		}

		if !planned {
			result.Skipped++

			continue
		}

		result.Planned++
	}

	s.logger.InfoContext(ctx, "payout planning complete",
		slog.Int("funds_due", len(due)),
		slog.Int("planned", result.Planned),
		slog.Int("skipped", result.Skipped),
	)

	return result, nil
}

// planOneDueFund returns whether a batch was created. The error is for the
// caller's tally only -- it is logged here, where the fund is still in scope.
func (s PayoutService) planOneDueFund(ctx context.Context, fund DueFund, logger *slog.Logger) (bool, error) {
	enrollments, err := s.payoutStore.GetEnrollmentsForPayout(ctx, fund.ID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get enrollments for payout", slog.String("error", err.Error()))

		return false, err
	}

	payable := 0
	for _, enrollment := range enrollments {
		if enrollment.PaypalEmail != "" {
			payable++
		}
	}

	if payable == 0 {
		// Nobody to pay, and no amount of waiting changes that for this period.
		// Advanced so a fund with no enrollees does not report itself due every
		// day forever.
		logger.WarnContext(ctx, "fund is due but has no payable enrollees, skipping period")

		if errAdvance := s.payoutStore.AdvanceFundNextPayment(ctx, fund.ID); errAdvance != nil {
			logger.ErrorContext(ctx, "failed to advance next payment", slog.String("error", errAdvance.Error()))

			return false, errAdvance
		}

		return false, nil
	}

	available, err := s.payoutStore.GetFundBalanceCents(ctx, fund.ID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get fund balance", slog.String("error", err.Error()))

		return false, err
	}

	// Held back for what sending the money will cost. The fee on a payout is only
	// known once it has been sent, so the balance cannot already have it
	// subtracted -- planning the whole balance means submitting a batch the account
	// cannot cover, and PayPal refuses the lot rather than the last item.
	//
	// Reserved high rather than exactly: what is not spent stays in the fund and
	// rolls into the next payout, which is where a remainder goes anyway.
	reserved := PayoutFeeCents * int64(payable)

	perHead := (available - reserved) / int64(payable)
	if perHead <= 0 {
		// Deliberately not advanced: the payout is still owed, and donations may
		// arrive tomorrow. Retrying is the right behaviour even though it means
		// this warning repeats until the fund is funded or deactivated.
		logger.WarnContext(ctx, "fund is due but has nothing to pay out, will retry",
			slog.Int64("available_cents", available),
			slog.Int64("reserved_for_fees_cents", reserved),
			slog.Int("payable", payable),
		)

		return false, nil
	}

	// int32 to match the column. perHead cannot exceed available, and available is
	// bounded by what the fund actually holds.
	batch, err := s.PlanBatch(ctx, PlanBatch{
		FundID: fund.ID,
		// The scheduled date, not today. A run that catches up a missed period must
		// record the date it is paying for, and the (fund_id, payout_date) unique
		// index is what stops a second run paying it twice.
		PayoutDate:  fund.NextPayment,
		AmountCents: int32(perHead),
		Description: fmt.Sprintf("%s payout", fund.Name),
		Notes: fmt.Sprintf("planned automatically: %d cents available, %d reserved for fees, %d payees",
			available, reserved, payable),
		RequireApproval: true,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to plan batch", slog.String("error", err.Error()))

		return false, err
	}

	// Only after the batch exists. Advancing first would lose the period entirely
	// if planning then failed.
	if err = s.payoutStore.AdvanceFundNextPayment(ctx, fund.ID); err != nil {
		// The batch is real and awaiting approval, so this is not fatal -- but the
		// fund still reads as due, and tomorrow's run will hit the unique index on
		// (fund_id, payout_date) rather than double-pay.
		logger.ErrorContext(ctx, "batch planned but failed to advance next payment",
			slog.String("error", err.Error()),
			slog.String("batch_id", batch.ID.String()),
		)
	}

	logger.InfoContext(ctx, "planned batch for due fund",
		slog.String("batch_id", batch.ID.String()),
		slog.Int64("per_head_cents", perHead),
		slog.Int("payees", payable),
		slog.Int64("reserved_for_fees_cents", reserved),
		slog.Int64("remainder_cents", available-reserved-perHead*int64(payable)),
	)

	return true, nil
}

// SubmitApprovedBatches sends every approved batch whose payout date has arrived.
//
// The date guard matters: `plan` accepts an arbitrary --date, so an approved
// batch can exist for a date still in the future, and approval is permission to
// pay on that date rather than permission to pay now.
func (s PayoutService) SubmitApprovedBatches(ctx context.Context) (int, error) {
	// Shared with the dry run rather than re-filtered here: a preview that selects
	// by different rules than the send is worse than no preview, because it is
	// trusted.
	due, err := s.GetBatchesReadyToSubmit(ctx)
	if err != nil {
		return 0, err
	}

	submitted := 0
	for _, batch := range due {
		if _, errSubmit := s.SubmitBatch(ctx, batch.ID); errSubmit != nil {
			// Left in 'ready' for the next run. SubmitBatch reuses the batch's
			// sender_batch_id, which the provider treats as an idempotency key, so
			// a retry after an ambiguous failure cannot pay twice.
			s.logger.ErrorContext(ctx, "failed to submit approved batch",
				slog.String("error", errSubmit.Error()),
				slog.String("batch_id", batch.ID.String()),
			)

			continue
		}

		submitted++
	}

	s.logger.InfoContext(ctx, "submitted approved batches",
		slog.Int("due", len(due)),
		slog.Int("submitted", submitted),
	)

	return submitted, nil
}

// ReconcilePendingBatches polls the provider for every batch still in flight.
//
// Webhooks normally settle these, so on a healthy system this finds nothing to
// change. It exists for the dropped webhook, which otherwise leaves a payout
// showing 'pending' indefinitely with the money long since moved.
func (s PayoutService) ReconcilePendingBatches(ctx context.Context) (int, error) {
	pending, err := s.payoutStore.GetBatchesByStatus(ctx, StatusPending)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get pending batches", slog.String("error", err.Error()))

		return 0, err
	}

	reconciled := 0
	for _, batch := range pending {
		if errReconcile := s.ReconcileBatch(ctx, batch.ID); errReconcile != nil {
			s.logger.ErrorContext(ctx, "failed to reconcile batch",
				slog.String("error", errReconcile.Error()),
				slog.String("batch_id", batch.ID.String()),
			)

			continue
		}

		reconciled++
	}

	s.logger.InfoContext(ctx, "reconciled pending batches",
		slog.Int("pending", len(pending)),
		slog.Int("reconciled", reconciled),
	)

	return reconciled, nil
}

// GetBatchesReadyToSubmit returns approved batches whose payout date has arrived.
func (s PayoutService) GetBatchesReadyToSubmit(ctx context.Context) ([]Batch, error) {
	ready, err := s.payoutStore.GetBatchesByStatus(ctx, StatusReady)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get approved batches", slog.String("error", err.Error()))

		return nil, err
	}

	now := time.Now()

	due := make([]Batch, 0, len(ready))
	for _, batch := range ready {
		if batch.PayoutDate.After(now) {
			continue
		}

		due = append(due, batch)
	}

	return due, nil
}

// EnrollmentsInUnsentBatches is who a batch would still pay if it were submitted
// now, keyed by enrollment id.
//
// Removing a member from a fund sets fund_enrollment.active = false, which every
// later plan honours. It does nothing to a batch already planned: SubmitBatch
// reads the payout rows by batch id, and those froze the amount and the address
// when the batch was built. That is defensible -- a treasurer approved a
// particular set of payees and amounts, and dropping one silently would either
// strand their share or underpay everyone else -- but it is not what an admin
// clicking a remove control expects, and nothing said so.
//
// Only batches that have not reached the provider. Once one is submitted the
// money has gone and removing the member cannot affect it, so saying so would be
// noise on every row after every payout.
func (s PayoutService) EnrollmentsInUnsentBatches(ctx context.Context, fundID uuid.UUID) (map[uuid.UUID]bool, error) {
	ids, err := s.payoutStore.GetEnrollmentsInUnsentBatches(ctx, fundID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to read enrollments in unsent batches",
			slog.String("error", err.Error()),
			slog.String("fund_id", fundID.String()),
		)

		return nil, err
	}

	pending := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		pending[id] = true
	}

	return pending, nil
}
