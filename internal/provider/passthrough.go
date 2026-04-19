package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PassthroughExec is the DI seam for running an arbitrary CLI
// transparently. Tests swap this to a fake that records the invocation
// without spawning a real process. The production value execs the
// target binary with stdin/stdout/stderr + exit code preserved.
//
// The package-level seam is intentionally shared across every CLI-
// wrapping provider (brew, apt, npm, pnpm, cargo, uv, goinstall, mas,
// code, …). A provider that adopts provider.Passthrough also picks up
// the seam for free — one test fake swap-out covers all of them.
//
//nolint:gochecknoglobals // DI seam; documented contract, only swapped in tests.
var PassthroughExec = func(ctx context.Context, tool string, args []string) error {
	cmd := exec.CommandContext(ctx, tool, args...) //nolint:gosec // args flow from the hams CLI; passthrough is the intended behavior
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Passthrough runs `tool` with `args` via PassthroughExec, preserving
// stdin, stdout, stderr, and exit code. It is the canonical default
// branch for every builtin provider's HandleCommand — when a user
// types `hams brew upgrade htop` or `hams apt list --installed` or
// `hams pnpm outdated`, the real tool runs exactly as it would
// outside hams.
//
// When flags.DryRun is set, Passthrough prints
// `[dry-run] Would run: <tool> <args>` to flags.Stdout() and returns
// nil without exec. This matches the contract established by
// git/unified.go::passthrough.
func Passthrough(ctx context.Context, tool string, args []string, flags *GlobalFlags) error {
	if flags != nil && flags.DryRun {
		if len(args) == 0 {
			fmt.Fprintf(flags.Stdout(), "[dry-run] Would run: %s\n", tool)
		} else {
			fmt.Fprintf(flags.Stdout(), "[dry-run] Would run: %s %s\n", tool, strings.Join(args, " "))
		}
		return nil
	}
	return PassthroughExec(ctx, tool, args)
}
