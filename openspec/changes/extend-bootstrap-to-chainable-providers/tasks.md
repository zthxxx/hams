# Tasks

## 1. pnpm

- [ ] 1.1 Rewrite `internal/provider/builtin/pnpm/pnpm.go` `Bootstrap(_)` to return `*provider.BootstrapRequiredError{Provider: "pnpm", Binary: "pnpm", Script: "npm install -g pnpm"}` when LookPath fails.
- [ ] 1.2 Update `Manifest().DependsOn[0]` to add `Script: "npm install -g pnpm"`.
- [ ] 1.3 Unit test `pnpm_bootstrap_test.go`: `pnpm` missing → structured error with correct script; pnpm present → nil.

## 2. duti

- [ ] 2.1 Rewrite `internal/provider/builtin/duti/duti.go` `Bootstrap(_)` symmetric to pnpm.
- [ ] 2.2 Update `Manifest().DependsOn` to include `{Provider: "brew", Script: "brew install duti"}` (currently no DependsOn declared).
- [ ] 2.3 Unit test `duti_bootstrap_test.go`: missing → structured error; present → nil.

## 3. mas

- [ ] 3.1 Rewrite `internal/provider/builtin/mas/mas.go` `Bootstrap(_)` symmetric.
- [ ] 3.2 Update `Manifest().DependsOn` to include `{Provider: "brew", Script: "brew install mas"}`.
- [ ] 3.3 Unit test `mas_bootstrap_test.go`.

## 4. ansible

- [ ] 4.1 Rewrite `internal/provider/builtin/ansible/ansible.go` `Bootstrap(_)`. Binary is `ansible-playbook` (not `ansible`). Script is `pipx install --include-deps ansible`.
- [ ] 4.2 Update `Manifest().DependsOn` to include `{Provider: "bash", Script: "pipx install --include-deps ansible"}`.
- [ ] 4.3 Add a prerequisite note in the surrounding error path (or in the provider-specific error message) pointing at `apt install pipx` / `brew install pipx` so users without pipx see the chain.
- [ ] 4.4 Unit test `ansible_bootstrap_test.go`.

## 5. Spec + verification

- [ ] 5.1 Apply the 4 ADDED requirements + 1 skip-list requirement from `specs/builtin-providers/spec.md` into the main `openspec/specs/builtin-providers/spec.md` (manual application; the auto-sync header-matching bug is known).
- [ ] 5.2 `task fmt` clean.
- [ ] 5.3 `task lint:go` clean.
- [ ] 5.4 `task test:unit` green with `-race` (incl. 4 new bootstrap tests).
- [ ] 5.5 `task ci:itest:run PROVIDER=pnpm` green — pnpm integration test still passes (should be unchanged since it pre-installs pnpm in the Dockerfile).
- [ ] 5.6 `task ci:itest:run PROVIDER=ansible` green — same.
- [ ] 5.7 Decision: ship integration-test variants that start WITHOUT the target binary (exercising --bootstrap end-to-end)? Per design.md: recommended NO (unit tests cover the orchestration branch-by-branch, cycle-5 precedent).
- [ ] 5.8 `openspec validate extend-bootstrap-to-chainable-providers --strict` clean.

## 6. Archive

- [ ] 6.1 `/opsx:verify extend-bootstrap-to-chainable-providers` — 9 scenarios (4 "provider missing produces error" + 4 implicit "present returns nil" covered by unit tests + 1 "skipped providers retain plain error" verified by grep) all map to named tests or code.
- [ ] 6.2 `/opsx:archive extend-bootstrap-to-chainable-providers` — prefer `--skip-specs` + manual delta application (known auto-sync bug).
- [ ] 6.3 Update `AGENTS.md` "Current Task" section with cycle summary.
