package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// lines parses everything written, one decoded record per line.
func lines(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}

		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not json: %v\n%s", err, line)
		}

		records = append(records, record)
	}

	return records
}

func TestEveryLineSaysWhichProcessWroteIt(t *testing.T) {
	var out bytes.Buffer

	newLogger(&out, "payout-sweep", "").Info("planned batches")

	records := lines(t, &out)
	if len(records) != 1 {
		t.Fatalf("wrote %d lines, want 1", len(records))
	}

	if records[0]["service"] != "payout-sweep" {
		// Six processes write to one stream. Without this they are one stream.
		t.Errorf("service = %v, want payout-sweep", records[0]["service"])
	}
}

func TestTheLevelComesFromTheEnvironment(t *testing.T) {
	for _, tc := range []struct {
		configured string
		debug      bool
		warn       bool
	}{
		{configured: "", debug: false, warn: true},
		{configured: "debug", debug: true, warn: true},
		{configured: "DEBUG", debug: true, warn: true},
		{configured: " warn ", debug: false, warn: true},
		{configured: "error", debug: false, warn: false},
	} {
		var out bytes.Buffer

		logger := newLogger(&out, "web", tc.configured)
		logger.Debug("a debug line")
		logger.Warn("a warn line")

		body := out.String()

		if strings.Contains(body, "a debug line") != tc.debug {
			t.Errorf("%q: debug visible = %v, want %v", tc.configured, !tc.debug, tc.debug)
		}
		if strings.Contains(body, "a warn line") != tc.warn {
			t.Errorf("%q: warn visible = %v, want %v", tc.configured, !tc.warn, tc.warn)
		}
	}
}

// A typo in LOG_LEVEL looks exactly like the variable working, right up until
// someone needs the debug lines they thought they had turned on.
func TestAnUnreadableLevelSaysSoRatherThanDefaultingQuietly(t *testing.T) {
	var out bytes.Buffer

	newLogger(&out, "web", "verbose").Info("started")

	records := lines(t, &out)
	if len(records) != 2 {
		t.Fatalf("wrote %d lines, want a warning and the info line", len(records))
	}

	if records[0]["level"] != "WARN" || !strings.Contains(records[0]["msg"].(string), LevelEnvVar) {
		t.Errorf("first line = %v, want a warning naming %s", records[0], LevelEnvVar)
	}
	if records[0]["value"] != "verbose" {
		t.Errorf("the warning should quote what was configured, got %v", records[0]["value"])
	}
}

func TestARequestIDInTheContextReachesTheLine(t *testing.T) {
	var out bytes.Buffer

	ctx := WithRequestID(context.Background(), "abc-123")
	newLogger(&out, "web", "").InfoContext(ctx, "saved a fund")

	if got := lines(t, &out)[0]["request_id"]; got != "abc-123" {
		t.Errorf("request_id = %v, want abc-123", got)
	}
}

// The whole value of reading the id from the context is that call sites do not
// change. A logger built with With() is the common case in this codebase --
// services hold one and derive from it -- and if deriving dropped the wrapper,
// the lines most worth tracing would be the ones that lost the id.
func TestDerivedLoggersKeepTheRequestID(t *testing.T) {
	var out bytes.Buffer

	ctx := WithRequestID(context.Background(), "abc-123")

	derived := newLogger(&out, "web", "").
		With(slog.String("fund_id", "f-1")).
		With(slog.String("batch_id", "b-1"))

	derived.InfoContext(ctx, "submitted")

	record := lines(t, &out)[0]

	if record["request_id"] != "abc-123" {
		t.Errorf("a derived logger lost the request id: %v", record)
	}
	if record["fund_id"] != "f-1" || record["batch_id"] != "b-1" {
		t.Errorf("a derived logger lost its own attributes: %v", record)
	}
}

// Pinning the known wart, so the first caller to open a group finds it written
// down rather than working it out from a log line that nearly matches.
//
// Handle adds the id as a record attribute and an open group swallows record
// attributes, so the id lands at payout.request_id and a cross-service search
// for request_id stops finding it. Unfixable from inside a wrapper: escaping
// every group means adding the attribute before the group was opened, and the
// id does not exist until the request does.
func TestAGroupNestsTheRequestID(t *testing.T) {
	var out bytes.Buffer

	ctx := WithRequestID(context.Background(), "abc-123")

	newLogger(&out, "web", "").WithGroup("payout").InfoContext(ctx, "submitted")

	record := lines(t, &out)[0]

	if _, topLevel := record["request_id"]; topLevel {
		t.Fatal("this now works; delete this test and the comment on WithGroup")
	}

	group, ok := record["payout"].(map[string]any)
	if !ok || group["request_id"] != "abc-123" {
		t.Errorf("expected the id nested under the group, got %v", record)
	}
}

// Cron jobs, webhook consumers and tests have no request. They must log
// normally, with the field absent rather than empty -- an empty request_id is a
// value that looks like a failed lookup.
func TestNoRequestMeansNoField(t *testing.T) {
	var out bytes.Buffer

	newLogger(&out, "payout-sweep", "").InfoContext(context.Background(), "swept")

	if _, present := lines(t, &out)[0]["request_id"]; present {
		t.Error("a line with no request behind it should carry no request_id at all")
	}
}
