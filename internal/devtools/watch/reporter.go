package main

import (
	"log/slog"
)

// slogReporter formats build lifecycle events through a slog.Logger.
type slogReporter struct {
	logger *slog.Logger
}

// NewSlogReporter wraps a slog.Logger as a Reporter.
func NewSlogReporter(logger *slog.Logger) Reporter {
	return &slogReporter{logger: logger}
}

func (r *slogReporter) BuildStarted() {
	r.logger.Info("build started")
}

func (r *slogReporter) BuildFinished(res BuildResult) {
	if res.Err != nil {
		r.logger.Error("build failed",
			"err", res.Err,
			"duration", FormatDuration(res.Duration),
			"stderr", res.Stderr,
		)
		return
	}
	r.logger.Info("build ok",
		"commit", res.CommitSHA,
		"duration", FormatDuration(res.Duration),
	)
}
