package provider

import "testing"

// TestPhaseGerund_KnownPhases locks in the English spelling for
// each known phase. The prior implementation used `phase+"ing"`
// which silently produced "updateing" and "removeing" in every
// `hams apply` log line — not English AND not greppable by the
// terms ops runbooks and docs use ("updating"/"removing").
func TestPhaseGerund_KnownPhases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		phase string
		want  string
	}{
		{phaseInstall, "installing"},
		{phaseUpdate, "updating"},
		{phaseRemove, "removing"},
	}
	for _, tc := range cases {
		if got := phaseGerund(tc.phase); got != tc.want {
			t.Errorf("phaseGerund(%q) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

// TestPhasePastTense_KnownPhases locks in the "-ed" spelling. The
// prior implementation used `phase+"d"` which produced "installd"
// (missing `e`) in the success-log line for every installed
// resource. This test gates future regressions.
func TestPhasePastTense_KnownPhases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		phase string
		want  string
	}{
		{phaseInstall, "installed"},
		{phaseUpdate, "updated"},
		{phaseRemove, "removed"},
	}
	for _, tc := range cases {
		if got := phasePastTense(tc.phase); got != tc.want {
			t.Errorf("phasePastTense(%q) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

// TestPhaseGerund_UnknownPhase asserts the fallback behavior for
// phases added by future refactors without a lookup entry: the
// naive concat is at least predictable, and the value flows to
// slog so a grep on the log surface still finds the phase name.
func TestPhaseGerund_UnknownPhase(t *testing.T) {
	t.Parallel()
	if got := phaseGerund("probe"); got != "probeing" {
		t.Errorf("phaseGerund(probe) = %q, want probeing (fallback)", got)
	}
	if got := phasePastTense("probe"); got != "probed" {
		t.Errorf("phasePastTense(probe) = %q, want probed (fallback)", got)
	}
}
