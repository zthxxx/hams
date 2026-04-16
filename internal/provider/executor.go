package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zthxxx/hams/internal/otel"
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

// Execute runs actions sequentially for a single provider. It updates
// the state file after each action. If otelSession is non-nil, provider
// and resource-level spans are recorded per the observability spec
// (openspec/specs/observability/spec.md): provider spans are named
// `hams.provider.<name>` with `hams.provider.*` attributes; resource
// spans are named `hams.resource.<action>` with `hams.resource.*`
// attributes + `hams.provider.name`.
func Execute(ctx context.Context, p Provider, actions []Action, sf *state.File, otelSession ...*otel.Session) ExecuteResult {
	var result ExecuteResult
	var session *otel.Session
	if len(otelSession) > 0 {
		session = otelSession[0]
	}

	name := p.Manifest().Name

	// Provider-level span — started before actions so the span
	// duration reflects the full provider phase. The
	// hams.provider.resource_count attribute records the PLANNED
	// action count; final failed/skipped tallies go on Shutdown
	// via session-level metrics.
	var providerSpan *otel.Span
	if session != nil {
		providerSpan = session.StartSpan("hams.provider."+name, "", map[string]string{
			"hams.provider.name":           name,
			"hams.provider.resource_count": fmt.Sprintf("%d", len(actions)),
		})
	}

	for _, action := range actions {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			if session != nil && providerSpan != nil {
				endProviderSpan(session, providerSpan, name, &result, "canceled")
			}
			return result
		default:
		}

		switch action.Type {
		case ActionSkip:
			result.Skipped++
			// Skipped actions still get a near-zero-duration span
			// per the observability spec (Scenario: Skipped
			// resource records skip reason).
			recordSkipSpan(session, name, action.ID)
			continue
		case ActionInstall:
			executeAction(ctx, p, action, sf, &result, session, phaseInstall)
		case ActionUpdate:
			executeAction(ctx, p, action, sf, &result, session, phaseUpdate)
		case ActionRemove:
			executeAction(ctx, p, action, sf, &result, session, phaseRemove)
		}
	}

	if session != nil && providerSpan != nil {
		status := "ok"
		if result.Failed > 0 {
			status = "error"
		}
		endProviderSpan(session, providerSpan, name, &result, status)
	}

	return result
}

// endProviderSpan finalizes the provider span with the correct attrs
// + ends it, and records the two provider-level metrics the
// observability spec requires (provider failures, resources total).
func endProviderSpan(session *otel.Session, span *otel.Span, providerName string, result *ExecuteResult, status string) {
	// Attribute update: the full failed_count is only known at
	// end-of-provider. Mutate via the public span.Attrs map.
	if span.Attrs == nil {
		span.Attrs = map[string]string{}
	}
	span.Attrs["hams.provider.failed_count"] = fmt.Sprintf("%d", result.Failed)
	session.EndSpan(span, status)

	// Metrics per observability spec:
	//   hams.provider.failures: counter incremented by 1 per provider
	//   with at least one failed resource (not by failed count — the
	//   spec's scenario says "incremented once per provider that has
	//   at least one failed resource").
	//   hams.resources.total: total resources processed by provider.
	if result.Failed > 0 {
		session.RecordMetric("hams.provider.failures", 1, "count", map[string]string{"hams.provider.name": providerName})
	}
	session.RecordMetric("hams.resources.total",
		float64(result.Installed+result.Updated+result.Removed+result.Skipped+result.Failed),
		"count", map[string]string{"hams.provider.name": providerName})
}

// recordSkipSpan emits a near-zero-duration span for a skipped
// resource action. Per the observability spec scenario "Skipped
// resource records skip reason".
func recordSkipSpan(session *otel.Session, providerName, resourceID string) {
	if session == nil {
		return
	}
	span := session.StartSpan("hams.resource.skip", "", map[string]string{
		"hams.resource.id":     resourceID,
		"hams.resource.action": "skip",
		"hams.resource.result": "skipped",
		"hams.provider.name":   providerName,
	})
	session.EndSpan(span, "ok")
}

const (
	phaseInstall = "install"
	phaseUpdate  = "update"
	phaseRemove  = "remove"
)

// phaseGerund returns the "-ing" form of a phase verb for log
// messages. The naive `phase+"ing"` concat produced "updateing" and
// "removeing" — neither English nor greppable by ops runbooks that
// expect "updating"/"removing". This map hard-codes the English
// spelling changes (drop trailing `e` for -e verbs).
func phaseGerund(phase string) string {
	switch phase {
	case phaseInstall:
		return "installing"
	case phaseUpdate:
		return "updating"
	case phaseRemove:
		return "removing"
	default:
		return phase + "ing"
	}
}

// phasePastTense returns the "-ed" form of a phase verb. The naive
// `phase+"d"` concat produced "installd" — missing the `e` — which
// is what the hams apply log currently outputs. Separate helper so
// tests can assert both tenses independently.
func phasePastTense(phase string) string {
	switch phase {
	case phaseInstall:
		return "installed"
	case phaseUpdate:
		return "updated"
	case phaseRemove:
		return "removed"
	default:
		return phase + "d"
	}
}

// executeAction is the unified helper for install, update, and remove phases.
// The phase parameter must be one of phaseInstall, phaseUpdate, or phaseRemove.
func executeAction(ctx context.Context, p Provider, action Action, sf *state.File, result *ExecuteResult, session *otel.Session, phase string) {
	name := p.Manifest().Name

	var span *otel.Span
	if session != nil {
		span = session.StartSpan("hams.resource."+phase, "", map[string]string{
			"hams.resource.id":     action.ID,
			"hams.resource.action": phase,
			"hams.provider.name":   name,
		})
	}

	// Set state to pending before executing (install only).
	if phase == phaseInstall {
		sf.SetResource(action.ID, state.StatePending)
	}

	// Run pre-hooks (install and update only).
	if err := runPhasePreHooks(ctx, action, phase); err != nil {
		slog.Error("pre-"+phase+" hook failed, skipping "+phase, "provider", name, "resource", action.ID, "error", err)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(err.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: pre-%s hook %s: %w", name, phase, action.ID, err))
		endSpan(session, span, "error")
		return
	}

	// Execute the action.
	slog.Info(phaseGerund(phase), "provider", name, "resource", action.ID)
	var execErr error
	if phase == phaseRemove {
		execErr = p.Remove(ctx, action.ID)
	} else {
		execErr = p.Apply(ctx, action)
	}
	if execErr != nil {
		slog.Error(phase+" failed", "provider", name, "resource", action.ID, "error", execErr)
		sf.SetResource(action.ID, state.StateFailed, state.WithError(execErr.Error()))
		result.Failed++
		result.Errors = append(result.Errors, fmt.Errorf("%s: %s %s: %w", name, phase, action.ID, execErr))
		endSpan(session, span, "error")
		return
	}

	// Run post-hooks (install and update only). When a hook fails,
	// runPostHooks already calls sf.SetResource with StateHookFailed
	// (see hooks.go:runPostHooks) — we don't duplicate that write
	// here; we just record the error, bump the action counter, and
	// end the span. ComputePlan promotes StateHookFailed to ActionInstall
	// on the next apply, so re-apply semantics stay correct.
	if err := runPhasePostHooks(ctx, action, phase, sf); err != nil {
		incrementCounter(result, phase)
		result.Errors = append(result.Errors, fmt.Errorf("%s: post-%s hook %s: %w", name, phase, action.ID, err))
		slog.Info(phasePastTense(phase)+" (hook failed)", "provider", name, "resource", action.ID)
		endSpan(session, span, "ok")
		return
	}

	// Success.
	if phase == phaseRemove {
		sf.SetResource(action.ID, state.StateRemoved)
	} else {
		sf.SetResource(action.ID, state.StateOK, action.StateOpts...)
	}
	incrementCounter(result, phase)
	slog.Info(phasePastTense(phase), "provider", name, "resource", action.ID)
	endSpan(session, span, "ok")
}

// runPhasePreHooks runs pre-hooks for the given phase. Returns nil if no hooks apply.
func runPhasePreHooks(ctx context.Context, action Action, phase string) error {
	if action.Hooks == nil {
		return nil
	}
	switch phase {
	case phaseInstall:
		if len(action.Hooks.PreInstall) > 0 {
			return RunPreInstallHooks(ctx, action.Hooks.PreInstall, action.ID)
		}
	case phaseUpdate:
		if len(action.Hooks.PreUpdate) > 0 {
			return RunPreUpdateHooks(ctx, action.Hooks.PreUpdate, action.ID)
		}
	}
	return nil
}

// runPhasePostHooks runs post-hooks for the given phase. Returns nil if no hooks apply.
func runPhasePostHooks(ctx context.Context, action Action, phase string, sf *state.File) error {
	if action.Hooks == nil {
		return nil
	}
	switch phase {
	case phaseInstall:
		if len(action.Hooks.PostInstall) > 0 {
			return RunPostInstallHooks(ctx, action.Hooks.PostInstall, action.ID, sf)
		}
	case phaseUpdate:
		if len(action.Hooks.PostUpdate) > 0 {
			return RunPostUpdateHooks(ctx, action.Hooks.PostUpdate, action.ID, sf)
		}
	}
	return nil
}

// endSpan closes an OTel span if the session is active, stamping
// the hams.resource.result attribute to match the status string per
// the observability spec. Status "ok" → result "ok", anything else
// → result "failed".
func endSpan(session *otel.Session, span *otel.Span, status string) {
	if session == nil || span == nil {
		return
	}
	if span.Attrs == nil {
		span.Attrs = map[string]string{}
	}
	result := "ok"
	if status != "ok" {
		result = "failed"
	}
	span.Attrs["hams.resource.result"] = result
	session.EndSpan(span, status)
}

func incrementCounter(result *ExecuteResult, phase string) {
	switch phase {
	case phaseInstall:
		result.Installed++
	case phaseUpdate:
		result.Updated++
	case phaseRemove:
		result.Removed++
	}
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
