package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/state"
)

type execStubProvider struct {
	manifest   Manifest
	applyErr   map[string]error
	removeErr  map[string]error
	appliedIDs []string
	removedIDs []string
}

func (e *execStubProvider) Manifest() Manifest                { return e.manifest }
func (e *execStubProvider) Bootstrap(_ context.Context) error { return nil }
func (e *execStubProvider) Probe(_ context.Context, _ *state.File) ([]ProbeResult, error) {
	return nil, nil
}
func (e *execStubProvider) Plan(_ context.Context, _ *hamsfile.File, _ *state.File) ([]Action, error) {
	return nil, nil
}
func (e *execStubProvider) Apply(_ context.Context, action Action) error {
	e.appliedIDs = append(e.appliedIDs, action.ID)
	if err, ok := e.applyErr[action.ID]; ok {
		return err
	}
	return nil
}
func (e *execStubProvider) Remove(_ context.Context, id string) error {
	e.removedIDs = append(e.removedIDs, id)
	if err, ok := e.removeErr[id]; ok {
		return err
	}
	return nil
}
func (e *execStubProvider) List(_ context.Context, _ *hamsfile.File, _ *state.File) (string, error) {
	return "", nil
}

func TestExecute_AllInstall(t *testing.T) {
	p := &execStubProvider{
		manifest: Manifest{Name: "test"},
		applyErr: make(map[string]error),
	}
	sf := state.New("test", "machine")
	actions := []Action{
		{ID: "htop", Type: ActionInstall},
		{ID: "jq", Type: ActionInstall},
	}

	result := Execute(context.Background(), p, actions, sf)
	if result.Installed != 2 {
		t.Errorf("Installed = %d, want 2", result.Installed)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if sf.Resources["htop"].State != state.StateOK {
		t.Errorf("htop state = %q, want ok", sf.Resources["htop"].State)
	}
}

func TestExecute_FailureRecorded(t *testing.T) {
	p := &execStubProvider{
		manifest: Manifest{Name: "test"},
		applyErr: map[string]error{"jq": errors.New("network error")},
	}
	sf := state.New("test", "machine")
	actions := []Action{
		{ID: "htop", Type: ActionInstall},
		{ID: "jq", Type: ActionInstall},
	}

	result := Execute(context.Background(), p, actions, sf)
	if result.Installed != 1 {
		t.Errorf("Installed = %d, want 1", result.Installed)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if sf.Resources["jq"].State != state.StateFailed {
		t.Errorf("jq state = %q, want failed", sf.Resources["jq"].State)
	}
	if sf.Resources["jq"].LastError != "network error" {
		t.Errorf("jq error = %q, want 'network error'", sf.Resources["jq"].LastError)
	}
}

func TestExecute_SkipActions(t *testing.T) {
	p := &execStubProvider{
		manifest: Manifest{Name: "test"},
		applyErr: make(map[string]error),
	}
	sf := state.New("test", "machine")
	actions := []Action{
		{ID: "htop", Type: ActionSkip},
		{ID: "jq", Type: ActionSkip},
	}

	result := Execute(context.Background(), p, actions, sf)
	if result.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
	if len(p.appliedIDs) != 0 {
		t.Error("provider should not be called for skip actions")
	}
}

func TestExecute_Remove(t *testing.T) {
	p := &execStubProvider{
		manifest:  Manifest{Name: "test"},
		removeErr: make(map[string]error),
	}
	sf := state.New("test", "machine")
	sf.SetResource("curl", state.StateOK)

	actions := []Action{
		{ID: "curl", Type: ActionRemove},
	}

	result := Execute(context.Background(), p, actions, sf)
	if result.Removed != 1 {
		t.Errorf("Removed = %d, want 1", result.Removed)
	}
	if sf.Resources["curl"].State != state.StateRemoved {
		t.Errorf("curl state = %q, want removed", sf.Resources["curl"].State)
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	p := &execStubProvider{
		manifest: Manifest{Name: "test"},
		applyErr: make(map[string]error),
	}
	sf := state.New("test", "machine")
	actions := []Action{
		{ID: "htop", Type: ActionInstall},
	}

	result := Execute(ctx, p, actions, sf)
	if len(result.Errors) == 0 {
		t.Error("should have context cancellation error")
	}
	if result.Installed != 0 {
		t.Error("should not install when context is canceled")
	}
}

func TestMergeResults(t *testing.T) {
	results := []ExecuteResult{
		{Installed: 3, Failed: 1, Skipped: 2},
		{Installed: 2, Removed: 1, Failed: 0},
	}
	merged := MergeResults(results)
	if merged.Installed != 5 || merged.Failed != 1 || merged.Removed != 1 || merged.Skipped != 2 {
		t.Errorf("merged = %+v, want {Installed:5 Failed:1 Removed:1 Skipped:2}", merged)
	}
}
