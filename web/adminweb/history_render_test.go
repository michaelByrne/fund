package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// The actor is shown only when it tells the reader something. An automated
// action must always say so, and a person who is also the subject is already
// named on the row.
func TestEventActor(t *testing.T) {
	ctx := context.Background()

	person := uuid.New()
	other := uuid.New()

	cases := []struct {
		name     string
		event    fundevents.Event
		want     string
		wantNone bool
	}{
		{
			name:  "no actor means automatic",
			event: fundevents.Event{Kind: fundevents.KindDonationCancelled},
			want:  "automatic",
		},
		{
			name: "an actor who is not the subject is credited",
			event: fundevents.Event{
				ActorMemberID: &person, ActorName: "treasurer",
				SubjectMemberID: &other, SubjectName: "payee",
			},
			want: "by treasurer",
		},
		{
			// member.bco_name is nullable, so this is reachable and must not be
			// mistaken for an automated action.
			name:  "an actor with no name is still a person",
			event: fundevents.Event{ActorMemberID: &person},
			want:  "by unknown member",
		},
		{
			// A donor starting their own donation. The row already says "tester";
			// adding "by tester" was noise.
			name: "an actor who is the subject is left out",
			event: fundevents.Event{
				ActorMemberID: &person, ActorName: "tester",
				SubjectMemberID: &person, SubjectName: "tester",
			},
			wantNone: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			if err := EventActor(c.event).Render(ctx, &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			text := strings.TrimSpace(stripTags(out.String()))

			if c.wantNone {
				if text != "" {
					t.Errorf("rendered %q, want nothing", text)
				}

				return
			}

			if !strings.Contains(out.String(), c.want) {
				t.Errorf("rendered %q, want it to contain %q", out.String(), c.want)
			}

			// Silence is the failure mode for every case that should say
			// something, and asserting on content alone would not catch it.
			if text == "" {
				t.Error("rendered nothing")
			}
		})
	}
}

func TestFundHistoryRenders(t *testing.T) {
	ctx := context.Background()

	donor := uuid.New()
	treasurer := uuid.New()
	amount := int32(2500)

	events := []fundevents.Event{
		{
			Kind: fundevents.KindDonationStarted, OccurredAt: time.Now(),
			ActorMemberID: &donor, ActorName: "tester",
			SubjectMemberID: &donor, SubjectName: "tester",
			AmountCents: &amount, Detail: "recurring",
		},
		{
			Kind: fundevents.KindDonationCancelled, OccurredAt: time.Now(),
			SubjectMemberID: &donor, SubjectName: "tester",
			Detail: "subscription cancelled at provider",
		},
		{
			Kind: fundevents.KindBatchApproved, OccurredAt: time.Now(),
			ActorMemberID: &treasurer, ActorName: "treasurer",
			AmountCents: &amount,
		},
		// Every optional field absent.
		{Kind: fundevents.KindBatchSettled, OccurredAt: time.Now()},
	}

	var out strings.Builder
	if err := FundHistory(events).Render(ctx, &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	rendered := out.String()

	for _, want := range []string{
		"donation started", "donation cancelled", "payout batch approved",
		"payout batch settled", "$25.00", "automatic", "by treasurer",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("history missing %q", want)
		}
	}

	// The donor started their own donation, so they are named once as the
	// subject and not again as the actor.
	if strings.Contains(rendered, "by tester") {
		t.Error("the subject was repeated as the actor")
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
