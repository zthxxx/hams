# Tasks: Typed i18n message-key catalog

- [x] **Key catalog**
  - [x] Create `internal/i18n/keys.go` with exported `const` for every existing key.
  - [x] Group keys by capability with doc comments explaining call-site context.

- [x] **Call-site rewrite**
  - [x] `internal/cli/apply.go` → 4 call-sites.
  - [x] `internal/cli/autoinit.go` → 2 call-sites.
  - [x] `internal/config/resolve.go` → 1 call-site.
  - [x] `internal/provider/builtin/git/unified.go` → 7 call-sites.

- [x] **Catalog-coherence test**
  - [x] `TestCatalogCoherence_EveryTypedKeyResolves` reads en.yaml + zh-CN.yaml directly and asserts every typed constant has a translation.
  - [x] Hand-maintained list of constants inside the test — adding a new `const` without also extending the list fails the test the first time it runs in CI.

- [x] **Verification**
  - [x] `go test ./internal/i18n/...` PASS.
  - [x] `go build ./...` 0 errors.
  - [x] `task fmt && task lint && task test:unit` green (pre-commit hook).
