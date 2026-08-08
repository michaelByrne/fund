package adminweb

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"boardfund/service/payouts"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func awaitingBatch(fundName string, names []string) payouts.BatchDetail {
	deadline := time.Now().Add(time.Hour)

	payees := make([]payouts.Payee, 0, len(names))
	for _, name := range names {
		payees = append(payees, payouts.Payee{ID: uuid.New(), Name: name})
	}

	return payouts.BatchDetail{
		Batch: payouts.Batch{
			ID: uuid.New(), FundID: uuid.New(), AmountCents: 4000,
			NumEnrollments: int32(len(names)), Status: payouts.StatusAwaitingApproval,
			PayoutDate: time.Now(), ApprovalDeadline: &deadline,
		},
		FundName: fundName,
		Payees:   payees,
	}
}

// "$40.00, 4 payees" does not say which fund is about to send money, and on a
// page listing several batches that was the whole question.
func TestABatchRowNamesItsFund(t *testing.T) {
	html := renderAdmin(t, BatchRow(awaitingBatch("human fund", []string{"ada", "bo"})))

	require.Contains(t, html, "human fund")
}

// The count cannot answer the question a treasurer has before approving: who is
// being paid.
func TestThePayeeCountOpensAPanelOfNames(t *testing.T) {
	batch := awaitingBatch("human fund", []string{"ada", "bo", "cyd"})

	// The control on its own. The row also carries a title on the fund name, for
	// truncation, and that is not what this is about.
	html := renderAdmin(t, PayeeCount(batch))

	require.Contains(t, html, "3 payees")

	for _, payee := range batch.Payees {
		require.Contains(t, html, ">"+payee.Name+"<", "every payee should be listed")
	}

	// Not a title. It is slow, unstylable, and on a touch screen there is no hover
	// at all -- the names could not be reached on a phone.
	require.NotContains(t, html, "title=")

	// Opens on hover. Asking for a click was the second complaint about this.
	require.Contains(t, html, "group-hover:block")

	// And on focus, which is what makes it reachable by keyboard and on a touch
	// screen, where a tap is a focus and there is no hover to give.
	require.Contains(t, html, "group-focus-within:block")
	require.Contains(t, html, `type="button"`)

	// Beside the count, not in the middle of the screen. The native popover put it
	// there, because placing one next to its button needs anchor positioning.
	require.Contains(t, html, "group relative")
	require.Contains(t, html, "absolute left-0 top-full")
	require.NotContains(t, html, "popovertarget")

	// And it looks like it does something.
	require.Contains(t, html, "decoration-dotted")
}

// Each row's panel belongs to that row. Positioned inside the row rather than
// addressed by an id, so there is no id to collide -- but each still has to hold
// its own names.
func TestEachBatchHasItsOwnPanel(t *testing.T) {
	first := awaitingBatch("human fund", []string{"ada"})
	second := awaitingBatch("winter fund", []string{"bo"})

	html := renderAdmin(t, BatchList([]payouts.BatchDetail{first, second}))

	require.Equal(t, 2, strings.Count(html, "group relative"))
	require.Contains(t, html, ">ada<")
	require.Contains(t, html, ">bo<")
}

// The list used to sit in a scroll box of its own. Anything positioned inside one
// is clipped by it, so the panel would be cut off at the edge -- or invisible
// entirely for the last row, which is where it would open downwards.
func TestTheApprovalListDoesNotClipItsPanels(t *testing.T) {
	html := renderAdmin(t, AwaitingApproval([]payouts.BatchDetail{
		awaitingBatch("human fund", []string{"ada"}),
	}))

	require.NotContains(t, html, "overflow-y-auto max-h-",
		"a scroll box around the list clips the panels positioned inside it")

	// The panel keeps its own scroll, which clips only its own names.
	require.Contains(t, html, "max-h-64 overflow-y-auto")
}

// A long list scrolls inside the panel rather than being cut short. In a tooltip
// there was nowhere to put forty names; in a panel there is.
func TestALongPayeeListScrollsRatherThanTruncating(t *testing.T) {
	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("payee-%02d", i)
	}

	html := renderAdmin(t, BatchRow(awaitingBatch("human fund", names)))

	require.Contains(t, html, "payee-00")
	require.Contains(t, html, "payee-39", "the last name should still be there")
	require.NotContains(t, html, "more")
	require.Contains(t, html, "overflow-y-auto")
}

// A batch with no payouts recorded yet is a real state. It still lists, and it
// does not pretend to have names to show.
func TestABatchWithNoNamesOffersNoHover(t *testing.T) {
	batch := awaitingBatch("human fund", nil)
	batch.NumEnrollments = 4

	html := renderAdmin(t, BatchRow(batch))

	require.Contains(t, html, "4 payees")
	require.NotContains(t, html, "cursor-help")
}

// The classes that open the panel have to exist in the built stylesheet. An
// unbuilt class does not error, it just has no effect -- the panel would be
// permanently hidden and nothing in a template test would notice.
func TestThePanelStylesAreBuilt(t *testing.T) {
	css, err := os.ReadFile("../../public/styles.css")
	require.NoError(t, err)

	sheet := string(css)

	for _, class := range []string{"group-hover", "group-focus-within", "z-50", "top-full"} {
		require.Containsf(t, sheet, class, "%s is not in the built stylesheet", class)
	}
}

// The names are the point of the panel, and a treasurer reading them is one click
// from wanting the person behind one.
func TestEachPayeeLinksToTheirMemberPage(t *testing.T) {
	batch := awaitingBatch("human fund", []string{"ada", "bo"})

	html := renderAdmin(t, PayeeCount(batch))

	for _, payee := range batch.Payees {
		require.Contains(t, html, `href="/admin/member/`+payee.ID.String()+`"`)
	}
}

// A payee whose member row is gone still has a name, from the snapshot taken when
// they were enrolled. There is nowhere to send anybody, and a link to
// /admin/member/00000000-... is worse than no link.
func TestAPayeeWithNoMemberIsNotLinked(t *testing.T) {
	batch := awaitingBatch("human fund", nil)
	batch.NumEnrollments = 2
	batch.Payees = []payouts.Payee{
		{ID: uuid.New(), Name: "ada"},
		{Name: "someone who left"},
	}

	html := renderAdmin(t, PayeeCount(batch))

	require.Contains(t, html, "someone who left")
	require.NotContains(t, html, uuid.Nil.String())
}

// Moving the pointer from the count down to a name crosses the space between
// them. With that space outside the panel it is not hovered, so the panel shut
// before any name could be reached.
func TestNothingUnhoverableSitsBetweenTheCountAndTheNames(t *testing.T) {
	html := renderAdmin(t, PayeeCount(awaitingBatch("human fund", []string{"ada"})))

	// The gap is padding inside the panel, not a margin beside it.
	require.Contains(t, html, "top-full pt-1")
	require.NotContains(t, html, "top-full mt-1")
}
