package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestMaybeStartOTelSession_DisabledWithoutEnv asserts that when
// HAMS_OTEL is unset, no session is created — the session field is
// nil and Session() returns nil.
func TestMaybeStartOTelSession_DisabledWithoutEnv(t *testing.T) {
	t.Setenv(otelEnvVar, "")
	got := maybeStartOTelSession(t.TempDir(), "hams.apply")
	if got.Session() != nil {
		t.Errorf("Session() = %v, want nil when HAMS_OTEL is unset", got.Session())
	}
}

// TestMaybeStartOTelSession_EnabledButNoDataHome asserts that
// HAMS_OTEL=1 with an empty dataHome also returns a disabled
// session — exporting would fail otherwise, and we prefer silent
// disable over noisy errors on systems where HAMS_DATA_HOME isn't
// resolvable.
func TestMaybeStartOTelSession_EnabledButNoDataHome(t *testing.T) {
	t.Setenv(otelEnvVar, "1")
	got := maybeStartOTelSession("", "hams.apply")
	if got.Session() != nil {
		t.Errorf("Session() = %v, want nil when dataHome is empty", got.Session())
	}
}

// TestMaybeStartOTelSession_EnabledCreatesSession asserts the happy
// path: HAMS_OTEL=1 with a writable dataHome produces an active
// session with a non-nil root span.
func TestMaybeStartOTelSession_EnabledCreatesSession(t *testing.T) {
	t.Setenv(otelEnvVar, "1")
	dir := t.TempDir()
	got := maybeStartOTelSession(dir, "hams.apply")
	if got.Session() == nil {
		t.Fatal("Session() is nil; expected active session")
	}
	if got.rootSpan == nil {
		t.Error("rootSpan is nil; expected non-nil")
	}
	// Gracefully close so the test doesn't leak files/goroutines.
	got.End(context.Background(), "ok")
}

// TestMaybeStartOTelSession_AcceptsLooseBooleans verifies the four
// truthy env-var forms (true / yes / on / 1) and one falsy (false)
// case. The `defer: true` hook parser uses the same loose booleans;
// consistency matters for user-facing env toggles.
func TestMaybeStartOTelSession_AcceptsLooseBooleans(t *testing.T) {
	cases := []struct {
		value string
		want  bool // expect session non-nil?
	}{
		{"true", true},
		{"yes", true},
		{"on", true},
		{"1", true},
		{"TRUE", true},
		{"YES", true},
		{"false", false},
		{"no", false},
		{"", false},
		{"0", false},
		{"garbage", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(otelEnvVar, tc.value)
			got := maybeStartOTelSession(t.TempDir(), "hams.apply")
			if (got.Session() != nil) != tc.want {
				t.Errorf("Session() non-nil = %v, want %v", got.Session() != nil, tc.want)
			}
			if got.Session() != nil {
				got.End(context.Background(), "ok")
			}
		})
	}
}

// TestOTelSession_EndFlushesTracesToDisk proves the end-to-end wire:
// HAMS_OTEL=1 + dataHome → .End() writes a trace JSON file under
// ${HAMS_DATA_HOME}/otel/traces/. This is the "OTel files actually
// appear" scenario from the cli-architecture spec.
func TestOTelSession_EndFlushesTracesToDisk(t *testing.T) {
	t.Setenv(otelEnvVar, "1")
	dir := t.TempDir()

	sess := maybeStartOTelSession(dir, "hams.apply")
	if sess.Session() == nil {
		t.Fatal("session is nil; setup failed")
	}

	// End flushes + shuts down.
	sess.End(context.Background(), "ok")

	// Verify a traces file was written.
	tracesDir := filepath.Join(dir, "otel", "traces")
	entries, err := os.ReadDir(tracesDir)
	if err != nil {
		t.Fatalf("traces dir not created at %q: %v", tracesDir, err)
	}
	if len(entries) == 0 {
		t.Errorf("no trace files written under %q", tracesDir)
	}
}

// TestOTelSessionState_EndOnZeroValueIsNoOp asserts calling End on a
// zero-valued otelSessionState (session disabled) does not panic and
// produces no output files — matching the documented "no-op" contract.
func TestOTelSessionState_EndOnZeroValueIsNoOp(t *testing.T) {
	var zero otelSessionState
	zero.End(context.Background(), "ok") // should not panic
	if zero.Session() != nil {
		t.Errorf("zero Session() = %v, want nil", zero.Session())
	}
}

// TestAttachRootAttrs_StampsProfileAndCount asserts the root span's
// hams.profile and hams.providers.count attributes match the
// observability spec's required keys after AttachRootAttrs is called.
func TestAttachRootAttrs_StampsProfileAndCount(t *testing.T) {
	t.Setenv(otelEnvVar, "1")
	dir := t.TempDir()
	sess := maybeStartOTelSession(dir, "hams.apply")
	if sess.Session() == nil {
		t.Fatal("session is nil; setup failed")
	}
	defer sess.End(context.Background(), otelStatusOK)

	sess.AttachRootAttrs("macOS", 7)
	if sess.rootSpan == nil {
		t.Fatal("rootSpan is nil")
	}
	if got := sess.rootSpan.Attrs["hams.profile"]; got != "macOS" {
		t.Errorf("hams.profile = %q, want \"macOS\"", got)
	}
	if got := sess.rootSpan.Attrs["hams.providers.count"]; got != "7" {
		t.Errorf("hams.providers.count = %q, want \"7\"", got)
	}
}

// TestAttachRootAttrs_NoOpOnZeroValue asserts AttachRootAttrs is a
// safe no-op when the session is disabled (zero state).
func TestAttachRootAttrs_NoOpOnZeroValue(_ *testing.T) {
	var zero otelSessionState
	zero.AttachRootAttrs("macOS", 7) // should not panic
}

// TestEnd_MapsStatusToHamsResult asserts the documented status →
// hams.result mapping at End: ok→success, error→failure, anything
// else passes through verbatim (e.g., "partial-failure").
func TestEnd_MapsStatusToHamsResult(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{otelStatusOK, "success"},
		{otelStatusError, "failure"},
		{"partial-failure", "partial-failure"},
		{"canceled", "canceled"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Setenv(otelEnvVar, "1")
			dir := t.TempDir()
			sess := maybeStartOTelSession(dir, "hams.apply")
			if sess.Session() == nil {
				t.Fatal("session is nil; setup failed")
			}
			sess.End(context.Background(), tc.input)
			if got := sess.rootSpan.Attrs["hams.result"]; got != tc.want {
				t.Errorf("hams.result = %q, want %q", got, tc.want)
			}
		})
	}
}
