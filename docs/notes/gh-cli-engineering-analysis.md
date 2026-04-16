# gh-cli Engineering Structure Analysis & Comparison Report

> Target: `/workspaces/gh-cli/` ([cli/cli](https://github.com/cli/cli) `v2` trunk)
> Compared against: current project `hams` (`/workspaces/hams/`)
> Date: 2026-04-15

---

## TL;DR

`gh-cli` is a highly mature Go CLI project with 5+ years of iteration: ~**806 Go files**, 673 files of command implementations, a 9,700-LoC API layer. Three defining traits:

1. **Factory-based lightweight DI** — no Uber Fx / wire / dig. A single `*cmdutil.Factory` struct holds 11 lazy-evaluated closures (`func() (X, error)`). Any field is trivially replaceable in tests, with zero startup overhead.
2. **Flat command tree + Cobra** — every subcommand lives in `pkg/cmd/<command>/<subcommand>/` and follows a three-part template: `Options` struct + `NewCmdX(f, runF)` + `xxxRun(opts)`. The `runF` hook exists specifically to give unit tests a direct injection point.
3. **Radical HTTP mocking** — a home-grown `pkg/httpmock.Registry` intercepts every REST/GraphQL call to GitHub. `defer reg.Verify(t)` enforces that every registered stub was actually hit. Acceptance tests take a separate path (`//go:build acceptance` + `testscript`) and run against the real API.

`hams` is stricter on DI discipline, OpenSpec-driven development, and per-provider Docker isolation; `gh-cli` is clearly ahead on command extensibility, engineering simplicity of the factory pattern, the GoReleaser-based release chain, and docs-site productization. They represent two reasonable engineering paradigms at different scales.

**See Section 11 for the item-by-item comparison.**

---

## 1. Directory Layout

```text
gh-cli/
├── cmd/gh/main.go            # 10-line entry, delegates to internal/ghcmd.Main()
├── internal/
│   ├── ghcmd/                # Lifecycle (startup, config, update-check, exit codes)
│   ├── config/               # 24K-LoC YAML config + token storage
│   ├── build/                # Version info (ldflags injection)
│   ├── agents/               # CI environment detection
│   └── tableprinter/         # Table output
├── pkg/
│   ├── cmd/                  # All subcommand implementations (673 files)
│   │   ├── root/             # Root command + command groups
│   │   ├── factory/          # Default Factory constructor
│   │   ├── cache/, pr/, repo/, issue/, …   # One dir per top-level command
│   │   └── …
│   ├── cmdutil/              # Factory struct, error types, flag helpers
│   ├── iostreams/            # TTY / color / pager abstraction
│   ├── httpmock/             # HTTP test stub framework (core testing infra)
│   └── extensions/           # `gh extension` subsystem
├── api/                      # GitHub REST/GraphQL client (24 files, 9.7K LoC)
├── git/, context/            # Git operation wrappers
├── acceptance/               # Acceptance tests (build tag "acceptance")
├── script/                   # Cross-platform build scripts written in Go
├── docs/                     # User docs + release-process docs
├── .goreleaser.yml           # 109-line GoReleaser v2 config
├── Makefile                  # 117 lines
├── AGENTS.md                 # AI/dev onboarding (5.3KB)
└── .github/workflows/        # 14 workflows
```

| Notable | Location | Takeaway |
|---|---|---|
| Thin `main` | `cmd/gh/main.go` | **10 lines**, only maps exit codes |
| Internal core | `internal/ghcmd/cmd.go` | Full lifecycle lives here |
| Command extension root | `pkg/cmd/<cmd>/<subcmd>/` | Each command is an independent package — easy parallel development |
| Testing foundation | `pkg/httpmock/` | Centralized HTTP mocking |

---

## 2. Architecture & Dependency Injection

### 2.1 Bootstrap Chain (3-level, minimal)

```go
// cmd/gh/main.go (all 10 lines)
func main() {
    code := ghcmd.Main()
    os.Exit(int(code))
}

// internal/ghcmd/cmd.go — conceptual outline
func Main() exitCode {
    buildVersion := build.Version            // ldflags-injected
    f := factory.New(buildVersion, invokingAgent)  // DI construction point
    rootCmd, err := root.NewCmdRoot(f, …)
    …
    if err := rootCmd.ExecuteContext(ctx); err != nil { return mapErr(err) }
    return exitOK
}
```

Exit codes are semantically distinguished — `exitOK=0`, `exitError=1`, `exitCancel=2`, `exitAuth=4`, `exitPending=8` — so shell scripts can branch on them.

### 2.2 Factory: Hand-written Closure-based DI (**the most important pattern in this project**)

```go
// pkg/cmdutil/factory.go
type Factory struct {
    AppVersion     string
    ExecutableName string
    InvokingAgent  string

    // Eagerly constructed dependencies
    Browser          browser.Browser
    ExtensionManager extensions.ExtensionManager
    GitClient        *git.Client
    IOStreams        *iostreams.IOStreams
    Prompter         prompter.Prompter

    // Lazy closures — evaluated only when called
    BaseRepo        func() (ghrepo.Interface, error)
    Branch          func() (string, error)
    Config          func() (gh.Config, error)
    HttpClient      func() (*http.Client, error)
    PlainHttpClient func() (*http.Client, error)
    Remotes         func() (context.Remotes, error)
}
```

Wiring order is explicit and linear, so the dependency chain is obvious at a glance:

```go
// pkg/cmd/factory/default.go — linear wiring
func New(appVersion, invokingAgent string) *cmdutil.Factory {
    f := &cmdutil.Factory{AppVersion: appVersion, InvokingAgent: invokingAgent}
    f.Config          = configFunc()                         // no deps
    f.IOStreams       = ioStreams(f)                         // deps: Config
    f.HttpClient      = httpClientFunc(f, appVersion, …)     // deps: Config, IO
    f.PlainHttpClient = plainHttpClientFunc(f, …)
    f.GitClient       = newGitClient(f)
    f.Remotes         = remotesFunc(f)
    f.BaseRepo        = BaseRepoFunc(f)
    f.Prompter        = newPrompter(f)
    f.Browser         = newBrowser(f)
    f.ExtensionManager = extensionManager(f)
    f.Branch          = branchFunc(f)
    return f
}
```

**Why it works**: no DI framework, but framework-level substitutability. Tests replace `f.HttpClient = func() (*http.Client, error) { return fakeHTTP, nil }` and they're done — zero startup cost.

### 2.3 Three-part Command Template (reused by every subcommand)

```go
// pkg/cmd/cache/list/list.go — simplified
type ListOptions struct {
    IO         *iostreams.IOStreams
    HttpClient func() (*http.Client, error)
    BaseRepo   func() (ghrepo.Interface, error)
    Limit      int
    Exporter   cmdutil.Exporter
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
    opts := ListOptions{IO: f.IOStreams, HttpClient: f.HttpClient}
    cmd := &cobra.Command{
        Use: "list",
        RunE: func(cmd *cobra.Command, args []string) error {
            opts.BaseRepo = f.BaseRepo
            if runF != nil { return runF(&opts) }  // ← test injection point
            return listRun(&opts)
        },
    }
    cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "…")
    cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.CacheFields)
    return cmd
}

func listRun(opts *ListOptions) error { … }
```

Three parts: `Options struct` + `NewCmdX(f, runF)` + `xxxRun(opts)`. The seemingly redundant `runF` hook is **the key to testability**: a unit test can bypass Cobra entirely and assert the parsed `ListOptions`.

### 2.4 API Client

```go
// api/client.go — minimal skeleton
type Client struct { http *http.Client }

func NewClientFromHTTP(h *http.Client) *Client { return &Client{http: h} }

func (c Client) GraphQL(host, q string, vars map[string]any, out any) error {
    opts := clientOptions(host, c.http.Transport)
    gqlClient, err := ghAPI.NewGraphQLClient(opts)       // wraps cli/go-gh/v2
    if err != nil { return err }
    return handleResponse(gqlClient.Do(q, vars, out))
}
```

GraphQL queries are centralized in `api/queries_pr.go`, `queries_repo.go`, etc., for high reuse.

### 2.5 I/O Abstraction

```go
// pkg/iostreams/iostreams.go
type IOStreams struct {
    term              term
    In                fileReader
    Out, ErrOut       fileWriter
    stdinTTYOverride  bool   // test override
    stdoutTTYOverride bool
    colorEnabled      bool
    pagerCommand      string
    progressIndicator *spinner.Spinner
}

// test helper
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) { … }
```

**Every** terminal output goes through `IOStreams`; tests just replace the streams with `bytes.Buffer` and assert on the captured output.

---

## 3. Linting & Code Quality

### 3.1 `.golangci.yml` (gh-cli)

```yaml
# 26 linters enabled, core categories below
linters:
  enable:
    - bodyclose
    - gocritic        # disabled checks: appendAssign, style tags
    - nilerr
    - gocheckcompilerdirectives
    - gomoddirectives
    - nolintlint      # every //nolint must name a linter AND explain why
    # Kept as "disabled" (not removed) for future activation:
    #   - gosec, staticcheck, errcheck
formatters:
  - gofmt             # mounted as a formatter, not a linter
exclusions:
  - third-party/
```

**Highlight**: `nolintlint` forces every `//nolint:xxx` comment to include a justification, preventing lint-suppression black holes.

### 3.2 Supplementary tools

- `govulncheck` in its own workflow, not as a golangci plugin.
- `go mod tidy` is **run and then diffed** in CI — if it produces any changes, CI fails. This forces every PR to submit a tidied `go.sum`.
- `make licenses-check` regenerates LICENSE files across all GOOS/GOARCH combinations to enforce distribution compliance.

---

## 4. Unit Testing

### 4.1 Layout

`_test.go` files **always sit next to their source files**. There is no top-level `tests/` directory.

### 4.2 HTTP Mock (core infra)

```go
// pkg/httpmock/stub.go — minimal Registry pattern
type Matcher   func(req *http.Request) bool
type Responder func(req *http.Request) (*http.Response, error)

type Stub struct {
    matched   bool
    Matcher   Matcher
    Responder Responder
}

type Registry struct { stubs []*Stub }

func (r *Registry) Register(m Matcher, resp Responder) { … }
func (r *Registry) Verify(t *testing.T)                { /* every stub must be hit */ }

// pre-built matchers
func REST(method, path string) Matcher { … }
func GraphQL(query string) Matcher     { … }
```

### 4.3 A typical test (table-driven + httpmock)

```go
// pkg/cmd/cache/delete/delete_test.go — simplified
func TestDeleteRun(t *testing.T) {
    tests := []struct {
        name     string
        cli      string
        wants    DeleteOptions
        wantsErr string
    }{
        {name: "no arguments", cli: "", wantsErr: "must provide cache id, key, or --all"},
        {name: "by id",        cli: "123"},
        …
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            reg := &httpmock.Registry{}
            defer reg.Verify(t)
            reg.Register(httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/123"),
                         httpmock.StatusStringResponse(204, ""))

            ios, _, stdout, _ := iostreams.Test()
            ios.SetStdoutTTY(true)
            …
            require.NoError(t, err)
            assert.Contains(t, stdout.String(), "Deleted 1 cache")
        })
    }
}
```

Conventions:

- `testify/require` for must-fast-fail assertions (mostly `err`);
- `testify/assert` for non-fatal assertions;
- `bytes.Buffer` as I/O stand-ins;
- `go:generate moq -rm -out prompter_mock.go . Prompter` — interface mocks generated via [moq](https://github.com/matryer/moq).

---

## 5. Acceptance Testing

```text
acceptance/
├── acceptance_test.go       # entry, build tag=acceptance
└── testdata/                # .txtar scripts
    ├── pr_create.txtar
    └── …
```

```go
// build tag: only visible with `go test -tags=acceptance ./acceptance`
//go:build acceptance

func TestScript(t *testing.T) {
    testscript.Run(t, testscript.Params{
        Dir:   "testdata",
        Setup: func(env *testscript.Env) error { … },   // inject GH_ACCEPTANCE_*
    })
}
```

Requires env vars `GH_ACCEPTANCE_HOST` / `GH_ACCEPTANCE_ORG` / `GH_ACCEPTANCE_TOKEN`, and talks to the **real GitHub API**; default `go test ./...` skips this.

---

## 6. CI Pipeline

### 6.1 14 workflows (the three core ones)

| Workflow | Trigger | Matrix | Key steps |
|---|---|---|---|
| `go.yml` | push to trunk / PR | Linux + macOS + Windows | `go test -race -tags=integration ./...` + `go build` |
| `lint.yml` | `**.go` changes | Linux | `go mod tidy` diff, `golangci-lint v2.11.0`, `make licenses-check`, `govulncheck ./...` |
| `deployment.yml` | manual `workflow_dispatch` | Linux + macOS + Windows (parallel) | Validate `tag_name` regex → `script/release --local $TAG --platform <os>` → sign → upload artifact |

```yaml
# deployment.yml excerpt
on:
  workflow_dispatch:
    inputs:
      tag_name:    { description: 'v1.2.3', required: true }
      environment: { type: environment,     required: true }
jobs:
  validate-tag-name:
    steps:
      - run: echo "${{ inputs.tag_name }}" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$'
  linux:
    needs: validate-tag-name
    steps:
      - run: script/release --local "$TAG_NAME" --platform linux
  macos:
    steps:
      - if: inputs.environment == 'production'
        run: /usr/bin/codesign … # Apple signing
```

The other 11 workflows: `codeql` / `bump-go` / `govulncheck` / `homebrew-bump` / `detect-spam` / `triage-*` — a lot of non-code responsibility has been baked into Actions.

---

## 7. Release Process

### 7.1 GoReleaser v2 (109-line `.goreleaser.yml`)

```yaml
# .goreleaser.yml — structural outline
version: 2

before:
  hooks:
    - cmd: make manpages     # non-Windows only
      env: [ 'GOOS=linux' ]
    - cmd: make completions
    - cmd: go generate script/build/windows-syso/generate.go  # Windows .syso

builds:
  - id: macos
    goos: [darwin]
    goarch: [amd64, arm64]
    hooks:
      post: gon -macos-signature={{.Path}}   # Apple signing
    ldflags:
      - -X github.com/cli/cli/v2/internal/build.Version={{.Version}}
      - -X github.com/cli/cli/v2/internal/build.Date={{time "2006-01-02"}}
  - id: linux
    env: [ CGO_ENABLED=0 ]
    goos: [linux]
    goarch: [386, amd64, arm, arm64]
  - id: windows
    hooks:
      post: powershell.exe sign …

archives:
  - id: linux,  format: tar.gz, files: [man/*]
  - id: macos,  format: zip,    files: [man/*]
  - id: windows, format: zip

nfpms:   # DEB + RPM
  - formats: [deb, rpm]
    contents:
      - { src: ./share/man/*,         dst: /usr/share/man/… }
      - { src: ./share/completions/*, dst: … }
```

### 7.2 Version Injection

```go
// internal/build/build.go — fallback
var (
    Version = "DEV"
    Date    = ""
)

func init() {
    if Version == "DEV" {
        if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
            Version = info.Main.Version   // this path wins for `go install` users
        }
    }
}
```

Two injection paths: GoReleaser ldflags (official release artifacts) + `runtime/debug.BuildInfo` (users installing via `go install`).

---

## 8. Build System

### 8.1 Makefile (117 lines)

```makefile
GOOS   ?= $(shell go env GOOS)
GO_LDFLAGS := -X github.com/cli/cli/v2/internal/build.Version=$(shell …)

bin/gh: script/build
    @GOOS= GOARCH= go run ./script/build $@

script/build: script/build.go
    GOOS= GOARCH= GOARM= GOFLAGS= CGO_ENABLED= go build -o $@ $<

manpages:    bin/gh ; @./script/generate-manpages
completions: bin/gh ; @./script/generate-completions
licenses:    ; go run ./script/gen-licenses $(GOOS) $(GOARCH) LICENSE.md
install:     manpages completions ; cp -r bin/gh $(DESTDIR)$(bindir)
```

**Notable**: build logic is implemented in Go (`script/build.go`) rather than bash — portable across platforms and testable.

### 8.2 go.mod

- `go 1.26.1` / `toolchain go1.26.2` (very recent)
- Key deps: `cli/go-gh/v2` (GitHub API wrapper), `spf13/cobra`, `charm.land/*` (TUI), `sigstore/sigstore-go` (signing), `zalando/go-keyring`

---

## 9. Docs & Contribution

```text
docs/
├── install_{linux,macos,windows,source}.md
├── multiple-accounts.md
├── codespaces.md
├── triage.md
├── project-layout.md
├── releasing.md
├── release-process-deep-dive.md
├── command-line-syntax.md
├── gh-vs-hub.md
└── license-compliance.md

AGENTS.md      # 5.3KB, architecture tour for AIs + new developers
```

`AGENTS.md` is remarkably compact: entry point, Factory wiring, command three-part template, HTTP mock, JSON output template — all covered in one page. LLM-friendly by design.

---

## 10. Reusable Engineering Patterns

| Pattern | Location | Best fit |
|---|---|---|
| Lightweight Factory DI | `pkg/cmdutil/factory.go` | Mid-size CLIs that reject DI frameworks but need test substitutability |
| `NewCmdX(f, runF)` three-part template | `pkg/cmd/*/` | Cobra subcommand standardization |
| `httpmock.Registry + Verify` | `pkg/httpmock/` | REST/GraphQL client unit tests |
| `//go:build acceptance` + testscript | `acceptance/` | Real-environment integration acceptance |
| GoReleaser multi-platform + multi-sign post-hook | `.goreleaser.yml` | 11 artifacts from one YAML |
| `nolintlint` enforced explanations | `.golangci.yml` | Suppressing lint-suppression black holes |
| Build scripts in Go | `script/build.go` | Cross-platform builds without bash |
| `runtime/debug.BuildInfo` version fallback | `internal/build/build.go` | Correct version for `go install` users |

---

## 11. Comparison: `gh-cli` vs `hams`

### 11.1 Numbers & paradigms

| Dimension | gh-cli | hams |
|---|---|---|
| Code size | ~806 Go files | ~80 Go files (internal/cli + provider ≈ 5.5K LoC combined) |
| CLI framework | Cobra | urfave/cli v3 |
| DI approach | Hand-written Factory closures (11 fields) | Constructor parameter injection (`NewApp(registry, sudoAcq)`); Uber Fx is in go.mod but used only by `internal/version/module.go` |
| Lint | 26 linters + `nolintlint` explanation requirement | **34 linters** (more aggressive) including `goconst/errname/modernize/intrange/tparallel` |
| Unit tests | `pkg/httpmock.Registry` + testify | Interface injection + rapid **property-based testing** (10+ files) |
| Acceptance/E2E | `acceptance/` + testscript against real API | **Docker per-provider matrix** (11 providers × independent images), plus `act` for local GitHub Actions replay |
| CI workflow count | 14 | 3 (`ci.yml` / `docs.yml` / `release.yml`) |
| CI execution model | Each step has raw commands inline | **Every step must invoke `task <name>`** (explicit rule, guarantees local/CI isomorphism) |
| Release tool | GoReleaser v2 (signing + DEB/RPM) | Hand-written `scripts/build-all.sh` + `softprops/action-gh-release` |
| Docs site | None (docs/ is in-repo markdown only) | **Nextra site** under `docs/`, deployed to `hams.zthxxx.me` |
| Spec/requirements mgmt | None | **OpenSpec-driven** (`openspec/{specs,changes,archive}`) |
| Release artifacts | darwin/linux/windows × multi-arch + DEB/RPM + signing + manpages | darwin/arm64 + linux/{amd64,arm64}, plain binaries only |
| Version fallback | `runtime/debug.BuildInfo` | ldflags only |

### 11.2 Things `gh-cli` does better (worth borrowing into `hams`)

1. **Lightweight Factory DI model** — hams is currently split: it has Uber Fx in `go.mod` (used only by the version package) and otherwise relies on explicit constructor parameters. `gh-cli` solves this with a ~50-line `Factory` struct that offers **zero startup overhead + trivial per-field test substitution**. hams could consolidate `Registry / SudoAcquirer / IOStreams / HTTP` etc. into a similar `hamsutil.Factory` rather than continuing to grow the constructor parameter list.

   ```go
   // Suggested refactor direction for hams (pseudocode)
   type Factory struct {
       IOStreams  *iostreams.IOStreams
       Config     func() (*config.Config, error)
       Registry   *provider.Registry
       SudoAcq    sudo.Acquirer
       HTTPClient func() (*http.Client, error)
       StateStore func() (state.Store, error)   // replaces loadOrCreateStateFile with DI
   }
   func NewApp(f *Factory) *cli.Command { … }
   ```

2. **Three-part Command + `runF` injection** — hams' `apply.go` / `refreshCmd` don't expose a `runF` hook, so tests must spin up the full urfave/cli stack. Borrowing `NewCmdX(f, runF func(*Opts) error)`:

   ```go
   // Current (hams internal/cli/apply.go, conceptual pseudocode)
   func applyCmd(registry *provider.Registry, sudoAcq sudo.Acquirer) *cli.Command {
       return &cli.Command{Action: func(ctx, cmd) error {
           opts := parseFlags(cmd)
           return runApply(ctx, opts, registry, sudoAcq)
       }}
   }
   // Suggested refactor
   func applyCmd(f *Factory, runF func(*ApplyOpts) error) *cli.Command {
       opts := &ApplyOpts{IO: f.IOStreams, Registry: f.Registry}
       return &cli.Command{Action: func(ctx, cmd) error {
           opts.FillFromFlags(cmd)
           if runF != nil { return runF(opts) }   // unit tests bypass the CLI layer
           return runApply(ctx, opts)
       }}
   }
   ```

3. **GoReleaser-based release chain** — hams' current `release.yml` hand-writes ldflags and `find | sha256sum`, and is missing:
   - Apple signing (macOS downloads will hit Gatekeeper)
   - DEB/RPM packages (inverse of `task test:itest:one PROVIDER=apt` — users should be able to `apt install hams`)
   - Automated Homebrew tap updates (gh-cli has `homebrew-bump.yml`)
   - Manpage distribution

4. **`nolintlint` enforced explanations** — hams' `.golangci.yml` does not enable `nolintlint`; `//nolint` comments without reasons will accumulate. Recommended addition:

   ```yaml
   # .golangci.yml
   linters:
     enable:
       - nolintlint
   linters-settings:
     nolintlint:
       require-explanation: true
       require-specific: true
   ```

5. **`go mod tidy` as a CI gate** — gh-cli runs `go mod tidy` in `lint.yml` then `git diff --exit-code`, failing any PR that submits a dirty `go.sum`. hams could add `task ci:tidy:check`.

6. **`runtime/debug.BuildInfo` version fallback** — users running `go install github.com/zthxxx/hams/cmd/hams@latest` currently see `DEV`. gh-cli's trick:

   ```go
   // Recommended addition to internal/version/module.go
   func init() {
       if version == "" || version == "DEV" {
           if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
               version = info.Main.Version
           }
       }
   }
   ```

7. **AGENTS.md as an LLM-friendly entry point** — gh-cli's `AGENTS.md` is **one file** covering architecture, commands, tests, JSON output patterns. hams' current `AGENTS.md` exists but mostly points to `CLAUDE.md`; the command-pattern and test-pattern sections with directly reusable code snippets are missing.

### 11.3 Things `hams` does better (worth borrowing into `gh-cli`)

1. **OpenSpec-driven development** — `openspec/{specs,changes,archive}` gives every feature a traceable "why" and "how it evolved" chain. gh-cli's requirement traceability relies on GitHub Issues/PRs + `docs/release-process-deep-dive.md`, which is much weaker than OpenSpec's historical record.

2. **Taskfile as the single source of truth for CI** — hams' rule "every GitHub Actions step must invoke `task <name>`, no raw commands" (from `.claude/rules/development-process.md`) enforces **local = CI isomorphism**, dramatically reducing "CI green, local red" drift. gh-cli inlines a lot of `go test` / `script/release` commands in workflows, forcing developers to mentally re-derive CI behavior.

3. **Docker per-provider integration matrix** — each hams provider has its own Docker image (`internal/provider/builtin/<provider>/integration/Dockerfile`), SHA-keyed caching, `fail-fast: false`. gh-cli's `acceptance/` is a single testscript suite against the real GitHub API; it has **no environment isolation for different auth modes or different API behavior**.

4. **Property-based testing** — hams makes heavy use of `pgregory.net/rapid` (`internal/provider/property_test.go` and 10+ other files), expressing **invariants** rather than example cases. gh-cli is 99% table-driven and rarely validates invariants.

5. **i18n + documentation site** — hams' `docs/` (Nextra) + `README.zh-CN.md` + `docs/**/*.zh-CN.*` bilingual norms, deployed to an independent domain. gh-cli has only English markdown in `docs/` with no independent site (the real cli.github.com lives in a separate repo).

6. **More aggressive linter set** — hams enables `modernize` / `intrange` / `tparallel` / `wastedassign` / `nakedret` / `nestif` and more — linters gh-cli doesn't. 34 vs 26. Modernization expectations on new code are higher.

7. **Universal secret decoupling rule** — hams explicitly requires all tokens to live in OS keychain or `*.local.*` files (`.claude/rules/development-process.md`). gh-cli's auth tokens do use keyring in practice, but the rule isn't formalized.

8. **Lefthook git hooks** — hams' `lefthook.yml` hangs fmt/lint/test on pre-commit/pre-push. gh-cli has **no git hooks** and relies entirely on CI as the safety net.

### 11.4 Overall Evaluation

- **gh-cli**: years of accumulated discipline, huge codebase, **outstanding engineering simplicity and command extensibility**. The Factory-closure pattern is a best practice any similar CLI can copy-paste.
- **hams**: young but **methodologically strict** (OpenSpec / DI / Taskfile isomorphism / property tests). It goes further than gh-cli on "correctness culture" and "re-entrant development process", making it a good model for a modern Go CLI.

Recommended next steps for hams: port gh-cli's **Factory + three-part command template** into `internal/cli/`; replace `scripts/build-all.sh` with GoReleaser; enable `nolintlint` in `.golangci.yml`. Those three changes together would put hams at gh-cli's engineering-maturity level.

---

## Appendix: Key `file:line` References

| Description | Path |
|---|---|
| gh-cli entry | `/workspaces/gh-cli/cmd/gh/main.go:1-12` |
| Factory definition | `/workspaces/gh-cli/pkg/cmdutil/factory.go:19-39` |
| Factory wiring | `/workspaces/gh-cli/pkg/cmd/factory/default.go:29-49` |
| Command three-part example | `/workspaces/gh-cli/pkg/cmd/cache/list/list.go:34-88` |
| API Client | `/workspaces/gh-cli/api/client.go:29-80` |
| IOStreams | `/workspaces/gh-cli/pkg/iostreams/iostreams.go:49-87` |
| httpmock | `/workspaces/gh-cli/pkg/httpmock/stub.go:16-62` |
| `.golangci.yml` | `/workspaces/gh-cli/.golangci.yml:1-72` |
| Deployment workflow | `/workspaces/gh-cli/.github/workflows/deployment.yml:1-100` |
| GoReleaser | `/workspaces/gh-cli/.goreleaser.yml:1-109` |
| hams entry | `/workspaces/hams/cmd/hams/main.go:1-10` |
| hams NewApp | `/workspaces/hams/internal/cli/root.go:44-90` |
| hams `release.yml` | `/workspaces/hams/.github/workflows/release.yml` |
| hams `ci.yml` | `/workspaces/hams/.github/workflows/ci.yml` |
