package fundevents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"boardfund/service/fundevents"

	"github.com/google/uuid"
)

// recordingStore stands in for the database so these can run without one. The
// second insert of a key already seen returns no event, which is what the real
// store does on a dedupe conflict.
type recordingStore struct {
	seen map[string]bool
	err  error
}

func (s *recordingStore) InsertFundEvent(_ context.Context, arg fundevents.Record) (*fundevents.Event, error) {
	if s.err != nil {
		return nil, s.err
	}

	if arg.DedupeKey != "" {
		if s.seen[arg.DedupeKey] {
			return nil, nil
		}

		s.seen[arg.DedupeKey] = true
	}

	return &fundevents.Event{ID: uuid.New(), FundID: arg.FundID, Kind: arg.Kind}, nil
}

func (s *recordingStore) GetFundEvents(context.Context, uuid.UUID, int32) ([]fundevents.Event, error) {
	return nil, nil
}

func record(t *testing.T, store *recordingStore, level slog.Level, r fundevents.Record) []map[string]any {
	t.Helper()

	var out bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: level}))
	fundevents.NewService(store, logger).Record(context.Background(), r)

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Fatalf("log line is not json: %v\n%s", err, line)
		}

		records = append(records, parsed)
	}

	return records
}

func newStore() *recordingStore { return &recordingStore{seen: map[string]bool{}} }

// The invariant: recorded means logged. Twelve of the nineteen record sites had
// no log line of their own, so a completed donation wrote a row and returned
// nil. Doing it here rather than at the call sites is what stops a twentieth
// site from being the next one that forgets.
func TestRecordingAnEventLogsIt(t *testing.T) {
	fundID, donor, actor, donation := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	amount := int32(2500)

	records := record(t, newStore(), slog.LevelInfo, fundevents.Record{
		FundID:          fundID,
		Kind:            fundevents.KindDonationStarted,
		AmountCents:     &amount,
		ActorMemberID:   &actor,
		SubjectMemberID: &donor,
		ReferenceID:     &donation,
		Detail:          "recurring",
	})

	if len(records) != 1 {
		t.Fatalf("wrote %d lines, want 1", len(records))
	}

	line := records[0]

	for field, want := range map[string]any{
		"level":             "INFO",
		"msg":               "fund event",
		"kind":              "donation_started",
		"fund_id":           fundID.String(),
		"amount_cents":      float64(2500),
		"actor_member_id":   actor.String(),
		"subject_member_id": donor.String(),
		"reference_id":      donation.String(),
		"detail":            "recurring",
	} {
		if line[field] != want {
			t.Errorf("%s = %v, want %v", field, line[field], want)
		}
	}
}

// A Record carries uuids, never names -- the actor and subject are resolved to
// names only by the query that renders a page. So the line cannot copy a
// member's details into a stream with different retention, and this pins that
// as a property rather than as something true today.
func TestTheLineCarriesNoNames(t *testing.T) {
	donor := uuid.New()

	records := record(t, newStore(), slog.LevelInfo, fundevents.Record{
		FundID:          uuid.New(),
		Kind:            fundevents.KindPaymentReceived,
		SubjectMemberID: &donor,
	})

	for field := range records[0] {
		for _, banned := range []string{"name", "email", "address"} {
			if strings.Contains(strings.ToLower(field), banned) {
				t.Errorf("the line carries %q, which is a person's details", field)
			}
		}
	}
}

// Absent rather than zero. An amount_cents of 0 on an enrollment reads as a
// payout of nothing rather than as an event that has no amount.
func TestFieldsThatDoNotApplyAreLeftOff(t *testing.T) {
	records := record(t, newStore(), slog.LevelInfo, fundevents.Record{
		FundID: uuid.New(),
		Kind:   fundevents.KindMemberEnrolled,
	})

	for _, field := range []string{"amount_cents", "actor_member_id", "subject_member_id", "reference_id", "detail"} {
		if _, present := records[0][field]; present {
			t.Errorf("%s is on a line it does not apply to", field)
		}
	}
}

// At-least-once delivery means the same webhook arrives again. The row is
// suppressed by the dedupe key, and the line has to be too: writing "payment
// received" a second time for one payment reports it as two.
func TestARedeliveredEventIsNotLoggedAgainAtInfo(t *testing.T) {
	store := newStore()

	first := fundevents.Record{
		FundID:    uuid.New(),
		Kind:      fundevents.KindPaymentReceived,
		DedupeKey: "payment:abc",
	}

	if len(record(t, store, slog.LevelInfo, first)) != 1 {
		t.Fatal("the first delivery should be logged")
	}

	again := record(t, store, slog.LevelInfo, first)
	if len(again) != 0 {
		t.Errorf("a redelivery wrote %v at info", again)
	}

	// Still visible when looking for it: a redelivery is ordinary, not invisible.
	debug := record(t, store, slog.LevelDebug, first)
	if len(debug) != 1 || debug[0]["msg"] != "fund event already recorded" {
		t.Errorf("a redelivery should be findable at debug, got %v", debug)
	}
	if debug[0]["dedupe_key"] != "payment:abc" {
		t.Errorf("the debug line should name the key that suppressed it, got %v", debug[0])
	}
}

// A failed insert already logged an error. Logging the info line as well would
// say the event was recorded when it was not.
func TestAFailedInsertIsNotReportedAsRecorded(t *testing.T) {
	store := &recordingStore{seen: map[string]bool{}, err: errors.New("database down")}

	records := record(t, store, slog.LevelInfo, fundevents.Record{
		FundID: uuid.New(),
		Kind:   fundevents.KindPaymentReceived,
	})

	if len(records) != 1 {
		t.Fatalf("wrote %d lines, want just the error", len(records))
	}
	if records[0]["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", records[0]["level"])
	}
	if records[0]["msg"] == "fund event" {
		t.Error("an event that failed to insert must not be logged as recorded")
	}
}
