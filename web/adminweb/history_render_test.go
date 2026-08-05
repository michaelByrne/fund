package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// The actor column exists to separate "a person did this" from "the provider or
// a scheduled job did this". Rendering nothing for a third case would collapse
// that distinction exactly where it matters.
func TestEventActorAlwaysSaysSomething(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()

	cases := []struct {
		name  string
		event fundevents.Event
		want  string
	}{
		{
			name:  "no actor means automatic",
			event: fundevents.Event{Kind: fundevents.KindDonationCancelled},
			want:  "automatic",
		},
		{
			name:  "a named actor is credited",
			event: fundevents.Event{ActorMemberID: &actor, ActorName: "treasurer"},
			want:  "by treasurer",
		},
		{
			// member.bco_name is nullable, so this is reachable.
			name:  "an actor with no name is still a person",
			event: fundevents.Event{ActorMemberID: &actor},
			want:  "by unknown member",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			if err := EventActor(c.event).Render(ctx, &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			got := out.String()

			if !strings.Contains(got, c.want) {
				t.Errorf("rendered %q, want it to contain %q", got, c.want)
			}

			if strings.TrimSpace(stripTags(got)) == "" {
				t.Error("rendered nothing; the actor column must never be silently blank")
			}
		})
	}
}

func TestFundHistoryRenders(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	amount := int32(2500)

	events := []fundevents.Event{
		{Kind: fundevents.KindDonationStarted, OccurredAt: time.Now(), ActorMemberID: &actor, ActorName: "donor", SubjectName: "donor", AmountCents: &amount, Detail: "recurring"},
		{Kind: fundevents.KindDonationCancelled, OccurredAt: time.Now(), SubjectName: "donor", Detail: "subscription cancelled at provider"},
		// Every optional field absent.
		{Kind: fundevents.KindBatchSettled, OccurredAt: time.Now()},
	}

	var out strings.Builder
	if err := FundHistory(events).Render(ctx, &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{"donation started", "donation cancelled", "payout batch settled", "$25.00", "automatic", "by donor"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("history missing %q", want)
		}
	}

	var empty strings.Builder
	if err := FundHistory(nil).Render(ctx, &empty); err != nil {
		t.Fatalf("empty render: %v", err)
	}

	if !strings.Contains(empty.String(), "nothing recorded yet") {
		t.Error("an empty history should say so rather than render a bare list")
	}
}

func stripTags(s string) string {
	var b strings.Builder

	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}

	return b.String()
}
