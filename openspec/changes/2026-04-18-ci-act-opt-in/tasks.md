# Tasks: CI act → opt-in

- [x] **ci.yml — artifact guards + act fallbacks**
  - [x] Guard both `actions/upload-artifact@v4` steps (coverage, binary) with `if: ${{ !env.ACT }}` + inline comment explaining ECONNRESET.
  - [x] In the `integration`, `e2e`, `itest` jobs, mark `Download linux/amd64 binary` as `if: ${{ !env.ACT }}`, add `actions/setup-go@v5` + `Build linux/amd64 binary (act fallback)` steps gated by `if: ${{ env.ACT }}`.

- [x] **Taskfile.yml — rewire `test:*` to `ci:*` direct**
  - [x] `test:e2e` iterates TARGETS and invokes `ci:e2e:run` (with `build:linux` dep).
  - [x] `test:e2e:one` invokes `ci:e2e:run` (with `build:linux` dep).
  - [x] `test:integration` invokes `ci:integration:run` directly.
  - [x] `test:sudo` invokes `ci:sudo:run` directly.
  - [x] `test:itest:one` invokes `ci:itest:run` (with `build:linux` dep).
  - [x] Add `test:itest` top-level that invokes `ci:itest`.
  - [x] Add `test:e2e:one-via-act` + `test:itest:one-via-act` as opt-in act variants.
  - [x] Expand the section header comment documenting ECONNRESET rationale.

- [x] **.golangci.yml — errcheck exclude-functions**
  - [x] Add `fmt.Fprint`, `fmt.Fprintf`, `fmt.Fprintln`, `(io.Writer).Write`, `(net/http.ResponseWriter).Write`, `(*bytes.Buffer).Write`, `(*bytes.Buffer).WriteString`, `(*strings.Builder).Write`, `(*strings.Builder).WriteString`, `(*strings.Builder).WriteByte` to the exclusion list with a doc comment explaining intent.

- [x] **Verification**
  - [x] `task fmt && task lint && task test:unit` green (local-only fast gate per AGENTS.md).
  - [x] `rg -n "//nolint:errcheck // .* stdout" internal/` — confirm any newly-redundant `nolint` directives are tracked for cleanup (low priority; not a blocker).
  - [x] Commit granularly (one commit per file or cohesive pair).
  - [x] Push after task completes.
