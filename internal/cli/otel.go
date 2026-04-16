package cli

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zthxxx/hams/internal/otel"
)

// otelEnvVar is the opt-in env var that enables OTel trace + metric
// collection during `hams apply` / `hams refresh`. Accepts the same
// loose boolean variants as hook defer:true (true/yes/on/1).
//
// Opt-in rather than default-on because the local file exporter
// writes JSON to `${HAMS_DATA_HOME}/otel/traces/` and
// `${HAMS_DATA_HOME}/otel/metrics/` on every run — silent file
// accumulation on users who didn't ask for it is bad UX. Users who
// want observability set the env var once (shell rc) or inline.
const otelEnvVar = "HAMS_OTEL"

// otelStatus* are the span.EndStatus string tokens used across the
// CLI/executor boundary. Extracted to satisfy goconst (each string
// appears in at least 3 places: apply.go, commands.go, otel.go).
const (
	otelStatusOK    = "ok"
	otelStatusError = "error"
)

// otelSessionState carries the session + root span so callers can
// both pass the session to provider.Execute and end/flush the root
// span at shutdown. Zero value represents "OTel disabled".
type otelSessionState struct {
	session   *otel.Session
	rootSpan  *otel.Span
	startTime time.Time
	operation string // "hams.apply" or "hams.refresh" for metric labeling
}

// maybeStartOTelSession returns an active OTel session when
// HAMS_OTEL is truthy AND dataHome is writable. Returns a zero
// otelSessionState (both fields nil) when disabled — calling code
// passes the nil session to provider.Execute without any additional
// branches (Execute treats nil-session as "no tracing").
//
// operation names the root span (e.g., "hams.apply", "hams.refresh")
// so trace consumers can filter by top-level action.
func maybeStartOTelSession(dataHome, operation string) otelSessionState {
	if !isOTelEnabled() {
		return otelSessionState{}
	}
	if dataHome == "" {
		slog.Debug("otel requested but HAMS_DATA_HOME is empty; skipping trace export")
		return otelSessionState{}
	}

	session := otel.NewSession(otel.Config{
		DataHome: dataHome,
		Enabled:  true,
	})
	// Root span per-observability-spec. Required attributes
	// (hams.profile, hams.providers.count, hams.result) are filled
	// in by the caller via AttachRootAttrs / recorded at End.
	root := session.StartSpan(operation, "", nil)
	return otelSessionState{
		session:   session,
		rootSpan:  root,
		startTime: time.Now(),
		operation: operation,
	}
}

// AttachRootAttrs records context-dependent attributes on the root
// span. Called once after the CLI has resolved the profile tag and
// counted the number of providers it will execute. No-op when
// OTel is disabled.
func (s otelSessionState) AttachRootAttrs(profile string, providerCount int) {
	if s.rootSpan == nil {
		return
	}
	if s.rootSpan.Attrs == nil {
		s.rootSpan.Attrs = map[string]string{}
	}
	s.rootSpan.Attrs["hams.profile"] = profile
	s.rootSpan.Attrs["hams.providers.count"] = strconv.Itoa(providerCount)
}

// End finalizes the root span and flushes the session via Shutdown.
// A background context with the caller's cancelation is acceptable;
// otel.Shutdown internally applies its own select on ctx.Done() so
// a stuck flush cannot hang the caller past a reasonable budget.
//
// The root span gets a hams.result attribute mapped from status:
//
//	"ok"              → "success"
//	"partial-failure" → passthrough
//	"error"           → "failure"
//
// Matching the observability spec's three documented result values.
//
// Duration metric hams.apply.duration is emitted unconditionally
// (spec requires it on both apply and refresh) with hams.command
// derived from the operation name.
//
// Safe to call on a zero otelSessionState (no-op).
func (s otelSessionState) End(ctx context.Context, status string) {
	if s.session == nil {
		return
	}

	// Map executor-level status → spec-required hams.result attr.
	result := status
	switch status {
	case otelStatusOK:
		result = "success"
	case otelStatusError:
		result = "failure"
	}

	if s.rootSpan != nil {
		if s.rootSpan.Attrs == nil {
			s.rootSpan.Attrs = map[string]string{}
		}
		s.rootSpan.Attrs["hams.result"] = result
		s.session.EndSpan(s.rootSpan, status)
	}

	// Apply duration metric per observability spec. Unit: ms.
	elapsed := time.Since(s.startTime).Milliseconds()
	cmd := strings.TrimPrefix(s.operation, "hams.")
	s.session.RecordMetric("hams.apply.duration", float64(elapsed), "ms", map[string]string{
		"hams.command": cmd,
		"hams.result":  result,
	})

	if err := s.session.Shutdown(ctx); err != nil {
		slog.Warn("otel shutdown failed", "error", err)
	}
}

// Session returns the underlying *otel.Session (nil when disabled).
// Tests and callers pass this to provider.Execute; a nil session is
// silently treated as "no tracing" by Execute.
func (s otelSessionState) Session() *otel.Session {
	return s.session
}

// isOTelEnabled reads HAMS_OTEL and applies loose-boolean parsing.
func isOTelEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(otelEnvVar))) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}
