package payouts

import "testing"

// A batch of one UNCLAIMED item came back from PayPal as SUCCESS, and the fund
// page said "paid" about a dollar nobody had received. The batch's status has to
// follow the money, not the provider's report on its own processing.
func TestBatchStatusFrom(t *testing.T) {
	item := func(s Status) Payout { return Payout{Status: s} }

	cases := []struct {
		name     string
		provider Status
		items    []Payout
		want     Status
	}{
		{
			name:     "the case found in sandbox: provider says success, money is unclaimed",
			provider: StatusPaid,
			items:    []Payout{item(StatusUnclaimed)},
			want:     StatusPending,
		},
		{
			name:     "everyone paid",
			provider: StatusPaid,
			items:    []Payout{item(StatusPaid), item(StatusPaid)},
			want:     StatusPaid,
		},
		{
			name:     "one still pending holds the batch open",
			provider: StatusPaid,
			items:    []Payout{item(StatusPaid), item(StatusPending)},
			want:     StatusPending,
		},
		{
			name:     "on hold is not resolved either",
			provider: StatusPaid,
			items:    []Payout{item(StatusOnhold)},
			want:     StatusPending,
		},
		{
			name:     "everything stopped and some did not arrive",
			provider: StatusPaid,
			items:    []Payout{item(StatusPaid), item(StatusReturned)},
			want:     StatusFailed,
		},
		{
			name:     "all returned",
			provider: StatusPaid,
			items:    []Payout{item(StatusReturned)},
			want:     StatusFailed,
		},
		{
			// The provider rejecting the whole batch wins: there are no item
			// outcomes to reason about.
			name:     "provider denied the batch",
			provider: StatusFailed,
			items:    []Payout{item(StatusPlanned)},
			want:     StatusFailed,
		},
		{
			name:     "provider cancelled the batch",
			provider: StatusCancelled,
			items:    nil,
			want:     StatusCancelled,
		},
		{
			name:     "no items yet, nothing to derive from",
			provider: StatusPending,
			items:    nil,
			want:     StatusPending,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := batchStatusFrom(c.provider, c.items); got != c.want {
				t.Errorf("batchStatusFrom(%q, %d items) = %q, want %q",
					c.provider, len(c.items), got, c.want)
			}
		})
	}
}
