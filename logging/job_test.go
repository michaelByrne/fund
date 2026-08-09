package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
)

func job(t *testing.T, name string, work func(context.Context) ([]slog.Attr, error)) ([]map[string]any, error) {
	t.Helper()

	var out bytes.Buffer

	err := Job(context.Background(), newLogger(&out, "payout", ""), name, work)

	return lines(t, &out), err
}

func nothing(context.Context) ([]slog.Attr, error) { return nil, nil }

// Both ends, because the failure these jobs actually have is not an error, it is
// not running. A started with no finished is a job that died; no started at all
// is a schedule that stopped.
func TestAJobBracketsItself(t *testing.T) {
	records, err := job(t, "plan-due", nothing)
	if err != nil {
		t.Fatalf("job: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("wrote %d lines, want a started and a finished", len(records))
	}

	if records[0]["msg"] != "job started" || records[1]["msg"] != "job finished" {
		t.Errorf("lines are %q then %q", records[0]["msg"], records[1]["msg"])
	}

	for i, record := range records {
		if record["job"] != "plan-due" {
			t.Errorf("line %d does not name the job: %v", i, record)
		}
	}

	if _, present := records[1]["duration_ms"]; !present {
		t.Error("the finished line should say how long it took")
	}
}

// Zero is the interesting case. "planned 0 batches" every day for a week is a
// signal; a line that omits the count when it is zero is not.
func TestZeroCountsAreStillReported(t *testing.T) {
	records, err := job(t, "plan-due", func(context.Context) ([]slog.Attr, error) {
		return []slog.Attr{slog.Int("planned", 0), slog.Int("skipped", 0)}, nil
	})
	if err != nil {
		t.Fatalf("job: %v", err)
	}

	finished := records[1]

	for _, field := range []string{"planned", "skipped"} {
		value, present := finished[field]
		if !present {
			t.Errorf("%s is missing from the finished line: %v", field, finished)
		}
		if value != float64(0) {
			t.Errorf("%s = %v, want 0", field, value)
		}
	}
}

// A submit that sent three batches and then lost the provider is a different
// morning from one that sent none, so whatever the work managed is reported
// alongside the failure.
func TestAFailedJobStillReportsWhatItManaged(t *testing.T) {
	wanted := errors.New("provider unreachable")

	records, err := job(t, "submit-approved", func(context.Context) ([]slog.Attr, error) {
		return []slog.Attr{slog.Int("submitted", 3)}, wanted
	})

	if !errors.Is(err, wanted) {
		t.Fatalf("err = %v, want the work's error passed through", err)
	}

	failed := records[1]

	if failed["level"] != "ERROR" || failed["msg"] != "job failed" {
		t.Errorf("second line = %v, want an error saying the job failed", failed)
	}
	if failed["submitted"] != float64(3) {
		t.Errorf("submitted = %v, want the partial count kept", failed["submitted"])
	}
	if failed["error"] != wanted.Error() {
		t.Errorf("error = %v, want %q", failed["error"], wanted)
	}
	if _, present := failed["duration_ms"]; !present {
		t.Error("a failed job should still say how long it ran for")
	}
}

// The started line has to come out before the work, or a job that dies opening
// the database is indistinguishable from one whose schedule never fired -- which
// is the pair this exists to tell apart.
func TestTheStartedLineIsWrittenBeforeTheWork(t *testing.T) {
	var out bytes.Buffer

	logger := newLogger(&out, "payout", "")

	var seen string
	_ = Job(context.Background(), logger, "plan-due", func(context.Context) ([]slog.Attr, error) {
		seen = out.String()

		return nil, errors.New("could not connect")
	})

	if seen == "" {
		t.Error("nothing had been written by the time the work ran")
	}
}
