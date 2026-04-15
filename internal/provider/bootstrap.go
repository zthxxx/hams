package provider

import (
	"context"
	"errors"
	"fmt"
)

// ErrBootstrapRequired signals that a provider's prerequisite is missing
// and the user has not (yet) consented to running the declared bootstrap
// script. Callers in the CLI layer detect this to prompt for consent or
// surface an actionable error.
var ErrBootstrapRequired = errors.New("bootstrap required")

// BashScriptRunner is the minimal boundary the bootstrap machinery needs
// from whichever provider is named as the host of a DependOn.Script.
// The Bash builtin provider is the canonical implementation.
type BashScriptRunner interface {
	RunScript(ctx context.Context, script string) error
}

type bootstrapAllowedKey struct{}

// WithBootstrapAllowed returns a context carrying the user's consent to
// execute provider bootstrap scripts. Apply/refresh set this based on
// the --bootstrap flag or an affirmative TTY prompt.
func WithBootstrapAllowed(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, bootstrapAllowedKey{}, allowed)
}

// BootstrapAllowed reports whether the caller has consented to running
// bootstrap scripts. Absent a prior WithBootstrapAllowed, returns false.
func BootstrapAllowed(ctx context.Context) bool {
	v, _ := ctx.Value(bootstrapAllowedKey{}).(bool)
	return v
}

// RunBootstrap executes the DependOn.Script entries of a provider's
// manifest via the registered host provider (typically "bash").
// Entries without a Script or whose Platform does not match the current
// OS are skipped. Entries whose host provider is not registered are
// reported as typed errors without executing anything.
func RunBootstrap(ctx context.Context, p Provider, registry *Registry) error {
	manifest := p.Manifest()
	for _, dep := range manifest.DependsOn {
		if !matchesPlatform(dep.Platform) {
			continue
		}
		if dep.Script == "" {
			continue
		}
		host := registry.Get(dep.Provider)
		if host == nil {
			return fmt.Errorf("bootstrap host provider %q not registered for %q", dep.Provider, manifest.Name)
		}
		runner, ok := host.(BashScriptRunner)
		if !ok {
			return fmt.Errorf("bootstrap host %q does not implement BashScriptRunner", dep.Provider)
		}
		if err := runner.RunScript(ctx, dep.Script); err != nil {
			return fmt.Errorf("bootstrap script for %q failed: %w", manifest.Name, err)
		}
	}
	return nil
}
