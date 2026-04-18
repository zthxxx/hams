package goinstall

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CmdRunner is the DI seam for every outbound invocation of `go`.
// Probe's Go-binary-presence check involves multiple commands
// (`go env GOPATH`, then exec'ing the binary, then PATH fallback) so
// the seam exposes a single IsBinaryInstalled to keep the provider
// code simple while letting tests stub the multi-step probe.
//
// The Uninstall method is a deliberate no-op here: `go install` has
// no inverse verb, so the goinstall provider cannot remove binaries
// programmatically. Keeping Uninstall on the interface (returning nil)
// lets goinstall fit `provider.PackageInstaller` and adopt the
// `provider.AutoRecordInstall` shared dispatcher without a second
// install-only dispatcher variant. The provider-level `Remove` method
// continues to warn the user to delete the binary manually.
type CmdRunner interface {
	// Install runs `go install <pkg>` (caller has already injected
	// @latest if no version was specified).
	Install(ctx context.Context, pkg string) error

	// Uninstall is a no-op returning nil, matching the documented
	// "go install has no uninstall verb" contract. Present only so
	// goinstall can satisfy PackageInstaller.
	Uninstall(ctx context.Context, pkg string) error

	// IsBinaryInstalled reports whether the binary derived from pkg
	// (last path segment, before @version) is present and executable.
	// Production:
	//   1. Determine binary name from pkg path
	//   2. Check $GOPATH/bin/<name> --version
	//   3. Fall back to exec.LookPath(<name>)
	IsBinaryInstalled(ctx context.Context, pkg string) bool

	// LookPath verifies `go` itself is on $PATH.
	LookPath() error
}

// NewRealCmdRunner returns the production CmdRunner.
func NewRealCmdRunner() CmdRunner {
	return &realCmdRunner{}
}

type realCmdRunner struct{}

func (r *realCmdRunner) Install(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "go", "install", pkg) //nolint:gosec // pkg sourced from hamsfile/state entries
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install %s: %w", pkg, err)
	}
	return nil
}

func (r *realCmdRunner) Uninstall(_ context.Context, _ string) error {
	// go install has no uninstall verb. Returning nil keeps the
	// PackageInstaller contract honest without actually touching the
	// host. The provider-level Remove method warns the user.
	return nil
}

func (r *realCmdRunner) IsBinaryInstalled(ctx context.Context, pkg string) bool {
	binName := binaryNameFromPkg(pkg)
	if binName == "" {
		return false
	}

	// First: try $GOPATH/bin/<name> --version (works for binaries
	// installed via go install).
	gopathCmd := exec.CommandContext(ctx, "go", "env", "GOPATH")
	out, err := gopathCmd.Output()
	if err == nil {
		gopath := strings.TrimSpace(string(out))
		binPath := filepath.Join(gopath, "bin", binName)
		check := exec.CommandContext(ctx, binPath, "--version") //nolint:gosec // path derived from GOPATH + tracked binary name
		if check.Run() == nil {
			return true
		}
	}

	// Fallback: maybe the binary is on PATH via some other route.
	if _, lookErr := exec.LookPath(binName); lookErr == nil {
		return true
	}
	return false
}

func (r *realCmdRunner) LookPath() error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go not found in PATH")
	}
	return nil
}

// binaryNameFromPkg derives the install-time binary name from a Go
// module path: drop everything after the optional `@version`, then
// take the last `/`-separated segment. Returns "" for malformed input.
func binaryNameFromPkg(pkg string) string {
	pkg = strings.SplitN(pkg, "@", 2)[0]
	if pkg == "" {
		return ""
	}
	parts := strings.Split(pkg, "/")
	return parts[len(parts)-1]
}
