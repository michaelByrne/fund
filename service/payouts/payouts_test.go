package payouts_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"boardfund/events"
	"boardfund/pg"
	"boardfund/service/payouts"
	payoutstore "boardfund/service/payouts/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider records what was submitted without touching the network.
type stubProvider struct {
	submitted     [][]payouts.ProviderPayoutItem
	senderBatchID []uuid.UUID
	batchID       string
	submitErr     error

	statusResult *payouts.ProviderBatchResult
}

func (p *stubProvider) SubmitBatch(_ context.Context, senderBatchID uuid.UUID, _ string, items []payouts.ProviderPayoutItem) (*payouts.ProviderBatchResult, error) {
	p.senderBatchID = append(p.senderBatchID, senderBatchID)
	p.submitted = append(p.submitted, items)

	if p.submitErr != nil {
		return nil, p.submitErr
	}

	return &payouts.ProviderBatchResult{ProviderBatchID: p.batchID, Status: "PENDING"}, nil
}

func (p *stubProvider) GetBatchStatus(_ context.Context, _ string) (*payouts.ProviderBatchResult, error) {
	return p.statusResult, nil
}

func newService(t *testing.T, pool *pgxpool.Pool, provider payouts.PayoutsProvider) *payouts.PayoutService {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	return payouts.NewPayoutService(payoutstore.NewPayoutStore(pool), provider, nil, 72*time.Hour, 24*time.Hour, logger)
}

// seedMember creates an active member. bco_name is unique per member because the
// member table constrains it.
func seedMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	memberID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO member (id, email, bco_name, active) VALUES ($1, $2, $3, true)`,
		memberID, memberID.String()+"@test.org", "bco-"+memberID.String(),
	)
	require.NoError(t, err)

	return memberID
}

func seedFund(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	fundID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, next_payment)
		 VALUES ($1, 'Test Fund', 'd', $2, 'paypal', 'monthly', now())`,
		fundID, fundID.String(),
	)
	require.NoError(t, err)

	return fundID
}

// seedEnrollment enrolls a member in a fund with a payout date already in the past,
// so they are eligible for the next batch.
func seedEnrollment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fundID, memberID uuid.UUID, paypalEmail string) {
	t.Helper()

	_, err := pool.Exec(ctx,
		`INSERT INTO fund_enrollment (id, fund_id, member_id, first_payout_date, paypal_email, active)
		 VALUES ($1, $2, $3, now() - INTERVAL '1 day', $4, true)`,
		uuid.New(), fundID, memberID, paypalEmail,
	)
	require.NoError(t, err)
}

// seedFundWithEnrollees creates a fund and n active enrollees eligible for payout,
// returning the fund ID.
func seedFundWithEnrollees(t *testing.T, ctx context.Context, pool *pgxpool.Pool, n int) uuid.UUID {
	t.Helper()

	fundID := seedFund(t, ctx, pool)

	for i := 0; i < n; i++ {
		memberID := seedMember(t, ctx, pool)
		seedEnrollment(t, ctx, pool, fundID, memberID, memberID.String()+"@paypal.test")
	}

	return fundID
}

func TestPayoutService(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	t.Run("planned batch awaits approval and is not submittable", func(t *testing.T) {
		provider := &stubProvider{batchID: "PAYPAL-BATCH-1"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 3)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          fundID,
			PayoutDate:      time.Now(),
			AmountCents:     2500,
			RequireApproval: true,
		})
		require.NoError(t, err)

		assert.Equal(t, payouts.StatusAwaitingApproval, batch.Status)
		assert.Equal(t, int32(3), batch.NumEnrollments)
		assert.Equal(t, int32(7500), batch.AmountCents)
		require.NotNil(t, batch.ApprovalDeadline)
		assert.Nil(t, batch.ApprovedBy)

		// The gate is real: submission must be refused before approval, and nothing
		// may reach the provider.
		_, err = svc.SubmitBatch(ctx, batch.ID)
		require.ErrorIs(t, err, payouts.ErrNotSubmittable)
		assert.Empty(t, provider.submitted)
	})

	t.Run("approval records the treasurer and enables submission", func(t *testing.T) {
		provider := &stubProvider{batchID: "PAYPAL-BATCH-2"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)
		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID:          fundID,
			PayoutDate:      time.Now(),
			AmountCents:     1000,
			RequireApproval: true,
		})
		require.NoError(t, err)

		approved, err := svc.ApproveBatch(ctx, batch.ID, approverID)
		require.NoError(t, err)

		assert.Equal(t, payouts.StatusReady, approved.Status)
		require.NotNil(t, approved.ApprovedBy)
		require.NotNil(t, approved.ApprovedAt)
		assert.Equal(t, approverID, *approved.ApprovedBy)

		submitted, err := svc.SubmitBatch(ctx, batch.ID)
		require.NoError(t, err)

		assert.Equal(t, payouts.StatusPending, submitted.Status)
		assert.Equal(t, "PAYPAL-BATCH-2", submitted.ProviderBatchID)

		// The idempotency key that reached the provider must be the one persisted, or
		// a retry would not deduplicate.
		require.Len(t, provider.senderBatchID, 1)
		assert.Equal(t, batch.SenderBatchID, provider.senderBatchID[0])
		assert.Len(t, provider.submitted[0], 2)
	})

	t.Run("a batch cannot be submitted twice", func(t *testing.T) {
		provider := &stubProvider{batchID: "PAYPAL-BATCH-3"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 500, RequireApproval: true,
		})
		require.NoError(t, err)

		_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
		require.NoError(t, err)

		_, err = svc.SubmitBatch(ctx, batch.ID)
		require.NoError(t, err)

		// This is the double-pay guard. Once submitted the batch leaves 'ready', so a
		// second submit must refuse and must not reach the provider again.
		_, err = svc.SubmitBatch(ctx, batch.ID)
		require.ErrorIs(t, err, payouts.ErrNotSubmittable)
		assert.Len(t, provider.submitted, 1)
	})

	t.Run("expired batches are cancelled by the sweep and cannot then be approved", func(t *testing.T) {
		provider := &stubProvider{batchID: "PAYPAL-BATCH-4"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 500, RequireApproval: true,
		})
		require.NoError(t, err)

		// Reach past the deadline without waiting three days.
		_, err = pool.Exec(ctx,
			`UPDATE batch_payout SET approval_deadline = now() - INTERVAL '1 minute' WHERE id = $1`,
			batch.ID,
		)
		require.NoError(t, err)

		require.NoError(t, svc.RunApprovalSweep(ctx))

		swept, err := svc.GetBatchByID(ctx, batch.ID)
		require.NoError(t, err)
		assert.Equal(t, payouts.StatusCancelled, swept.Status)
		assert.Equal(t, "approval window expired", swept.FailureReason)

		// A treasurer arriving late must not be able to revive a cancelled batch.
		_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
		require.ErrorIs(t, err, payouts.ErrNotApprovable)

		still, err := svc.GetBatchByID(ctx, batch.ID)
		require.NoError(t, err)
		assert.Equal(t, payouts.StatusCancelled, still.Status)
	})

	t.Run("sweep leaves batches inside the window alone", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{})

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 500, RequireApproval: true,
		})
		require.NoError(t, err)

		require.NoError(t, svc.RunApprovalSweep(ctx))

		unchanged, err := svc.GetBatchByID(ctx, batch.ID)
		require.NoError(t, err)
		assert.Equal(t, payouts.StatusAwaitingApproval, unchanged.Status)
	})

	t.Run("no-approval batches are ready immediately", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{batchID: "PAYPAL-BATCH-5"})

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 500, RequireApproval: false,
		})
		require.NoError(t, err)

		assert.Equal(t, payouts.StatusReady, batch.Status)
		assert.Nil(t, batch.ApprovalDeadline)
	})

	t.Run("enrollees without a paypal email are skipped, not fatal", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{})

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)

		// An enrollee with no PayPal address: nowhere to send their share.
		seedEnrollment(t, ctx, pool, fundID, seedMember(t, ctx, pool), "")

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 1000, RequireApproval: true,
		})
		require.NoError(t, err)

		// Three enrolled, one unpayable: the batch covers the other two rather than
		// failing outright.
		assert.Equal(t, int32(2), batch.NumEnrollments)
		assert.Equal(t, int32(2000), batch.AmountCents)
	})

	t.Run("a fund with no payable enrollees yields no batch", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{})

		fundID := seedFundWithEnrollees(t, ctx, pool, 0)

		_, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 1000, RequireApproval: true,
		})
		require.ErrorIs(t, err, payouts.ErrNoEnrollments)
	})

	t.Run("a failed submission leaves the batch approved and retryable", func(t *testing.T) {
		provider := &stubProvider{submitErr: assert.AnError}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)

		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 500, RequireApproval: true,
		})
		require.NoError(t, err)

		_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
		require.NoError(t, err)

		_, err = svc.SubmitBatch(ctx, batch.ID)
		require.Error(t, err)

		// Crucially not marked failed: the request may have reached the provider, and
		// a failed batch would be re-planned and paid a second time.
		after, err := svc.GetBatchByID(ctx, batch.ID)
		require.NoError(t, err)
		assert.Equal(t, payouts.StatusReady, after.Status)
	})

	t.Run("reconcile writes per-item status back through sender_item_id", func(t *testing.T) {
		provider := &stubProvider{batchID: "PAYPAL-BATCH-6"}
		svc := newService(t, pool, provider)

		fundID := seedFundWithEnrollees(t, ctx, pool, 2)

		approverID := seedMember(t, ctx, pool)

		batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: time.Now(), AmountCents: 1000, RequireApproval: true,
		})
		require.NoError(t, err)

		_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
		require.NoError(t, err)

		_, err = svc.SubmitBatch(ctx, batch.ID)
		require.NoError(t, err)

		items, err := svc.GetPayoutsForBatch(ctx, batch.ID)
		require.NoError(t, err)
		require.Len(t, items, 2)

		// One paid, one unclaimed: the case the original four-value enum could not
		// represent.
		provider.statusResult = &payouts.ProviderBatchResult{
			ProviderBatchID: "PAYPAL-BATCH-6",
			Status:          "SUCCESS",
			Items: []payouts.ProviderItemResult{
				{PayoutID: items[0].ID, ProviderPayoutItemID: "ITEM-1", Status: "SUCCESS", FeeCents: 25},
				{PayoutID: items[1].ID, ProviderPayoutItemID: "ITEM-2", Status: "UNCLAIMED"},
			},
		}

		require.NoError(t, svc.ReconcileBatch(ctx, batch.ID))

		settled, err := svc.GetPayoutsForBatch(ctx, batch.ID)
		require.NoError(t, err)

		byID := map[uuid.UUID]payouts.Payout{}
		for _, item := range settled {
			byID[item.ID] = item
		}

		assert.Equal(t, payouts.StatusPaid, byID[items[0].ID].Status)
		assert.Equal(t, "ITEM-1", byID[items[0].ID].ProviderPayoutItemID)
		assert.Equal(t, int32(25), byID[items[0].ID].ProviderFeeCents)

		assert.Equal(t, payouts.StatusUnclaimed, byID[items[1].ID].Status)
		assert.False(t, payouts.StatusUnclaimed.Terminal(), "unclaimed must not be terminal; PayPal auto-returns it")
	})

	t.Run("the database rejects a second batch for the same fund and date", func(t *testing.T) {
		svc := newService(t, pool, &stubProvider{})

		fundID := seedFundWithEnrollees(t, ctx, pool, 1)
		date := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)

		_, err := svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: date, AmountCents: 500, RequireApproval: true,
		})
		require.NoError(t, err)

		// A scheduler that fires twice must not be able to pay everyone again.
		_, err = svc.PlanBatch(ctx, payouts.PlanBatch{
			FundID: fundID, PayoutDate: date, AmountCents: 500, RequireApproval: true,
		})
		require.Error(t, err)
	})
}

func TestProviderStatusToStatus(t *testing.T) {
	cases := map[string]payouts.Status{
		"SUCCESS":               payouts.StatusPaid,
		"success":               payouts.StatusPaid,
		"FAILED":                payouts.StatusFailed,
		"DENIED":                payouts.StatusFailed,
		"PENDING":               payouts.StatusPending,
		"PROCESSING":            payouts.StatusPending,
		"UNCLAIMED":             payouts.StatusUnclaimed,
		"RETURNED":              payouts.StatusReturned,
		"REVERSED":              payouts.StatusReturned,
		"ONHOLD":                payouts.StatusOnhold,
		"BLOCKED":               payouts.StatusBlocked,
		"CANCELED":              payouts.StatusCancelled,
		"DENIED_FOR_COMPLIANCE": payouts.StatusPending,
		"":                      payouts.StatusPending,
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, want, payouts.ProviderStatusToStatus(input))
		})
	}
}

// captureSubscriber records the callbacks Handlers registers, so a test can drive
// a webhook payload through the real subscription wiring.
type captureSubscriber struct {
	handlers map[string]func([]byte)
}

func (c *captureSubscriber) Subscribe(event string, cb func(data []byte)) error {
	if c.handlers == nil {
		c.handlers = map[string]func([]byte){}
	}

	c.handlers[event] = cb

	return nil
}

// A payout item webhook typically arrives before anything has reconciled, so
// provider_payout_item_id is still NULL. Matching on it would update no rows and
// silently drop the outcome; sender_item_id carries our own payout ID and is
// available immediately.
func TestPayoutItemWebhookMatchesBeforeReconcile(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := payoutstore.NewPayoutStore(pool)
	svc := payouts.NewPayoutService(store, &stubProvider{batchID: "PAYPAL-WEBHOOK"}, nil, 72*time.Hour, 24*time.Hour, logger)

	fundID := seedFundWithEnrollees(t, ctx, pool, 1)
	approverID := seedMember(t, ctx, pool)

	batch, err := svc.PlanBatch(ctx, payouts.PlanBatch{
		FundID: fundID, PayoutDate: time.Now(), AmountCents: 1000, RequireApproval: true,
	})
	require.NoError(t, err)

	_, err = svc.ApproveBatch(ctx, batch.ID, approverID)
	require.NoError(t, err)

	_, err = svc.SubmitBatch(ctx, batch.ID)
	require.NoError(t, err)

	items, err := svc.GetPayoutsForBatch(ctx, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Nothing has reconciled, so the provider's item ID is not on the row yet.
	require.Empty(t, items[0].ProviderPayoutItemID)

	sub := &captureSubscriber{}
	handlers := payouts.NewHandlers(store, logger)
	require.NoError(t, handlers.Subscribe(sub))

	cb := sub.handlers[events.PayoutsItemSucceeded]
	require.NotNil(t, cb, "handler must subscribe to PAYMENT.PAYOUTS-ITEM.SUCCEEDED")

	payload := fmt.Sprintf(`{
		"payout_item_id": "ITEM-FROM-WEBHOOK",
		"transaction_status": "SUCCESS",
		"payout_item_fee": {"currency": "USD", "value": "0.25"},
		"payout_item": {"sender_item_id": %q}
	}`, items[0].ID.String())

	cb([]byte(payload))

	settled, err := svc.GetPayoutsForBatch(ctx, batch.ID)
	require.NoError(t, err)
	require.Len(t, settled, 1)

	assert.Equal(t, payouts.StatusPaid, settled[0].Status)
	assert.Equal(t, int32(25), settled[0].ProviderFeeCents)

	// The provider's ID is recorded in the same statement, so a later reconcile or
	// a subsequent webhook can both find the row either way.
	assert.Equal(t, "ITEM-FROM-WEBHOOK", settled[0].ProviderPayoutItemID)
}
