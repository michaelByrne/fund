package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/messaging"
	"boardfund/service/finance"
	"boardfund/service/members"

	"github.com/google/uuid"
)

func renderWebhooks(t *testing.T, status messaging.Status) string {
	t.Helper()

	var out strings.Builder
	member := members.Member{ID: uuid.New(), BCOName: "michael"}

	if err := Webhooks(status, &member, "/admin/webhooks").Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

// Retried is zero on a healthy consumer, so it is the number an operator should
// notice without reading the row. Rendered like every other figure it is just
// another column.
func TestRetriedCountIsCalledOut(t *testing.T) {
	stuck := renderWebhooks(t, messaging.Status{
		Consumers: []messaging.ConsumerStatus{
			{Subject: "PAYMENT.SALE.COMPLETED", Pending: 3, AckPending: 1, Redelivered: 7},
		},
	})

	if !strings.Contains(stuck, "PAYMENT.SALE.COMPLETED") {
		t.Error("the consumer should be listed by its event type, not its durable name")
	}

	if !strings.Contains(stuck, "text-red-600") {
		t.Error("a non-zero retry count should stand out")
	}

	healthy := renderWebhooks(t, messaging.Status{
		Consumers: []messaging.ConsumerStatus{
			{Subject: "PAYMENT.SALE.COMPLETED", Redelivered: 0},
		},
	})

	if strings.Contains(healthy, "text-red-600") {
		t.Error("a healthy consumer should not be flagged, or the flag means nothing")
	}
}

// No consumers means webhooks are arriving and nothing is acting on them, which
// is a fault that an empty table would render as blank space.
func TestNoConsumersSaysSo(t *testing.T) {
	html := renderWebhooks(t, messaging.Status{})

	if !strings.Contains(html, "no consumers are attached") {
		t.Error("an empty consumer list should say what it means")
	}
}

// The exhausted list lives in memory. An empty table after a restart would read
// as an all clear when it is really an absence of evidence.
func TestExhaustedListAdmitsItIsCleared(t *testing.T) {
	html := renderWebhooks(t, messaging.Status{
		Exhausted: []messaging.Exhausted{
			{Consumer: "PAYMENT_SALE_COMPLETED", StreamSeq: 42, Deliveries: 5, At: time.Now()},
		},
	})

	if !strings.Contains(html, "42") {
		t.Error("the stream sequence is how the message is found, so it has to be shown")
	}

	if !strings.Contains(html, "cleared when the service restarts") {
		t.Error("the page should say the list does not survive a restart")
	}
}

// An empty stream has a zero first-message time, which formats as a date in the
// year one and reads as corruption.
func TestAnEmptyStreamDoesNotRenderTheZeroTime(t *testing.T) {
	html := renderWebhooks(t, messaging.Status{
		Stream: messaging.StreamStatus{Name: "WEBHOOKS"},
	})

	if strings.Contains(html, "0001") {
		t.Error("the zero time should render as a dash, not as a date")
	}
}

// humanBytes indexed a four-character suffix string with an exponent that keeps
// climbing, so a stream at a petabyte panicked while rendering the page meant to
// show you the stream.
func TestHumanBytesDoesNotRunOffTheEndOfItsSuffixes(t *testing.T) {
	const unit = 1024

	cases := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{unit, "1.0 KB"},
		{unit * unit, "1.0 MB"},
		{unit * unit * unit, "1.0 GB"},
		{unit * unit * unit * unit, "1.0 TB"},
		// The old code panicked from here on.
		{unit * unit * unit * unit * unit, "1.0 PB"},
		{unit * unit * unit * unit * unit * unit, "1.0 EB"},
		// Past the last suffix the exponent clamps rather than indexing past it.
		{^uint64(0), "16.0 EB"},
	}

	for _, c := range cases {
		if got := humanBytes(c.bytes); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

// blue-boxy-filter is filter: drop-shadow, which shadows the shape of everything
// inside it. A heading or a table has a background and casts a box, which is the
// point. Bare text casts a copy of itself four pixels down and right, and reads
// on screen as the sentence printed twice.
func TestAuditCaptionSitsOutsideTheShadowedBox(t *testing.T) {
	var out strings.Builder

	audit := finance.Audit{
		FundID: uuid.New(), FundName: "human fund", Date: time.Now(),
		Payments: []finance.AuditPayment{{
			PaymentID: uuid.New(), DonorName: "michael", AmountCents: 500,
		}},
	}

	member := members.Member{ID: uuid.New(), BCOName: "michael"}

	if err := FundPaymentsAudit(audit, &member, "/admin/fund/audit").
		Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	html := out.String()

	caption := strings.Index(html, "checked against PayPal nightly")
	if caption < 0 {
		t.Fatal("the caption should explain what unchecked means")
	}

	// The caption has to come before the shadowed container opens, or the filter
	// applies to it.
	box := strings.Index(html, "blue-boxy-filter")
	for box >= 0 && box < caption {
		// The heading also carries the filter and legitimately precedes the
		// caption; look past it.
		next := strings.Index(html[box+1:], "blue-boxy-filter")
		if next < 0 {
			box = -1

			break
		}

		box += next + 1
	}

	if box >= 0 && box < caption {
		t.Error("the caption is inside a drop-shadowed container and will render doubled")
	}
}
