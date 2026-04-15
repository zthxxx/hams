package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const defaultDebounce = 500 * time.Millisecond

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(&cfg, logger); err != nil {
		logger.Error("watcher exited with error", "err", err)
		os.Exit(1)
	}
}

type config struct {
	arch     string
	roots    []string
	output   string
	pkg      string
	repoRoot string
	debounce time.Duration
}

func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	arch := fs.String("arch", "", `target linux GOARCH, one of "amd64" or "arm64" (required)`)
	debounce := fs.Duration("debounce", defaultDebounce, "quiet window before a rebuild fires")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if err := validateArch(*arch); err != nil {
		return config{}, err
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		return config{}, fmt.Errorf("watch: getwd: %w", err)
	}
	output := filepath.Join("bin", "hams-linux-"+*arch)
	return config{
		arch:     *arch,
		roots:    []string{"cmd", "internal", "pkg"},
		output:   output,
		pkg:      "./cmd/hams",
		repoRoot: repoRoot,
		debounce: *debounce,
	}, nil
}

func validateArch(arch string) error {
	switch arch {
	case "amd64", "arm64":
		return nil
	case "":
		return fmt.Errorf("--arch is required (use amd64 or arm64)")
	default:
		return fmt.Errorf("--arch %q is not supported (use amd64 or arm64)", arch)
	}
}

func run(cfg *config, logger *slog.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	roots := filterExistingRoots(cfg.roots, logger)
	if len(roots) == 0 {
		return fmt.Errorf("watch: no watchable directories found under %s", cfg.repoRoot)
	}

	notifier, err := NewFSNotifier(roots, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := notifier.Close(); err != nil {
			logger.Warn("watch: close notifier", "err", err)
		}
	}()

	builder := NewGoBuilder(cfg.arch, cfg.output, cfg.pkg, cfg.repoRoot)
	reporter := NewSlogReporter(logger)
	engine := NewEngine(cfg.debounce, RealClock(), builder, reporter)

	events := make(chan struct{}, 8)

	// Kick a build on startup so the sandbox image has a fresh binary as
	// soon as the container comes up, without waiting for the first save.
	go func() {
		select {
		case events <- struct{}{}:
		case <-ctx.Done():
		}
	}()

	go notifier.Run(ctx, events)
	engine.Run(ctx, events)
	return nil
}

func filterExistingRoots(roots []string, logger *slog.Logger) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		//nolint:gosec // roots come from the watcher's compile-time default list, not user input.
		if info, err := os.Stat(r); err == nil && info.IsDir() {
			out = append(out, r)
			continue
		}
		logger.Debug("watch: skipping missing root", "path", r)
	}
	return out
}
