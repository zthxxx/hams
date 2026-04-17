# Task: Extend Provider Test Coverage to apt Parity

## Finding

Test-design audit surfaced a sharp inequality across the 15 builtin providers:

| Provider    | Test functions | Apply/Probe tests | DI boundary | Property-based tests |
|-------------|----------------|-------------------|-------------|----------------------|
| apt         | 38 (U1–U37)    | ✓                 | `FakeCmdRunner` | Partial             |
| bash        | ~8             | ✓                 | `bootstrapExecCommand` | —            |
| homebrew    | ~10            | Bootstrap only    | `brewBinaryLookup` + `envPathAugment` | — |
| pnpm        | ~6             | Bootstrap only    | `pnpmBinaryLookup` | —                |
| ansible     | ~5             | Bootstrap only    | — (needs audit) | —                   |
| mas         | ~4             | Bootstrap only    | `*BinaryLookup` | —                   |
| duti        | ~4             | Bootstrap only    | `*BinaryLookup` | —                   |
| cargo       | 2              | ✗                 | ✗ none visible | ✗                    |
| npm         | 2              | ✗                 | ✗ none visible | ✗                    |
| uv          | 2              | ✗                 | ✗ none visible | ✗                    |
| goinstall   | 2              | ✗                 | ✗ none visible | ✗                    |
| vscodeext   | 2              | ✗                 | ✗ none visible | ✗                    |
| defaults    | ~3             | ✗                 | ✗ none visible | ✗                    |
| git-config  | 1              | ✗                 | ✗ (direct git calls) | ✗              |
| git-clone   | 1              | ✗                 | ✗ (direct git calls) | ✗              |

The 4 providers with 2 tests have exactly `TestManifest` and `TestParseXxxList` — Manifest is trivial, parser is example-based only.

## Risks From This Gap

1. **Silent host mutation** — a contributor runs `go test ./internal/provider/builtin/cargo/...` and `apply` tests (if added without DI) would run real `cargo install`. The current workaround is "don't add tests", which is the problem.
2. **Parser fragility** — `ParseNpmList`, `ParseCargoList`, `ParsePnpmList`, `ParseExtensionList`, `ParseUvToolList`, `ParseMasList` all consume unstructured output from external CLIs. A upstream JSON format change would crash silently. Three hand-picked test cases per parser is not coverage.
3. **Lifecycle regressions** — `apt`'s U22-U37 tests caught version-pin regressions during the apt-cli-complex-invocations cycle. The other package-like providers have no equivalent tripwire; pins being silently dropped wouldn't be caught.

## Architect Decision: Prioritize by User Risk

Tier 1 (highest risk; real packages users install daily):

- [ ] `cargo` — copy `apt_test.go` U-pattern: U1 install, U2 idempotent, U3 install-failure, U4 remove, U5 dry-run, U12 state-write, U13 first-install-at preservation. Introduce `FakeCmdRunner` in `internal/provider/builtin/cargo/command.go` + `command_fake.go` mirroring apt's structure.
- [ ] `npm` — same pattern.
- [ ] `pnpm` — same pattern.
- [ ] `uv` — same pattern.
- [ ] `goinstall` — same pattern, plus test `@latest` injection.

Tier 2 (macOS-specific or low-frequency):

- [ ] `vscodeext` — same pattern, plus extension version parsing edge cases.
- [ ] `mas` — same pattern, numeric app-ID handling.
- [ ] `defaults` — apply/probe for macOS pref mutations. Must inject an exec boundary (no real `defaults` calls in unit tests).
- [ ] `duti` — apply/probe for extension→bundle-ID mappings. Same DI requirement.

Tier 3 (declarative, low blast radius):

- [ ] `git-config` — apply/probe for `~/.gitconfig` mutation. Must inject a writer that writes to `t.TempDir()`, not the real home dir.
- [ ] `git-clone` — probe for repo-present-or-not. Must inject a filesystem-check seam.

Property-based (cross-cutting):

- [ ] For each `ParseXxxList` parser, add a `TestProperty_ParseXxxListRobustness` using `rapid` that:
  - Generates arbitrary `StringMatching` inputs.
  - Asserts no panics.
  - Asserts that any returned package names are non-empty and don't contain whitespace.
  - Asserts parser is idempotent on the same input.

## Non-Goals

- Do **not** introduce integration tests here — those already exist per-provider under `internal/provider/builtin/*/integration/` and run via `task ci:itest:run PROVIDER=<name>`. This task is strictly about DI-isolated unit-test parity with apt.
- Do **not** rewrite apt's tests — they are the reference implementation to copy from.
