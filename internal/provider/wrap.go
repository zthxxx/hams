package provider

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/zthxxx/hams/internal/runner"
)

// WrapExec runs a wrapped CLI command, forwarding args and capturing output.
// It handles auto-injection of flags (e.g., -g for pnpm, -y for apt).
func WrapExec(ctx context.Context, command string, args []string, autoInject map[string]string) ([]byte, error) {
	finalArgs := injectFlags(args, autoInject)

	slog.Debug("executing wrapped command", "command", command, "args", finalArgs)
	cmd := exec.CommandContext(ctx, command, finalArgs...) //nolint:gosec // provider wraps user-configured CLI tools
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return output, fmt.Errorf("command %s %s failed: %w", command, strings.Join(finalArgs, " "), err)
	}

	return output, nil
}

// WrapExecPassthrough runs a wrapped CLI command with stdin/stdout/stderr connected.
// Used when the wrapped command needs interactive output (e.g., brew install progress).
func WrapExecPassthrough(ctx context.Context, command string, args []string, autoInject map[string]string) error {
	finalArgs := injectFlags(args, autoInject)

	slog.Debug("executing wrapped command (passthrough)", "command", command, "args", finalArgs)
	cmd := exec.CommandContext(ctx, command, finalArgs...) //nolint:gosec // provider wraps user-configured CLI tools
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// WrapExecWithRunner runs a wrapped CLI command using a Runner interface for DI testability.
func WrapExecWithRunner(ctx context.Context, r runner.Runner, command string, args []string, autoInject map[string]string) ([]byte, error) {
	finalArgs := injectFlags(args, autoInject)
	slog.Debug("executing wrapped command (runner)", "command", command, "args", finalArgs)
	return r.Run(ctx, command, finalArgs...)
}

// WrapExecPassthroughWithRunner runs a wrapped CLI command with terminal I/O via a Runner.
func WrapExecPassthroughWithRunner(ctx context.Context, r runner.Runner, command string, args []string, autoInject map[string]string) error {
	finalArgs := injectFlags(args, autoInject)
	slog.Debug("executing wrapped command (passthrough runner)", "command", command, "args", finalArgs)
	return r.RunPassthrough(ctx, command, finalArgs...)
}

// injectFlags adds auto-inject flags if they're not already present.
// autoInject maps flag names to their values (empty string for boolean flags).
// Example: {"--global": "", "-y": ""} for pnpm and apt respectively.
func injectFlags(args []string, autoInject map[string]string) []string {
	if len(autoInject) == 0 {
		return args
	}

	existing := make(map[string]bool)
	for _, arg := range args {
		existing[arg] = true
	}

	var injected []string
	for flag, value := range autoInject {
		if existing[flag] {
			continue
		}
		if value == "" {
			injected = append(injected, flag)
		} else {
			injected = append(injected, flag+"="+value)
		}
	}

	return append(args, injected...)
}

// ParseVerb extracts the verb (first non-flag argument) from args.
// Returns (verb, remainingArgs). The returned remaining slice is a new
// allocation and never mutates the caller's underlying array.
func ParseVerb(args []string) (verb string, remaining []string) {
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			remaining = make([]string, 0, len(args)-1)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+1:]...)
			return arg, remaining
		}
	}
	return "", args
}
