package provider

import (
	"context"
	"errors"
	"io/fs"
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
		g.Go(func() (err error) {
			manifest := p.Manifest()
			name := manifest.Name
			filePrefix := manifest.FilePrefix
			if filePrefix == "" {
				filePrefix = name
			}

			// Recover from a panicking Probe — a buggy provider must
			// not crash the whole refresh and take down parallel probes
			// for healthy providers. Log and omit from results so
			// runRefresh reports the probed/planned mismatch.
			defer func() {
				if r := recover(); r != nil {
					slog.Error("probe panicked; provider omitted from results",
						"provider", name, "panic", r)
				}
			}()

			sf, loadErr := loadOrCreateState(stateDir, filePrefix, name, machineID)
			if loadErr != nil {
				// Corrupted state file (not just missing). Skip this
				// provider rather than probe against an empty synthesized
				// state that would mark every real resource as "pending
				// install". Surface to runRefresh via the results-map
				// absence (runRefresh reports probed/planned mismatch).
				slog.Error("skipping probe — state file unreadable",
					"provider", name, "error", loadErr)
				return nil
			}

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

	if err := g.Wait(); err != nil {
		// Per-provider errors are already logged inside g.Go; nothing returns a non-nil error,
		// so this path is only reached if the errgroup itself fails (e.g., ctx canceled).
		slog.Debug("probe errgroup returned error", "error", err)
	}
	return results
}

// loadOrCreateState returns the loaded state file, or a fresh empty
// one if the file does not exist. A non-ErrNotExist error (parse
// failure, permission denied) is propagated so the caller can skip
// the provider instead of silently resetting its state. Ambiguous
// "fail silently" behavior lost drift-tracked resources on the first
// apply after a merge conflict or editor crash corrupted the YAML.
func loadOrCreateState(stateDir, filePrefix, providerName, machineID string) (*state.File, error) {
	path := filepath.Join(stateDir, filePrefix+".state.yaml")
	sf, err := state.Load(path)
	if err == nil {
		return sf, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return state.New(providerName, machineID), nil
	}
	return nil, err
}
