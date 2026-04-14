# sudo DI Isolation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** DI-isolate the `sudo` module so unit tests never trigger real `sudo` prompts, and add a separate Docker-based test target to verify real sudo behavior (root, non-root, non-root-with-NOPASSWD).

**Architecture:** Extract `sudo.Acquirer` interface (lifecycle) and `sudo.CmdBuilder` interface (command construction). Wire them via Uber Fx into `runApply` and `apt.Provider`. Unit tests inject noop/mock implementations. A new `e2e/sudo/` Docker test target exercises real sudo under root and non-root users.

**Tech Stack:** Go 1.25, Uber Fx v1.24, Docker (golang:1.25-bookworm base), `//go:build sudo` tag

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/sudo/sudo.go` | Extract `Acquirer` and `CmdBuilder` interfaces, keep `Manager` as production impl |
| Create | `internal/sudo/noop.go` | `NoopAcquirer` and `DirectBuilder` for unit tests |
| Modify | `internal/sudo/sudo_test.go` | Update tests to use interfaces, remove `isRoot` override hack |
| Create | `internal/sudo/fx.go` | Fx module providing sudo interfaces |
| Modify | `internal/provider/builtin/apt/apt.go` | Accept `sudo.CmdBuilder` via constructor |
| Modify | `internal/provider/builtin/apt/apt_test.go` | Inject `DirectBuilder` in tests |
| Modify | `internal/cli/apply.go` | Accept `sudo.Acquirer` (from Fx or param) instead of hardcoding `NewManager()` |
| Modify | `internal/cli/apply_test.go` | Inject `NoopAcquirer` |
| Modify | `internal/cli/register.go` | Pass `CmdBuilder` to `apt.New()` |
| Modify | `internal/cli/root.go` | Wire Fx for sudo module |
| Create | `internal/sudo/sudo_sudo_test.go` | `//go:build sudo` tests for real sudo behavior |
| Create | `e2e/sudo/Dockerfile` | Docker image with root + non-root user + NOPASSWD sudoers |
| Create | `e2e/sudo/run-tests.sh` | Shell script to run sudo tests as root and non-root |
| Modify | `.github/workflows/ci.yml` | Add `sudo` test job |
| Modify | `Taskfile.yml` | Add `test:sudo` task |
| Modify | `cspell.yaml` | Add new terms if needed |

---

### Task 1: Extract `Acquirer` and `CmdBuilder` interfaces

**Files:**
- Modify: `internal/sudo/sudo.go`

- [x] **Step 1: Read current sudo.go**

Current file has: `Manager` struct, `NewManager()`, `Acquire()`, `IsAcquired()`, `Stop()`, `RunWithSudo()`, `checkSudo()`, `isRoot` var.

- [x] **Step 2: Extract interfaces at top of file**

Add these interfaces before the `Manager` struct:

```go
// Acquirer manages one-time sudo credential acquisition and keepalive.
// Unit tests inject NoopAcquirer; production uses Manager.
type Acquirer interface {
	Acquire(ctx context.Context) error
	Stop()
}

// CmdBuilder constructs exec.Cmd instances with optional sudo wrapping.
// Unit tests inject DirectBuilder; production uses SudoBuilder.
type CmdBuilder interface {
	Command(ctx context.Context, name string, args ...string) *exec.Cmd
}
```

- [x] **Step 3: Rename `RunWithSudo` to a method on a new `SudoBuilder` struct**

Replace the package-level `RunWithSudo` function with:

```go
// SudoBuilder wraps commands with sudo when not running as root.
type SudoBuilder struct{}

// Command returns an exec.Cmd, prepending sudo if not root.
func (s *SudoBuilder) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if isRoot() {
		return exec.CommandContext(ctx, name, args...) //nolint:gosec // root-skip path; args from hamsfile declarations
	}
	sudoArgs := make([]string, 0, len(args)+1)
	sudoArgs = append(sudoArgs, name)
	sudoArgs = append(sudoArgs, args...)
	return exec.CommandContext(ctx, "sudo", sudoArgs...) //nolint:gosec // sudo wrapping is intentional; args come from provider declarations not user input
}
```

Keep `RunWithSudo` as a deprecated wrapper for backward compatibility during the transition:

```go
// RunWithSudo is deprecated; use SudoBuilder.Command instead.
// Kept temporarily for any callers not yet migrated to CmdBuilder DI.
func RunWithSudo(ctx context.Context, name string, args ...string) *exec.Cmd {
	return (&SudoBuilder{}).Command(ctx, name, args...)
}
```

- [x] **Step 4: Verify compile**

Run: `go build ./internal/sudo/...`
Expected: PASS (no callers changed yet, backward-compat wrapper keeps things working)

- [x] **Step 5: Commit**

```bash
git add internal/sudo/sudo.go
git commit -m "refactor(sudo): extract Acquirer and CmdBuilder interfaces"
```

---

### Task 2: Create noop/mock implementations

**Files:**
- Create: `internal/sudo/noop.go`

- [x] **Step 1: Write the noop implementations**

```go
package sudo

import (
	"context"
	"os/exec"
)

// NoopAcquirer is a no-op Acquirer for unit tests.
// It never prompts for sudo and always succeeds.
type NoopAcquirer struct{}

// Acquire is a no-op that always returns nil.
func (NoopAcquirer) Acquire(context.Context) error { return nil }

// Stop is a no-op.
func (NoopAcquirer) Stop() {}

// DirectBuilder runs commands directly without sudo wrapping.
// Used in unit tests to avoid privilege escalation.
type DirectBuilder struct{}

// Command returns an exec.Cmd without sudo wrapping.
func (DirectBuilder) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...) //nolint:gosec // test-only direct execution
}
```

- [x] **Step 2: Verify compile**

Run: `go build ./internal/sudo/...`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add internal/sudo/noop.go
git commit -m "feat(sudo): add NoopAcquirer and DirectBuilder for unit test DI"
```

---

### Task 3: Create Fx module for sudo

**Files:**
- Create: `internal/sudo/fx.go`

- [x] **Step 1: Write the Fx module**

```go
package sudo

import "go.uber.org/fx"

// Module provides production sudo implementations via Fx.
var Module = fx.Module("sudo",
	fx.Provide(
		fx.Annotate(
			func() *Manager { return NewManager() },
			fx.As(new(Acquirer)),
		),
		fx.Annotate(
			func() *SudoBuilder { return &SudoBuilder{} },
			fx.As(new(CmdBuilder)),
		),
	),
)

// TestModule provides noop sudo implementations for unit tests.
var TestModule = fx.Module("sudo-test",
	fx.Provide(
		fx.Annotate(
			func() NoopAcquirer { return NoopAcquirer{} },
			fx.As(new(Acquirer)),
		),
		fx.Annotate(
			func() DirectBuilder { return DirectBuilder{} },
			fx.As(new(CmdBuilder)),
		),
	),
)
```

- [x] **Step 2: Verify compile**

Run: `go build ./internal/sudo/...`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add internal/sudo/fx.go
git commit -m "feat(sudo): add Fx Module and TestModule for DI wiring"
```

---

### Task 4: Inject `CmdBuilder` into apt provider

**Files:**
- Modify: `internal/provider/builtin/apt/apt.go`
- Modify: `internal/provider/builtin/apt/apt_test.go`

- [x] **Step 1: Update apt.Provider to accept CmdBuilder**

Change the Provider struct and constructor:

```go
// Provider implements the APT package manager provider.
type Provider struct {
	sudo sudo.CmdBuilder
}

// New creates a new apt provider with the given sudo command builder.
func New(sb sudo.CmdBuilder) *Provider { return &Provider{sudo: sb} }
```

- [x] **Step 2: Replace all `sudo.RunWithSudo` calls with `p.sudo.Command`**

In `Apply`:
```go
func (p *Provider) Apply(ctx context.Context, action provider.Action) error {
	slog.Info("apt install", "package", action.ID)
	cmd := p.sudo.Command(ctx, "apt-get", "install", "-y", action.ID)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
```

In `Remove`:
```go
func (p *Provider) Remove(ctx context.Context, resourceID string) error {
	slog.Info("apt remove", "package", resourceID)
	cmd := p.sudo.Command(ctx, "apt-get", "remove", "-y", resourceID)
	return cmd.Run()
}
```

In `HandleCommand` install case:
```go
cmd := p.sudo.Command(context.Background(), "apt-get", append([]string{"install", "-y"}, remaining...)...)
```

In `HandleCommand` remove case:
```go
cmd := p.sudo.Command(context.Background(), "apt-get", append([]string{"remove", "-y"}, remaining...)...)
```

- [x] **Step 3: Remove the `sudo` import (no longer needed as package-level call)**

The import changes from `"github.com/zthxxx/hams/internal/sudo"` — keep it, since we use the `sudo.CmdBuilder` type.

- [x] **Step 4: Update apt tests to inject DirectBuilder**

In `apt_test.go`, update any test that creates `apt.New()` to pass `sudo.DirectBuilder{}`:

```go
import "github.com/zthxxx/hams/internal/sudo"

func TestManifest(t *testing.T) {
	p := New(sudo.DirectBuilder{})
	// ... rest unchanged
}
```

(Do this for all `New()` calls in `apt_test.go` — currently `TestManifest`, `TestNameDisplayName`, and `TestParseDpkgVersion` which doesn't use New.)

- [x] **Step 5: Verify compile and tests**

Run: `go build ./internal/provider/builtin/apt/... && go test ./internal/provider/builtin/apt/...`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add internal/provider/builtin/apt/apt.go internal/provider/builtin/apt/apt_test.go
git commit -m "refactor(apt): inject sudo.CmdBuilder via constructor"
```

---

### Task 5: Update register.go to pass CmdBuilder to apt

**Files:**
- Modify: `internal/cli/register.go`

- [x] **Step 1: Add sudo CmdBuilder parameter to registerBuiltins**

Change signature and pass `SudoBuilder` to apt:

```go
func registerBuiltins(registry *provider.Registry, sudoCmd sudo.CmdBuilder) {
```

Update the `apt.New()` call inside:
```go
apt.New(sudoCmd),
```

- [x] **Step 2: Verify compile**

Run: `go build ./internal/cli/...`
Expected: FAIL — callers of `registerBuiltins` need updating (done in Task 6)

- [x] **Step 3: Do NOT commit yet** — wait for Task 6 to make it compile

---

### Task 6: Inject Acquirer into runApply and wire Fx in root.go

**Files:**
- Modify: `internal/cli/apply.go`
- Modify: `internal/cli/root.go`

- [x] **Step 1: Add Acquirer parameter to runApply**

Change `runApply` signature to accept an `Acquirer`:

```go
func runApply(ctx context.Context, flags *provider.GlobalFlags, registry *provider.Registry, sudoAcq sudo.Acquirer, fromRepo string, noRefresh bool, only, except string) error {
```

Replace lines 128-129:
```go
sudoMgr := sudo.NewManager()
defer sudoMgr.Stop()
```
with:
```go
defer sudoAcq.Stop()
```

Replace line 188:
```go
if sudoErr := sudoMgr.Acquire(ctx); sudoErr != nil {
```
with:
```go
if sudoErr := sudoAcq.Acquire(ctx); sudoErr != nil {
```

- [x] **Step 2: Update applyCmd to pass Acquirer**

Change `applyCmd` to accept and forward the Acquirer:

```go
func applyCmd(registry *provider.Registry, sudoAcq sudo.Acquirer) *cli.Command {
```

Update the Action closure:
```go
Action: func(ctx context.Context, cmd *cli.Command) error {
	flags := globalFlags(cmd)
	return runApply(ctx, flags, registry, sudoAcq,
		cmd.String("from-repo"),
		cmd.Bool("no-refresh"),
		cmd.String("only"),
		cmd.String("except"),
	)
},
```

- [x] **Step 3: Update NewApp to accept and forward sudo types**

```go
func NewApp(registry *provider.Registry, sudoAcq sudo.Acquirer) *cli.Command {
```

Update the `applyCmd` call:
```go
Commands: []*cli.Command{
	applyCmd(registry, sudoAcq),
	// ... rest unchanged
},
```

- [x] **Step 4: Update Execute to wire Fx**

Replace the current `Execute()` function:

```go
func Execute() {
	i18n.Init()

	app := fx.New(
		sudo.Module,
		fx.Provide(func() *provider.Registry {
			return provider.NewRegistry()
		}),
		fx.Invoke(func(registry *provider.Registry, sudoAcq sudo.Acquirer, sudoCmd sudo.CmdBuilder) {
			registerBuiltins(registry, sudoCmd)

			cliApp := NewApp(registry, sudoAcq)

			if err := cliApp.Run(context.Background(), os.Args); err != nil {
				flags := &provider.GlobalFlags{}
				for _, arg := range os.Args {
					if arg == "--json" {
						flags.JSON = true
					}
				}
				PrintError(err, flags.JSON)

				exitCode := hamserr.ExitGeneralError
				var ue *hamserr.UserFacingError
				if errors.As(err, &ue) {
					exitCode = ue.Code
				}
				os.Exit(exitCode)
			}
		}),
	)

	app.Done()
}
```

Wait — Fx's lifecycle is designed for long-running services, not CLI tools. The `fx.New` + `app.Done()` pattern doesn't work well for CLIs. Instead, use `fx.New` with `fx.NopLogger` and extract dependencies via `fx.Populate`:

```go
func Execute() {
	i18n.Init()

	var sudoAcq sudo.Acquirer
	var sudoCmd sudo.CmdBuilder

	fxApp := fx.New(
		fx.NopLogger,
		sudo.Module,
		fx.Populate(&sudoAcq, &sudoCmd),
	)
	if err := fxApp.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "initialization error: %v\n", err)
		os.Exit(1)
	}

	registry := provider.NewRegistry()
	registerBuiltins(registry, sudoCmd)

	app := NewApp(registry, sudoAcq)

	if err := app.Run(context.Background(), os.Args); err != nil {
		flags := &provider.GlobalFlags{}
		for _, arg := range os.Args {
			if arg == "--json" {
				flags.JSON = true
			}
		}
		PrintError(err, flags.JSON)

		exitCode := hamserr.ExitGeneralError
		var ue *hamserr.UserFacingError
		if errors.As(err, &ue) {
			exitCode = ue.Code
		}
		os.Exit(exitCode)
	}
}
```

- [x] **Step 5: Update root_test.go to pass NoopAcquirer**

`root_test.go` calls `NewApp(registry)` in 3 tests. Update all to:

```go
import "github.com/zthxxx/hams/internal/sudo"

// In each test:
app := NewApp(registry, sudo.NoopAcquirer{})
```

- [x] **Step 6: Verify compile**

Run: `go build ./cmd/hams/...`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add internal/cli/apply.go internal/cli/root.go internal/cli/register.go internal/cli/root_test.go
git commit -m "refactor(cli): wire sudo via Fx DI, inject Acquirer into runApply"
```

---

### Task 7: Update apply_test.go to inject NoopAcquirer

**Files:**
- Modify: `internal/cli/apply_test.go`

- [x] **Step 1: Update all runApply calls to pass NoopAcquirer**

Add import:
```go
"github.com/zthxxx/hams/internal/sudo"
```

Every `runApply(context.Background(), flags, registry, "", true, "", "")` becomes:
```go
runApply(context.Background(), flags, registry, sudo.NoopAcquirer{}, "", true, "", "")
```

There are 4 such calls in the current test file (lines 114, 184, 188, 258).

- [x] **Step 2: Run tests**

Run: `go test -race ./internal/cli/...`
Expected: PASS — no more `Password:` prompt

- [x] **Step 3: Commit**

```bash
git add internal/cli/apply_test.go
git commit -m "test(cli): inject NoopAcquirer in apply tests to prevent sudo prompt"
```

---

### Task 8: Update sudo_test.go to use interfaces

**Files:**
- Modify: `internal/sudo/sudo_test.go`

- [x] **Step 1: Add interface compliance tests**

```go
func TestManager_ImplementsAcquirer(t *testing.T) {
	var _ Acquirer = (*Manager)(nil)
}

func TestSudoBuilder_ImplementsCmdBuilder(t *testing.T) {
	var _ CmdBuilder = (*SudoBuilder)(nil)
}

func TestNoopAcquirer_ImplementsAcquirer(t *testing.T) {
	var _ Acquirer = NoopAcquirer{}
}

func TestDirectBuilder_ImplementsCmdBuilder(t *testing.T) {
	var _ CmdBuilder = DirectBuilder{}
}
```

- [x] **Step 2: Update existing RunWithSudo tests to use SudoBuilder**

Replace `TestRunWithSudo_NonRoot_WrapsSudo`:
```go
func TestSudoBuilder_NonRoot_WrapsSudo(t *testing.T) {
	orig := isRoot
	isRoot = func() bool { return false }
	t.Cleanup(func() { isRoot = orig })

	sb := &SudoBuilder{}
	cmd := sb.Command(context.Background(), "ls", "-la")
	args := cmd.Args
	if len(args) != 3 || args[0] != "sudo" || args[1] != "ls" || args[2] != "-la" {
		t.Errorf("Args = %v, want [sudo ls -la]", args)
	}
}
```

Replace `TestRunWithSudo_Root_SkipsSudo`:
```go
func TestSudoBuilder_Root_SkipsSudo(t *testing.T) {
	orig := isRoot
	isRoot = func() bool { return true }
	t.Cleanup(func() { isRoot = orig })

	sb := &SudoBuilder{}
	cmd := sb.Command(context.Background(), "ls", "-la")
	args := cmd.Args
	if len(args) != 2 || args[0] != "ls" || args[1] != "-la" {
		t.Errorf("Args = %v, want [ls -la] when running as root", args)
	}
}
```

- [x] **Step 3: Add DirectBuilder test**

```go
func TestDirectBuilder_NeverWrapsSudo(t *testing.T) {
	db := DirectBuilder{}
	cmd := db.Command(context.Background(), "ls", "-la")
	args := cmd.Args
	if len(args) != 2 || args[0] != "ls" || args[1] != "-la" {
		t.Errorf("Args = %v, want [ls -la]", args)
	}
}
```

- [x] **Step 4: Add NoopAcquirer test**

```go
func TestNoopAcquirer_AlwaysSucceeds(t *testing.T) {
	na := NoopAcquirer{}
	if err := na.Acquire(context.Background()); err != nil {
		t.Fatalf("NoopAcquirer.Acquire should always succeed: %v", err)
	}
	na.Stop() // Should not panic.
}
```

- [x] **Step 5: Run tests**

Run: `go test -race ./internal/sudo/...`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add internal/sudo/sudo_test.go
git commit -m "test(sudo): update tests to use Acquirer/CmdBuilder interfaces"
```

---

### Task 9: Remove deprecated `RunWithSudo` wrapper

**Files:**
- Modify: `internal/sudo/sudo.go`

- [x] **Step 1: Grep for remaining callers**

Run: `rg 'sudo\.RunWithSudo' --type go`
Expected: No matches (all callers migrated in Tasks 4-6)

- [x] **Step 2: Remove the deprecated function**

Delete the `RunWithSudo` function from `sudo.go`.

- [x] **Step 3: Verify full build and tests**

Run: `go build ./... && go test -race ./...`
Expected: PASS, no `Password:` prompt

- [x] **Step 4: Commit**

```bash
git add internal/sudo/sudo.go
git commit -m "refactor(sudo): remove deprecated RunWithSudo function"
```

---

### Task 10: Create Docker-based sudo tests

**Files:**
- Create: `internal/sudo/sudo_sudo_test.go` (with `//go:build sudo` tag)
- Create: `e2e/sudo/Dockerfile`
- Create: `e2e/sudo/run-tests.sh`

- [x] **Step 1: Write the build-tagged test file**

```go
//go:build sudo

package sudo

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests run inside Docker containers where sudo is available.
// Build tag "sudo" ensures they never run in normal `go test ./...`.

func TestAcquire_AsRoot_Succeeds(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root")
	}

	m := NewManager()
	defer m.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Acquire(ctx); err != nil {
		t.Fatalf("Acquire as root should succeed: %v", err)
	}
	if !m.IsAcquired() {
		t.Error("expected acquired = true after Acquire as root")
	}
}

func TestAcquire_AsNonRoot_WithNOPASSWD_Succeeds(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user with NOPASSWD sudoers")
	}

	m := NewManager()
	defer m.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Acquire(ctx); err != nil {
		t.Fatalf("Acquire with NOPASSWD should succeed: %v", err)
	}
	if !m.IsAcquired() {
		t.Error("expected acquired = true")
	}
}

func TestSudoBuilder_AsRoot_SkipsSudo(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test requires root")
	}

	sb := &SudoBuilder{}
	cmd := sb.Command(context.Background(), "id", "-u")
	args := cmd.Args
	// As root, should NOT prepend sudo.
	if args[0] == "sudo" {
		t.Errorf("expected no sudo prefix when running as root, got %v", args)
	}

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if string(out) == "" {
		t.Error("expected output from id -u")
	}
}

func TestSudoBuilder_AsNonRoot_PrependsSudo(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	sb := &SudoBuilder{}
	cmd := sb.Command(context.Background(), "id", "-u")
	args := cmd.Args
	if args[0] != "sudo" {
		t.Errorf("expected sudo prefix when non-root, got %v", args)
	}

	// With NOPASSWD, this should actually execute.
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sudo command failed (NOPASSWD not configured?): %v", err)
	}
	// sudo id -u should return "0".
	if got := string(out); got != "0\n" {
		t.Errorf("sudo id -u = %q, want %q", got, "0\n")
	}
}
```

- [x] **Step 2: Write the Dockerfile**

```dockerfile
FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends sudo && rm -rf /var/lib/apt/lists/*

# Create a non-root user with NOPASSWD sudo access.
RUN useradd -m -s /bin/bash testuser \
    && echo "testuser ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/testuser \
    && chmod 0440 /etc/sudoers.d/testuser

WORKDIR /src

# Cache Go module downloads; source code is bind-mounted at runtime via -v.
COPY go.mod go.sum ./
RUN go mod download
```

- [x] **Step 3: Write the test runner script**

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "=== Running sudo tests as root ==="
go test -v -tags=sudo -race -count=1 -run 'TestAcquire_AsRoot|TestSudoBuilder_AsRoot' ./internal/sudo/...

echo ""
echo "=== Running sudo tests as non-root (testuser with NOPASSWD) ==="
su testuser -c 'cd /src && go test -v -tags=sudo -race -count=1 -run "TestAcquire_AsNonRoot|TestSudoBuilder_AsNonRoot" ./internal/sudo/...'

echo ""
echo "All sudo tests passed."
```

- [x] **Step 4: Verify Dockerfile builds**

Run: `docker build -f e2e/sudo/Dockerfile -t hams-sudo-test .`
Expected: Image builds successfully

- [x] **Step 5: Run the sudo tests in Docker**

Run: `docker run --rm -v "$(pwd):/src" hams-sudo-test bash /src/e2e/sudo/run-tests.sh`
Expected: All 4 tests pass (2 as root, 2 as testuser)

- [x] **Step 6: Commit**

```bash
git add internal/sudo/sudo_sudo_test.go e2e/sudo/Dockerfile e2e/sudo/run-tests.sh
git commit -m "test(sudo): add Docker-based tests for real sudo behavior"
```

---

### Task 11: Add CI job and Taskfile entry for sudo tests

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Taskfile.yml`

- [x] **Step 1: Add sudo test job to CI**

Add after the `integration:` job in `ci.yml`:

```yaml
  sudo:
    name: Sudo Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build sudo test image (cached by content hash)
        run: |
          HASH=$(cat e2e/sudo/Dockerfile go.mod go.sum | sha256sum | head -c 12)
          IMAGE="hams-sudo:${HASH}"
          if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
            docker build -f e2e/sudo/Dockerfile -t "$IMAGE" .
            docker images --format '{{.Repository}}:{{.Tag}}' \
              | grep '^hams-sudo:' | grep -v ":${HASH}$" \
              | xargs -r docker rmi 2>/dev/null || true
          fi
          echo "SUDO_IMAGE=$IMAGE" >> "$GITHUB_ENV"

      - name: Run sudo tests
        run: |
          docker run --rm \
            -v "$(pwd):/src" \
            "$SUDO_IMAGE" \
            bash /src/e2e/sudo/run-tests.sh
```

- [x] **Step 2: Add Taskfile entry**

Add to `Taskfile.yml` after `test:integration:`:

```yaml
  test:sudo:
    desc: Run sudo DI tests in Docker
    cmds:
      - act push --container-architecture linux/amd64 -j sudo --rm
```

- [x] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml Taskfile.yml
git commit -m "ci: add sudo test job and task"
```

---

### Task 12: Final verification

- [x] **Step 1: Run full unit test suite — confirm no Password prompt**

Run: `go test -race -count=1 ./...`
Expected: All tests PASS. No `Password:` prompt. No interactive blocking.

- [x] **Step 2: Run lint**

Run: `golangci-lint run ./...`
Expected: PASS

- [x] **Step 3: Run sudo Docker tests**

Run: `docker run --rm -v "$(pwd):/src" hams-sudo-test bash /src/e2e/sudo/run-tests.sh`
Expected: All 4 sudo tests pass

- [x] **Step 4: Add any new terms to cspell.yaml if needed**

Check: `bun cspell lint --no-progress "internal/sudo/**"`

- [x] **Step 5: Verify no stale references**

Run: `rg 'sudo\.RunWithSudo' --type go`
Expected: No matches

Run: `rg 'sudo\.NewManager' --type go`
Expected: Only in `internal/sudo/sudo.go` (the constructor) and `internal/sudo/fx.go` (the Fx provider)

- [x] **Step 6: Commit any final fixes**

```bash
git add -A
git commit -m "chore: final cleanup after sudo DI isolation"
```
