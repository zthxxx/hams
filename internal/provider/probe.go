package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/zthxxx/hams/internal/state"
)

// ProbeAll runs Probe on all given providers in parallel using errgroup-style coordination.
// Results are merged into the corresponding state files. Probe errors are logged but do not
// stop other providers from probing (best-effort).
func ProbeAll(ctx context.Context, providers []Provider, stateDir, machineID string) map[string]*state.File {
	results := make(map[string]*state.File)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Go(func() {
			name := p.Manifest().Name
			sf := loadOrCreateState(stateDir, name, machineID)

			probeResults, err := p.Probe(ctx, sf)
			if err != nil {
				slog.Warn("probe failed", "provider", name, "error", err)
				return
			}

			// Update state with probe results.
			for _, pr := range probeResults {
				var opts []state.ResourceOption
				if pr.Version != "" {
					opts = append(opts, state.WithVersion(pr.Version))
				}
				if pr.Value != "" {
					opts = append(opts, state.WithValue(pr.Value))
				}
				if pr.Stdout != "" {
					opts = append(opts, state.WithCheckStdout(pr.Stdout))
				}
				if pr.ErrorMsg != "" {
					opts = append(opts, state.WithError(pr.ErrorMsg))
				}
				sf.SetResource(pr.ID, pr.State, opts...)
			}

			mu.Lock()
			results[name] = sf
			mu.Unlock()
		})
	}

	wg.Wait()
	return results
}

func loadOrCreateState(stateDir, providerName, machineID string) *state.File {
	path := fmt.Sprintf("%s/%s.state.yaml", stateDir, providerName)
	sf, err := state.Load(path)
	if err != nil {
		sf = state.New(providerName, machineID)
	}
	return sf
}
