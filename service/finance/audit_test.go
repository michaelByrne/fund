package finance

import (
	"testing"
	"time"
)

// The old page compared our amount to the provider's and flagged anything that
// differed. For a payment nobody had reconciled the provider's amount was zero,
// so every recent payment was flagged -- and a page where everything is red says
// nothing. Three states, not two.
func TestAuditVerdict(t *testing.T) {
	checked := time.Now()

	cases := []struct {
		name    string
		payment AuditPayment
		want    AuditVerdict
		attend  bool
	}{
		{
			name:    "never reconciled",
			payment: AuditPayment{AmountCents: 1000},
			want:    AuditUnchecked,
			// The job has not got there. Not a fault, and not a clean bill either.
			attend: false,
		},
		{
			name: "agrees with the provider",
			payment: AuditPayment{
				AmountCents: 1000, ProviderAmountCents: 1000,
				ProviderStatus: "COMPLETED", ReconciledAt: &checked,
			},
			want:   AuditOK,
			attend: false,
		},
		{
			name: "provider took a different amount",
			payment: AuditPayment{
				AmountCents: 1000, ProviderAmountCents: 900,
				ProviderStatus: "COMPLETED", ReconciledAt: &checked,
			},
			// The one finding this job produces that nothing else would.
			want:   AuditAmountMismatch,
			attend: true,
		},
		{
			name: "provider has it but not settled",
			payment: AuditPayment{
				AmountCents: 1000, ProviderAmountCents: 1000,
				ProviderStatus: "PENDING", ReconciledAt: &checked,
			},
			want:   AuditNotSettled,
			attend: true,
		},
		{
			name: "checked, and the provider had nothing",
			payment: AuditPayment{
				AmountCents: 1000, ReconciledAt: &checked,
			},
			// Reporting lags by hours, so a recent payment is routinely unknown to
			// it. Distinct from never checked, and not worth waking anybody for.
			want:   AuditMissingAtProvider,
			attend: false,
		},
		{
			name: "status casing does not decide it",
			payment: AuditPayment{
				AmountCents: 1000, ProviderAmountCents: 1000,
				ProviderStatus: "completed", ReconciledAt: &checked,
			},
			want:   AuditOK,
			attend: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.payment.Verdict(); got != c.want {
				t.Errorf("Verdict() = %q, want %q", got, c.want)
			}

			if got := c.payment.NeedsAttention(); got != c.attend {
				t.Errorf("NeedsAttention() = %v, want %v", got, c.attend)
			}
		})
	}
}

// An unreconciled payment must not read as a problem, which is what the page did
// for every payment taken since the last run.
func TestUncheckedIsNotAProblem(t *testing.T) {
	fresh := AuditPayment{AmountCents: 2500}

	if fresh.NeedsAttention() {
		t.Error("a payment the job has not reached yet should not be flagged")
	}

	if fresh.Verdict() == AuditOK {
		t.Error("nor should it be reported as agreeing with the provider")
	}
}
