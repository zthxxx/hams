package provider

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/zthxxx/hams/internal/state"
)

// ProbeAll runs Probe on all given providers in parallel using errgroup coordination.
// Results are merged into the corresponding state files. Probe errors are logged but do not
// stop other providers from probing (best-effort).
func ProbeAll(ctx context.Context, providers []Provider, stateDir, machineID string) map[string]*state.File {
	results := make(map[string]*state.File)
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)

	for _, p := range providers {
		g.Go(func() error {
			manifest := p.Manifest()
			name := manifest.Name
			filePrefix := manifest.FilePrefix
			if filePrefix == "" {
				filePrefix = name
			}
			sf := loadOrCreateState(stateDir, filePrefix, name, machineID)

			probeResults, err := p.Probe(ctx, sf)
			if err != nil {
				slog.Warn("probe failed", "provider", name, "error", err)
				return nil // Best-effort: log but don't abort other probes.
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
			results[filePrefix] = sf
			mu.Unlock()
			return nil
		})
	}

	_ = g.Wait() // Errors already logged per-provider.
	return results
}

func loadOrCreateState(stateDir, filePrefix, providerName, machineID string) *state.File {
	path := filepath.Join(stateDir, filePrefix+".state.yaml")
	sf, err := state.Load(path)
	if err != nil {
		sf = state.New(providerName, machineID)
	}
	return sf
}
