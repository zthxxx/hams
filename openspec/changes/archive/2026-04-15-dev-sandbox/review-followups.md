# Codex Review — Follow-ups Not Related to dev-sandbox

`/codex:review --wait --base 483714b` ran on 2026-04-15 against a very
historical base (`483714b`, from an early scaffolding phase). Because of the
broad base, the review surfaced issues in code paths unrelated to this
dev-sandbox change. They are recorded here so they are not lost, and
scheduled as a separate follow-up change rather than blocking dev-sandbox
archive.

## Findings

### FALSE POSITIVE

**FP1 — `docs/.github/workflows/docs.yml:39` publishes `./docs/dist` but Next
writes to `docs/out`.**

Codex suggested this was a regression. Verified against current state:
`docs/dist/index.html` contains the actual exported landing page
(`<title>Index – hams</title>` + MDX-rendered content), and
`python3 -m http.server` served the whole tree correctly in earlier
`/opsx:apply` verification. With Next.js 16 + `output: 'export'`,
`distDir: 'dist'` directs the export output there (the pre-Next-14 behavior
where export always went to `out/` no longer applies). No action.

### P1 — schedule for a follow-up change

**P1-a — Self-upgrade downloads without checksum verification**
`internal/cli/commands.go:585` — `runBinaryUpgrade` calls
`selfupdate.ReplaceBinary(exePath, body, "")` with an empty checksum. The
release pipeline emits a `checksums.txt`, and `ReplaceBinary` already
supports verification; the upgrade path must resolve the expected hash for
the downloaded asset and pass it so a truncated/tampered download is
rejected before rename.

### P2 — schedule for a follow-up change

**P2-a — Local bare repos bootstrap produces an unusable store**
`internal/cli/bootstrap.go:61-64` — if `--from-repo` is a local bare
repository (e.g., `/tmp/store.git`), the code accepts the path because
`HEAD` exists and returns the bare directory as the store root. `runApply`
then looks for checked-out files inside the bare repo and finds nothing.
Bare local repos must be cloned (via `file://<path>`), not treated as
ready stores.

**P2-b — `hams config edit` breaks when `$EDITOR` contains arguments**
`internal/cli/commands.go:167-175` — `exec.CommandContext(ctx, editor,
configPath)` treats the entire `$EDITOR` string as the executable name,
so `code --wait` or `nvim -f` fail with "file not found". Needs
shell-style splitting (`strings.Fields` or `shellwords.Split`) before
`append(..., configPath)`.

## Out of scope for dev-sandbox

None of these findings regress dev-sandbox code. `commands.go`,
`bootstrap.go`, and `docs.yml` (for the FP) are all outside the dev-sandbox
delta.

## Scope decision (architect + user subagents, 2026-04-15)

Per user instruction "如果你有任何的疑问或问题，你需要通过创建一个 Agent
team，以架构式的视角和用户的视角来相互讨论", two independent subagents
were consulted:

**Architect's recommendation**: *Defer all three real findings (P1-a,
P2-a, P2-b) to a new `cli-self-upgrade-and-edit-fixes` change; archive
dev-sandbox as-is.*

Rationale (compressed):
- OpenSpec changes are coherent shippable units; bundling unrelated CLI
  hardening into dev-sandbox violates scoping discipline and muddies the
  archived spec deltas.
- All three findings are pre-existing — surfaced because `--base 483714b`
  covers the entire project history, not a regression introduced by
  dev-sandbox.
- The CLAUDE.md loop clause "If yes: uncheck and fix" presupposes
  in-scope findings; the correct adaptation for an out-of-scope finding
  is to record it as a follow-up (already done here).
- Archive-then-fix preserves reviewability: a focused "CLI hardening"
  change is easier to review than fixes buried inside a sandbox PR.

**User's perspective** (daily hams user, not the maintainer):
- Ranks the findings by real-world impact: **P1-a (checksum) > P2-b
  (EDITOR) > P2-a (bare repo)**. The FP doesn't bite.
- "I don't care [about atomicity]. Did the fix ship?' is the only
  question."
- "Ship dev-sandbox in a day. Track the three real fixes as a follow-up.
  Two-to-three days of delay to bundle unrelated fixes is process
  theater."
- Caveat on P1-a: *"This needs a named follow-up with a date, not a
  vague `tasks.md` line. Fix it this week. Don't let it rot."*

**Combined decision**:

1. Dev-sandbox archives now; these findings stay out of its delta.
2. A new change `cli-self-upgrade-and-edit-fixes` (proposal below) is
   opened immediately after dev-sandbox archive, committing to
   implementation this week because of the security-relevance of P1-a.
3. This review-followups.md stays in the dev-sandbox archive as
   provenance: anyone reading the archived change can trace why these
   findings were deferred and where they went.

## Follow-up change proposal

**ID**: `cli-self-upgrade-and-edit-fixes`
**Target ship date**: within one week of dev-sandbox archive.

**Proposal skeleton** (to live in
`openspec/changes/cli-self-upgrade-and-edit-fixes/proposal.md` after
dev-sandbox archive):

- **What**: Fix three pre-existing CLI hardening defects surfaced by the
  2026-04-15 codex review of dev-sandbox:
  1. **P1-a — Self-upgrade integrity verification**. Resolve the expected
     SHA256 from the release's `checksums.txt` before calling
     `selfupdate.ReplaceBinary(exePath, body, <checksum>)`. Reject
     mismatched downloads before the atomic rename.
  2. **P2-a — Bare-repo bootstrap**. In `internal/cli/bootstrap.go`,
     detect local bare repos (directory whose `HEAD` exists but `.git`
     does not) and route them through the clone path (`file://<path>`),
     not the direct-use path. Document the behavior in
     `openspec/specs/cli-architecture/spec.md`.
  3. **P2-b — `$EDITOR` arg splitting**. In the `hams config edit`
     handler, split `$EDITOR` / `$VISUAL` with `shellwords.Split`
     (preferred over `strings.Fields` to respect quoted args), then
     append the config path before `exec.CommandContext`.

- **Why**: Single, focused CLI-hardening change. P1-a carries security
  weight (integrity verification is the user's main defense against a
  compromised release channel); P2-b is a day-one papercut for every
  VSCode/Cursor user; P2-a is a niche but confusing silent failure.

- **Impact**:
  - Spec deltas under `cli-architecture` (self-upgrade + from-repo
    bootstrap) and `cli-config` (editor invocation).
  - Tests: DI-mocked HTTP for checksum verification, tempdir bare-repo
    fixture, table-driven test for `EDITOR` splitting (incl.
    `'code --wait'`, `'"nvim -f"'`, `'vim'`, unset).
  - No user-visible runtime behavior changes for the happy path; the
    failing-download case now errors loudly with a checksum mismatch
    message.
