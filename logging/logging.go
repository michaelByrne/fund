// Package logging builds the logger every entry point uses.
//
// There were four of these, one per command, each of them
// slog.New(slog.NewJSONHandler(os.Stdout, nil)). Identical, so nothing was
// gained by having four, and nowhere to put anything they should share: no
// level to turn down, and no field saying which of the six processes wrote a
// line. Five of those six are Railway cron jobs whose output lands in the same
// place as the web service's.
package logging

import (
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
)

// LevelEnvVar names the environment variable that sets the level. Unset means
// info, which is what the whole of this package exists to make usable: until
// there was a dial, every line added at info shipped forever.
const LevelEnvVar = "LOG_LEVEL"

// New returns the logger for a process, tagged with which process it is.
//
// service is the name of the entry point -- "web", "payout-sweep" -- not the
// package doing the logging. Six processes write to one stream, and telling
// them apart afterwards is the difference between reading the logs and grepping
// them.
func New(service string) *slog.Logger {
	return newLogger(os.Stdout, service, os.Getenv(LevelEnvVar))
}

func newLogger(out io.Writer, service, configured string) *slog.Logger {
	level, unrecognised := levelFrom(configured)

	handler := WithContext(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))

	logger := slog.New(handler).With(slog.String("service", service))

	if revision := buildRevision(); revision != "" {
		logger = logger.With(slog.String("revision", revision))
	}

	// Said out loud rather than silently defaulted. A typo in LOG_LEVEL is
	// indistinguishable from the variable working, right up until someone needs
	// the debug lines they thought they had turned on.
	if unrecognised != "" {
		logger.Warn("unrecognised "+LevelEnvVar+", using info",
			slog.String("value", unrecognised),
		)
	}

	return logger
}

// levelFrom parses the configured level, and reports the value back when it
// could not.
func levelFrom(configured string) (slog.Level, string) {
	switch strings.ToLower(strings.TrimSpace(configured)) {
	case "":
		return slog.LevelInfo, ""
	case "debug":
		return slog.LevelDebug, ""
	case "info":
		return slog.LevelInfo, ""
	case "warn", "warning":
		return slog.LevelWarn, ""
	case "error":
		return slog.LevelError, ""
	default:
		return slog.LevelInfo, configured
	}
}

// buildRevision is the commit the binary was built from, or empty.
//
// Read from the build info Go embeds rather than passed with -ldflags, so it
// needs no build-time cooperation. It is empty when the build had no VCS
// information to embed -- a container built from a copied source tree, say --
// and the field is then left off entirely rather than reported as "unknown",
// which would be a value that sorts and filters like a real one.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, setting := range info.Settings {
		if setting.Key != "vcs.revision" {
			continue
		}

		if len(setting.Value) > 7 {
			return setting.Value[:7]
		}

		return setting.Value
	}

	return ""
}
