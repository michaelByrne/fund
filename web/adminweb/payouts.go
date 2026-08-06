package adminweb

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"boardfund/service/enrollments"
	"boardfund/service/fundevents"
	"boardfund/service/members"

	"github.com/google/uuid"
)

func payoutAmount(cents int32) string {
	return "$" + centsToDecimalString(cents)
}

func remaining(deadline time.Time) string {
	return formatRemaining(time.Until(deadline))
}

// formatRemaining renders how long is left before an approval window closes, at the
// coarsest useful precision -- a treasurer deciding whether to act now does not need
// seconds.
//
// Minutes round up, so a window with any time left never reads "0m". Expired is a
// separate visual state, and the two must not look alike.
func formatRemaining(left time.Duration) string {
	if left <= 0 {
		return "0m"
	}

	totalMinutes := int((left + time.Minute - 1) / time.Minute)

	days := totalMinutes / (24 * 60)
	hours := (totalMinutes % (24 * 60)) / 60
	minutes := totalMinutes % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

func (h *AdminHandlers) payoutsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	batches, err := h.payoutService.GetBatchesAwaitingApproval(ctx)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, msgUnavailable)

		return
	}

	Payouts(batches, &member, r.URL.Path).Render(ctx, w)
}

func (h *AdminHandlers) payoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	batchID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badRequest(w, r, "that is not a valid batch id.")

		return
	}

	batch, err := h.payoutService.GetBatchByID(ctx, batchID)
	if err != nil {
		h.notFound(w, r)

		return
	}

	items, err := h.payoutService.GetPayoutsForBatch(ctx, batchID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	PayoutDetail(*batch, items, &member, "/admin/payouts").Render(ctx, w)
}

// approvePayout records the approval against the member in session. The service
// performs a compare-and-set on 'awaiting_approval', so a batch the sweep cancelled
// between page load and click is refused rather than revived -- the stale button is
// harmless.
func (h *AdminHandlers) approvePayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	member, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	batchID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badRequest(w, r, "that is not a valid batch id.")

		return
	}

	batch, err := h.payoutService.ApproveBatch(ctx, batchID, member.ID)
	if err != nil {
		// Most likely the batch is no longer awaiting approval. Re-render its
		// current state so the page tells the truth rather than showing an error.
		h.renderBatchActions(w, r, batchID)

		return
	}

	BatchActions(*batch).Render(ctx, w)
}

func (h *AdminHandlers) rejectPayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, ok := h.sessionManager.Get(ctx, "member").(members.Member)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)

		return
	}

	batchID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.badRequest(w, r, "that is not a valid batch id.")

		return
	}

	reason := r.FormValue("reason")

	batch, err := h.payoutService.RejectBatch(ctx, batchID, reason)
	if err != nil {
		h.renderBatchActions(w, r, batchID)

		return
	}

	BatchActions(*batch).Render(ctx, w)
}

// renderBatchActions re-renders a batch's action block from current state. Used when
// an action was refused, so the operator sees why rather than a dead button.
func (h *AdminHandlers) renderBatchActions(w http.ResponseWriter, r *http.Request, batchID uuid.UUID) {
	ctx := r.Context()

	batch, err := h.payoutService.GetBatchByID(ctx, batchID)
	if err != nil {
		h.internalError(w, r)

		return
	}

	BatchActions(*batch).Render(ctx, w)
}

// eventLabel renders an event kind as something a treasurer reads rather than a
// database enum.
func eventLabel(kind fundevents.Kind) string {
	switch kind {
	case fundevents.KindDonationStarted:
		return "donation started"
	case fundevents.KindDonationCancelled:
		return "donation cancelled"
	case fundevents.KindPaymentReceived:
		return "payment received"
	case fundevents.KindMemberEnrolled:
		return "member enrolled"
	case fundevents.KindEnrollmentCancelled:
		return "enrollment cancelled"
	case fundevents.KindBatchPlanned:
		return "payout batch planned"
	case fundevents.KindBatchApproved:
		return "payout batch approved"
	case fundevents.KindBatchRejected:
		return "payout batch rejected"
	case fundevents.KindBatchExpired:
		return "payout batch expired unapproved"
	case fundevents.KindBatchSubmitted:
		return "payout batch submitted"
	case fundevents.KindBatchSettled:
		return "payout batch settled"
	case fundevents.KindFundClosed:
		return "fund closed"
	default:
		// A kind added to the enum but not here still renders legibly rather
		// than as a blank row.
		return strings.ReplaceAll(string(kind), "_", " ")
	}
}

// payoutCoverage is how many of a fund's current enrollees a batch planned now
// would actually reach.
//
// Computed once and passed around rather than recalculated: the notice needs the
// count both to decide whether to appear and to say what it says, and deriving
// those separately meant two passes over the slice against two different clock
// reads, which could disagree across the eligibility boundary.
type payoutCoverage struct {
	Total     int
	Unpayable int
}

// Incomplete reports whether anyone would be left out.
func (c payoutCoverage) Incomplete() bool {
	return c.Unpayable > 0
}

func (c payoutCoverage) Summary() string {
	return fmt.Sprintf("a payout planned now would cover %d of %d enrollees",
		c.Total-c.Unpayable, c.Total)
}

// coverageOf counts who a batch would leave out: no PayPal address, or a first
// payout date still in the future. Both are silent exclusions in the payout
// path, which is why the list surfaces them.
func coverageOf(enrolled []enrollments.Enrollment) payoutCoverage {
	now := time.Now()

	coverage := payoutCoverage{Total: len(enrolled)}
	for _, enrollment := range enrolled {
		if !enrollment.Payable(now) {
			coverage.Unpayable++
		}
	}

	return coverage
}
