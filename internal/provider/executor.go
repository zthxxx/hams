package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zthxxx/hams/internal/state"
)

// ExecuteResult summarizes the outcome of an apply execution.
type ExecuteResult struct {
	Installed int
	Updated   int
	Removed   int
	Skipped   int
	Failed    int
	Errors    []error
}

// Execute runs actions sequentially for a single provider.
// It updates the state file after each action.
func Execute(ctx context.Context, p Provider, actions []Action, sf *state.File) ExecuteResult {
	var result ExecuteResult

	for _, action := range actions {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			return result
		default:
		}

		switch action.Type {
		case ActionSkip:
			result.Skipped++
			continue
		case ActionInstall:
			executeInstall(ctx, p, action, sf, &result)
		case ActionUpdate:
			executeUpdate(ctx, p, action, sf, &result)
		case ActionRemove:
			executeRemove(ctx, p, action, sf, &result)
		}
	}

	return result
}

func executeInstall(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult) {
	name := p.Manifest().Name

	// Set state to pending before executing.
	sf.SetResource(action.ID, state.StatePending)

	slog.Info("installing", "provider", name, "resource", action.ID)
	if err := p.Apply(ctx, action); err != nil {
		slog.Error("install failed", "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: install %s: %w", name, action.ID, err))
		return
	}

	sf.SetResource(action.ID, state.StateOK)
	result.Installed++
	slog.Info("installed", "provider", name, "resource", action.ID)
}

func executeUpdate(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult) {
	name := p.Manifest().Name

	slog.Info("updating", "provider", name, "resource", action.ID)
	if err := p.Apply(ctx, action); err != nil {
		slog.Error("update failed", "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: update %s: %w", name, action.ID, err))
		return
	}

	sf.SetResource(action.ID, state.StateOK)
	result.Updated++
	slog.Info("updated", "provider", name, "resource", action.ID)
}

func executeRemove(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult) {
	name := p.Manifest().Name

	slog.Info("removing", "provider", name, "resource", action.ID)
	if err := p.Remove(ctx, action.ID); err != nil {
		slog.Error("remove failed", "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: remove %s: %w", name, action.ID, err))
		return
	}

	sf.SetResource(action.ID, state.StateRemoved)
	result.Removed++
	slog.Info("removed", "provider", name, "resource", action.ID)
}

// MergeResults combines multiple ExecuteResults into one.
func MergeResults(results []ExecuteResult) ExecuteResult {
	var merged ExecuteResult
	for _, r := range results {
		merged.Installed += r.Installed
		merged.Updated += r.Updated
		merged.Removed += r.Removed
		merged.Skipped += r.Skipped
		merged.Failed += r.Failed
		merged.Errors = append(merged.Errors, r.Errors...)
	}
	return merged
}
