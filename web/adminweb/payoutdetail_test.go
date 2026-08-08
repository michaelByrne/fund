package adminweb

import (
	"strings"
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
func TestThePayeeCountNamesThePayeesOnHover(t *testing.T) {
	html := renderAdmin(t, BatchRow(awaitingBatch("human fund", []string{"ada", "bo", "cyd"})))

	require.Contains(t, html, "3 payees")

	// One title holding all three. Newlines are escaped in the attribute, so this
	// looks for them as rendered.
	require.Contains(t, html, "ada")
	require.Contains(t, html, "bo")
	require.Contains(t, html, "cyd")
	require.Contains(t, html, "title=")

	// Reachable without a mouse: a browser shows a title on focus, and without
	// this the names are information only a pointer can get at.
	require.Contains(t, html, `tabindex="0"`)

	// And it looks hoverable, or nobody hovers it.
	require.Contains(t, html, "decoration-dotted")
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

// A batch can pay everybody in a fund, and a tooltip the height of the screen is
// not a tooltip.
func TestALongPayeeListIsCapped(t *testing.T) {
	names := make([]string, 40)
	for i := range names {
		names[i] = "payee-" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
	}

	listed := payeeList(names)

	require.Equal(t, 16, strings.Count(listed, "\n")+1, "fifteen names and a line saying how many more")
	require.Contains(t, listed, "and 25 more")
}

// Under the cap, everybody is named and nothing says there is more.
func TestAShortPayeeListIsWhole(t *testing.T) {
	listed := payeeList([]string{"ada", "bo", "cyd"})

	require.Equal(t, "ada\nbo\ncyd", listed)
	require.NotContains(t, listed, "more")
}
