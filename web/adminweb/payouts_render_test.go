package adminweb

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"boardfund/service/members"
	"boardfund/service/payouts"

	"github.com/google/uuid"
)

// Templates compile whatever they dereference, so nil pointers only surface when
// something renders. Batch carries four optional pointers and the approval states
// differ structurally, so each variant is exercised here.
func TestPayoutTemplatesRender(t *testing.T) {
	ctx := context.Background()
	member := members.Member{ID: uuid.New(), BCOName: "treasurer"}

	approver := uuid.New()
	approvedAt := time.Now().Add(-time.Hour)
	future := time.Now().Add(48 * time.Hour)
	past := time.Now().Add(-time.Hour)

	batches := []payouts.BatchDetail{
		{
			Batch: payouts.Batch{
				ID: uuid.New(), FundID: uuid.New(), AmountCents: 7500, NumEnrollments: 3,
				Status: payouts.StatusAwaitingApproval, PayoutDate: time.Now(),
				ApprovalDeadline: &future,
			},
			FundName:   "human fund",
			PayeeNames: []string{"ada", "bo", "cyd"},
		},
		{
			// Expired: actions must render as status, not as live buttons.
			Batch: payouts.Batch{
				ID: uuid.New(), FundID: uuid.New(), AmountCents: 2500, NumEnrollments: 1,
				Status: payouts.StatusAwaitingApproval, PayoutDate: time.Now(),
				ApprovalDeadline: &past,
			},
			FundName:   "winter fund",
			PayeeNames: []string{"dee"},
		},
		{
			Batch: payouts.Batch{
				ID: uuid.New(), FundID: uuid.New(), AmountCents: 1000, NumEnrollments: 1,
				Status: payouts.StatusReady, PayoutDate: time.Now(),
				ApprovalDeadline: &future, ApprovedBy: &approver, ApprovedAt: &approvedAt,
			},
			FundName: "rent fund",
		},
		{
			// Every optional field nil, and a terminal status with a reason.
			Batch: payouts.Batch{
				ID: uuid.New(), FundID: uuid.New(), AmountCents: 500, NumEnrollments: 1,
				Status: payouts.StatusCancelled, PayoutDate: time.Now(),
				FailureReason: "approval window expired",
			},
			FundName: "board costs",
		},
	}

	var listed strings.Builder
	if err := Payouts(batches, &member, "/admin/payouts").Render(ctx, &listed); err != nil {
		t.Fatalf("Payouts render: %v", err)
	}

	out := listed.String()

	for _, want := range []string{"$75.00", "3 payees", "approve", "expired", "approval window expired"} {
		if !strings.Contains(out, want) {
			t.Errorf("batch list missing %q", want)
		}
	}

	// The expired batch must not offer an approve control.
	expiredID := batches[1].ID.String()
	if strings.Contains(out, "/admin/payout/approve/"+expiredID) {
		t.Error("expired batch rendered an approve button")
	}

	// The live one must.
	liveID := batches[0].ID.String()
	if !strings.Contains(out, "/admin/payout/approve/"+liveID) {
		t.Error("live batch did not render an approve button")
	}

	if err := Payouts(nil, &member, "/admin/payouts").Render(ctx, io.Discard); err != nil {
		t.Fatalf("empty batch list render: %v", err)
	}

	items := []payouts.Payout{
		{ID: uuid.New(), AmountCents: 2500, Status: payouts.StatusPaid, DestinationEmail: "a@test.org", ProviderFeeCents: 25},
		{ID: uuid.New(), AmountCents: 2500, Status: payouts.StatusUnclaimed, DestinationEmail: "b@test.org"},
		// Refunds report a negative fee; this is what broke the CLI formatter.
		{ID: uuid.New(), AmountCents: 2500, Status: payouts.StatusReturned, DestinationEmail: "c@test.org", ProviderFeeCents: -25},
	}

	for _, batch := range batches {
		var detail strings.Builder
		if err := PayoutDetail(batch.Batch, items, &member, "/admin/payouts").Render(ctx, &detail); err != nil {
			t.Fatalf("PayoutDetail render (status %s): %v", batch.Status, err)
		}

		if !strings.Contains(detail.String(), "a@test.org") {
			t.Errorf("detail for %s did not render its payouts", batch.Status)
		}
	}
}

func TestFormatRemaining(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{50 * time.Hour, "2d 2h"},
		{5*time.Hour + 12*time.Minute, "5h 12m"},
		{43 * time.Minute, "43m"},
		// Any time left must not read as expired.
		{30 * time.Second, "1m"},
		{time.Nanosecond, "1m"},
		{0, "0m"},
		{-time.Hour, "0m"},
	}

	for _, c := range cases {
		if got := formatRemaining(c.in); got != c.want {
			t.Errorf("formatRemaining(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
