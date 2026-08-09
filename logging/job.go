package logging

import (
	"context"
	"log/slog"
	"time"
)

// Job runs a scheduled command between a line at each end.
//
// Five of these run on Railway cron schedules, and the failure they actually
// have is not an error -- it is not running. A job that stopped firing, whose
// container never started, or that died before it reached any code that logs,
// produces exactly what a healthy quiet night produces: nothing. Both ends are
// written so that absence is visible: a "started" with no "finished" is a job
// that died, and no "started" at all is a schedule that stopped.
//
// The counts come back from the work rather than being logged inside it, so
// they land on the same line as the duration and are there even when they are
// zero. Zero is the interesting case: "planned 0 batches" every day for a week
// is a signal, and silence is not.
//
// Wrap the whole command, including the part that opens the database. A job that
// cannot connect has still run, and the started line is the only evidence of it.
func Job(
	ctx context.Context,
	logger *slog.Logger,
	name string,
	work func(context.Context) ([]slog.Attr, error),
) error {
	logger = logger.With(slog.String("job", name))
	started := time.Now()

	logger.InfoContext(ctx, "job started")

	attrs, err := work(ctx)

	// Built as []any because slog's variadic form takes them, and appended to
	// rather than prepended so the counts read after the duration.
	fields := []any{slog.Int64("duration_ms", time.Since(started).Milliseconds())}
	for _, attr := range attrs {
		fields = append(fields, attr)
	}

	if err != nil {
		// Whatever the work managed before failing is still worth reporting: a
		// submit that sent three batches and then lost the provider is a different
		// morning from one that sent none.
		logger.ErrorContext(ctx, "job failed", append(fields, slog.String("error", err.Error()))...)

		return err
	}

	logger.InfoContext(ctx, "job finished", fields...)

	return nil
}
