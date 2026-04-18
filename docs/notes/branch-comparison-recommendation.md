# Branch Recommendation: `dev` ← `local/loop` Gap Analysis

Date: 2026-04-18
Companion to: `docs/notes/branch-comparison-dev-vs-local-loop.md`
Purpose: Gap analysis against OpenSpec requirements to drive the next improvement cycle on `dev`. `local/loop` is **reference only — not modified**.

Legend:

- **[dev]** — `dev`'s implementation matches the spec better; keep it.
- **[loop]** — `local/loop`'s implementation matches the spec better; `dev` needs to absorb.
- **[both]** — both branches violate the spec or leave debt; `dev` needs to fix independently.

---

## 1. Auto-init scaffolding

Spec anchor: `openspec/specs/cli-architecture/spec.md` "Onboarding Auto-Init" + CLAUDE.md §Current Tasks §3 ("CLI user-facing messages … must implement i18n").

- **[dev] — package layering + go-git fallback.** `internal/storeinit/` is a dedicated package with embedded templates; falls back to `github.com/go-git/go-git/v5` when the `git` binary is not on PATH (required by CLAUDE.md §Build & Distribution: "Bundles go-git for fresh machines without git"). `local/loop` inlined the scaffolder into `internal/cli/scaffold.go` and dropped go-git — a regression against the stated design.
- **[loop] — UX completeness.** `EnsureStoreScaffolded` short-circuits when `flags.DryRun` is set, wraps `git init` in `context.WithTimeout(ctx, 30*time.Second)`, and seeds `profile_tag` + `machine_id` in the global config. `dev`'s `EnsureStoreReady` misses all three, so `hams --dry-run brew install htop` on a pristine host still runs `git init`, a hung global hook can wedge first-time setup, and every post-scaffold invocation emits a "profile_tag is empty" nudge.
- **Better reference:** `/tmp/hams-loop/internal/cli/scaffold.go:91` (`EnsureStoreScaffolded` lifecycle), `:149` (`seedIfMissing`), `:186` (`scaffoldStoreFiles` with ctx timeout).
- **Action on `dev`:** port dry-run short-circuit, `context.WithTimeout(30s)` on git init, and identity-seeding into `internal/storeinit/storeinit.go` + `internal/cli/autoinit.go`. Keep the package boundary and go-git fallback.

## 2. Unified `hams git` entry point

Spec anchor: CLAUDE.md §Current Tasks: "Provider wrapped commands MUST behave exactly like the original command, at least at the first-level command entry point."

- **[loop] — spec conformance.** `/tmp/hams-loop/internal/provider/builtin/git/unified.go:94` routes `config`/`clone` to the recorded sub-providers and passes every other verb (`hams git pull`, `hams git log`, `hams git status`, …) transparently to the real `git` binary with stdio + exit code preserved. It also translates `hams git clone <remote> <path>` into the CloneProvider's internal DSL so the natural git CLI shape records, and rejects unforwarded git flags (`--depth`, `--branch`) with an actionable UFE.
- **[dev] — i18n discipline.** `internal/provider/builtin/git/unified.go` routes every user-facing string through `i18n.T`/`i18n.Tf`. `local/loop`'s 221-LoC handler ships raw English (`"hams git requires a subcommand"`, `"git clone requires a remote URL"`) even though the same branch committed to the i18n infrastructure.
- **Better reference:** merge is additive — take `local/loop`'s passthrough + DSL translation, replace its hard-coded strings with `dev`'s i18n pattern + typed keys from §4.
- **Action on `dev`:** rewrite `internal/provider/builtin/git/unified.go` to include the passthrough branch, keep `dev`'s existing i18n usage for the known-subcommand path, add new i18n keys for the passthrough errors.

## 3. `--tag` / `--profile` flag handling

Spec anchor: `openspec/specs/cli-architecture/spec.md` + the intent documented in commit `df093ee` (dev) and `4ca1904` (loop): "`--tag` is the canonical name; `--profile` is the legacy alias."

- **[loop] — conflict detection.** `/tmp/hams-loop/internal/config/resolve.go:36` `ResolveCLITagOverride(cliTag, cliProfile)` returns a UFE when both flags are set to different values. `dev` registers `--tag` as an alias on the same underlying field, so `--tag=macOS --profile=linux` silently lets the later flag win. A user migrating a CI script loses no ceremony today but will surface a silent mistake on any machine that still has the old flag hard-coded.
- **[loop] — better factoring.** Dedicated `Tag` field on `provider.GlobalFlags` is separate from `Profile`; resolution is explicit instead of implicit. Additional helper `DeriveMachineID` (same file, `:88`) factors the hostname-or-env logic out of the CLI layer so tests can seam it.
- **Action on `dev`:** add `Tag string` on `GlobalFlags`, lift `/tmp/hams-loop/internal/config/resolve.go` verbatim (adjust i18n keys to match `dev`'s naming), add conflict-detection call-site in `internal/cli/apply.go` and `internal/cli/provider_cmd.go`.

## 4. i18n infrastructure

Spec anchor: `openspec/specs/cli-architecture/spec.md` "Internationalization via locale environment variables": *"The i18n module SHALL provide a message catalog interface that all user-facing strings (errors, help text, prompts) go through."*

- **[loop] — typed keys catalog.** `/tmp/hams-loop/internal/i18n/keys.go` defines each message ID as a `const` with a doc comment naming the call-site. `i18n.T(i18n.CLIErrTagProfileConflict)` compiles; a typo `i18n.T(i18n.CLIErrTagProfileConflic)` does not. `dev` relies on string literals — any typo silently returns the key ID as its own translation.
- **[dev] — coverage inside git dispatcher.** `dev` routes every string in `unified.go` through `i18n.T` (7 call-sites); `local/loop` routes zero there (shipped raw English). See §2.
- **[both] — call-site coverage is ~15%.** `rg -n 'i18n\.T[f]?\(' internal/` on `dev` returns ~13 call-sites; on `local/loop` ~15. The codebase has ~50 `hamserr.NewUserError` call-sites and ~100+ `fmt.Print*` on user-facing stdout/stderr. Spec requires ALL user-facing strings; neither branch is close.
- **Action on `dev`:** (a) port `/tmp/hams-loop/internal/i18n/keys.go` (rename keys as needed for naming coherence); (b) grep every `hamserr.NewUserError(` / `fmt.Printf`/`Println`/`Fprintln` with a user-visible message and replace the literal with `i18n.T(…)` / `i18n.Tf(…)`; (c) add matching keys to both `en.yaml` and `zh-CN.yaml`.

## 5. Provider name: `code-ext` → `code`

Spec anchor: CLAUDE.md §Current Tasks §3: "The `code-ext` provider likewise should expose only the `hams code` entry point."

- **[loop] — completed the rename.** `Manifest().Name` is `code`; `FilePrefix` is `code`; hamsfile on disk is `code.hams.yaml`; provider registry key is `code`.
- **[dev] — half-rename.** `hams code install …` works, but `Manifest().Name` is still `code-ext`, `FilePrefix` is still `vscodeext`, hamsfile on disk is `vscodeext.hams.yaml`, and `apply --only=code-ext` is the registry-side invocation. The fix `f6c063d` papers over the integration test divergence via a `MANIFEST_NAME` override — symptom treatment rather than cure.
- **[user-noted, important]** hams has not formally released, so there is no shipped `vscodeext.hams.yaml` to migrate for end users. Both internal and external names SHOULD be `code`. The `MANIFEST_NAME=code-ext` override is the tell — it means the internal name and the CLI verb drifted, which is exactly what CLAUDE.md's unify-everything principle forbids.
- **Action on `dev`:** rename `Manifest.Name` + `FilePrefix` to `code` in `internal/provider/builtin/vscodeext/vscodeext.go`; delete the `MANIFEST_NAME=code-ext` override in `internal/provider/builtin/vscodeext/integration/integration.sh`; grep `vscodeext.hams.yaml` and `code-ext` across docs and scripts; update fixture file names.

## 6. CI / Taskfile ergonomics

Spec anchor: CLAUDE.md §Development Process: "Local/CI isomorphism: code style checks (golangci-lint), unit tests, and Docker-based E2E tests MUST all run identically on a developer's local machine and in GitHub Actions CI."

- **[loop] — act is opt-in.** `.github/workflows/ci.yml` guards `actions/upload-artifact@v4` with `if: ${{ !env.ACT }}`; `Taskfile.yml` routes `test:e2e` / `test:integration` / `test:itest` through `ci:*` tasks directly via docker; `:one-via-act` variants are the explicit opt-in. `dev`'s `task test:e2e` still fails on act's artifact-server ECONNRESET bug. This blocks reliable local CI reproduction.
- **[dev]** — no CI changes. Still depends on act for every local integration/e2e path.
- **Action on `dev`:** cherry-pick the CI + Taskfile + `.golangci.yml` deltas from `local/loop` verbatim; they are orthogonal to the feature work.
- **Reference:** `/tmp/hams-loop/.github/workflows/ci.yml` + `/tmp/hams-loop/Taskfile.yml` (compare against current `dev` counterparts).

## 7. Integration-test log assertions

Spec anchor: CLAUDE.md §Current Tasks: "Whether logging is emitted — for each provider as well as for hams itself — must be verified in integration tests."

- **[dev] — stronger assertion.** `e2e/base/lib/assertions.sh` adds `assert_log_contains` + `assert_log_records_session` which read the rolling log file — so the assertion transitively validates slog→file handoff, not just stderr capture.
- **[loop] — broader fan-out.** Assertions wired into bash / ansible / git integration scripts (3 providers) instead of just apt (1 provider).
- **[both] — still missing 7 providers.** Neither branch covers cargo, goinstall, homebrew, npm, pnpm, uv, vscodeext. The spec says *every* provider.
- **Action on `dev`:** keep file-based helper (it is stricter), add `assert_stderr_contains` from `local/loop` as a faster sibling, fan out to every provider's `integration.sh`.

## 8. Shared provider abstraction

Spec anchor: CLAUDE.md §Current Tasks §3 last bullet: "All providers follow the same pattern: parse the original command structure, extract what needs to be recorded, then pass the remainder through to the underlying command for execution. … design shared abstractions — either a single generic base or a few categorical base types — so that extending with a new provider is a matter of filling in a well-defined template, not reimplementing the pattern from scratch."

- **[dev] — narrow, universal helper.** `internal/provider/baseprovider/baseprovider.go` covers hamsfile path + effective config — applicable to every provider shape.
- **[loop] — broad, package-only helper.** `internal/provider/package_dispatcher.go` captures the install/remove/lock/record flow for package-like providers (apt/brew/cargo/goinstall/npm/pnpm/uv/mas/vscodeext).
- **[both] — zero existing providers migrated.** Both helpers are *opt-in scaffolding only*. Neither branch deleted the duplicated `hamsfile.go` in any existing provider. The CLAUDE.md task remains open on both.
- **[both] — passthrough for unhandled subcommands is not formalized.** The git dispatcher gets per-provider passthrough in `local/loop` but the concept is not lifted into the shared abstraction. `hams brew upgrade`, `hams apt list`, `hams pnpm outdated` all either error or don't exist — violating the "wrapped commands MUST behave exactly like the original" rule at the first-level entry point.
- **Action on `dev`:**
  - Keep `baseprovider` as the narrow, universal helper. Port `package_dispatcher` alongside it (not as a replacement).
  - Actually migrate every package-like provider: replace each `hamsfile.go` with calls into `baseprovider.LoadOrCreateHamsfile`/`HamsfilePath`/`EffectiveConfig`; replace each install/remove handler with a call into `AutoRecordInstall`/`AutoRecordRemove` from `package_dispatcher`.
  - Add a shared `Passthrough` helper on the `provider` package that a CLI handler can call for unhandled subcommands — mirrors the git passthrough pattern but in the shared layer so every provider inherits it.
  - Delete the duplicated boilerplate.

## 9. Other gaps both branches share

| Gap | Spec anchor | Remediation on `dev` |
|---|---|---|
| `provider_flow.sh` assumes CLI verb == manifest name | `openspec/specs/provider-system/spec.md` | Make `MANIFEST_NAME` match `PROVIDER_NAME` everywhere by finishing the rename (§5). |
| Docs still reference `code-ext` / `git-config` / `git-clone` in various `*.mdx` | `openspec/specs/builtin-providers/spec.md` | Full grep + rewrite across `docs/content/{en,zh-CN}/**`. |
| `openspec/specs/**` not updated to reflect `hams git` passthrough contract | `openspec/specs/provider-system/spec.md` | Add a "Passthrough for Unhandled Subcommands" requirement. |
| Log assertions not wired into `hams apply` itself (only per-provider) | CLAUDE.md §Current Tasks | Add a framework-level assertion in `e2e/base/lib/assertions.sh` that `hams apply` emits `session-started` + final-status logs. |
| `config.ResolveCLITagOverride` not exposed as a sanitizer for the legacy `--profile` CI flow | `openspec/specs/schema-design/spec.md` | Implement §3, emit a deprecation slog line when only `--profile` is supplied. |

---

## 10. Remediation plan (to be executed on `dev`)

1. **storeinit completeness** — dry-run, ctx timeout, identity seeding. Keep `internal/storeinit` package + go-git fallback.
2. **git passthrough** — rewrite `unified.go` with passthrough branch + translation + flag rejection, all through i18n.
3. **`--tag` conflict detection** — separate `Tag` on `GlobalFlags`, `config.ResolveCLITagOverride`, wired into apply + provider_cmd.
4. **Typed i18n keys** — `internal/i18n/keys.go`, refactor call-sites.
5. **`code-ext` → `code` full rename** — Manifest.Name + FilePrefix + drop `MANIFEST_NAME` override + doc grep.
6. **CI + Taskfile** — act → opt-in, artifact guards, `ci:*`-direct defaults.
7. **Integration log assertion fan-out** — all 11 providers + framework-level assertion.
8. **Shared abstraction migration** — every package-like provider uses `baseprovider` + `package_dispatcher`; delete duplicated code.
9. **Provider-level passthrough** — shared helper + adoption in every CLI-wrapping provider.
10. **i18n fan-out** — wrap every `NewUserError` + user-facing `fmt.Print*`.
11. **Docs sync** — delete all `code-ext` / `git-config` / `git-clone` references; add `--tag` / passthrough docs.

Each item lands as its own OpenSpec change in `openspec/changes/2026-04-18-*` so the work is reviewable and the delta specs update the main specs on archive.

Last verified 2026-04-18 against `dev@f6c063d` (current) and `origin/local/loop@66266ff` (reference).
