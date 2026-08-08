package adminweb

import (
	"fmt"
	"os"
	"testing"
	"time"

	"boardfund/service/payouts"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func awaitingBatch(fundName string, names []string) payouts.BatchDetail {
	deadline := time.Now().Add(time.Hour)

	return payouts.BatchDetail{
		Batch: payouts.Batch{
			ID: uuid.New(), FundID: uuid.New(), AmountCents: 4000,
			NumEnrollments: int32(len(names)), Status: payouts.StatusAwaitingApproval,
			PayoutDate: time.Now(), ApprovalDeadline: &deadline,
		},
		FundName:   fundName,
		PayeeNames: names,
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

	for _, name := range batch.PayeeNames {
		require.Contains(t, html, ">"+name+"<", "every payee should be listed")
	}

	// A real popover, not a title. A title is slow, unstylable, and on a touch
	// screen there is no hover at all -- the names could not be reached on a phone.
	require.NotContains(t, html, "title=")

	// The button and its panel have to agree, and the panel has to be one.
	id := "payees-" + batch.ID.String()
	require.Contains(t, html, `popovertarget="`+id+`"`)
	require.Contains(t, html, `id="`+id+`"`)
	require.Contains(t, html, "popover")

	// A button, so it is clickable and focusable without anything being added.
	require.Contains(t, html, `type="button"`)

	// And it looks like it does something.
	require.Contains(t, html, "decoration-dotted")
}

// The approval page lists several batches. Two panels sharing an id would open
// whichever the browser found first, which is the wrong list of people.
func TestEachBatchHasItsOwnPanel(t *testing.T) {
	first := awaitingBatch("human fund", []string{"ada"})
	second := awaitingBatch("winter fund", []string{"bo"})

	html := renderAdmin(t, BatchList([]payouts.BatchDetail{first, second}))

	require.Contains(t, html, "payees-"+first.ID.String())
	require.Contains(t, html, "payees-"+second.ID.String())
	require.NotEqual(t, first.ID, second.ID)
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

// The stylesheet has to carry both rules or the panel is broken in one direction
// or the other, and neither shows up in a template test.
func TestThePopoverStylesAreBuilt(t *testing.T) {
	css, err := os.ReadFile("../../public/styles.css")
	require.NoError(t, err)

	sheet := string(css)

	// A browser without the popover API drops the :popover-open rule as invalid.
	// Without this one the panel is not hidden by anything, and every payee list
	// on the page renders inline under the batches.
	require.Contains(t, sheet, "[popover]{display:none}")

	// And with only the first rule it never opens: an author rule beats the UA
	// stylesheet that would otherwise reveal it.
	require.Contains(t, sheet, ":popover-open")
}
