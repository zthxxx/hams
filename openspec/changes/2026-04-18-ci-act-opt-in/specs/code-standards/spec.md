# Spec delta: code-standards — errcheck exclusions for writer-bound helpers

## MODIFIED Requirement: Go linting — errcheck

The `errcheck` linter SHALL treat stdout/stderr/io.Writer-bound helpers as explicitly excluded from unchecked-error reporting. `errcheck.exclude-functions` in `.golangci.yml` SHALL list at minimum:

- `fmt.Fprint`, `fmt.Fprintf`, `fmt.Fprintln` — writer-bound print variants used throughout the CLI for stdout/stderr user-facing prose.
- `(io.Writer).Write` — bare writer interface call, used through `flags.Stdout()` / `flags.Stderr()` DI seams.
- `(net/http.ResponseWriter).Write` — HTTP handler writes; any remote listener is outside hams today but the rule is portable.
- `(*bytes.Buffer).Write`, `(*bytes.Buffer).WriteString`, `(*strings.Builder).Write`, `(*strings.Builder).WriteString`, `(*strings.Builder).WriteByte` — in-memory builder writes never fail (documented in Go stdlib).

Rationale: write failures on these sinks have no meaningful recovery path (the process cannot communicate the failure back to a caller that can no longer receive it, because the sink is how it would). A dedicated `//nolint:errcheck` directive on every call-site was the previous workaround; the explicit exclusion list collapses hundreds of lint diffs into one config entry.

#### Scenario: a new CLI handler writes to `flags.Stdout()` without checking error

- **Given** code like `fmt.Fprintln(flags.Stdout(), "[dry-run] Would install: ...")`
- **When** `golangci-lint run` executes
- **Then** no `errcheck` violation is reported for this call-site.

#### Scenario: a package explicitly wants to check write errors

- **Given** an HTTP handler that needs to detect a broken client connection
- **When** the developer assigns the error: `_, err := w.Write(payload); if err != nil { ... }`
- **Then** the exclusion does not suppress the developer's explicit check — errcheck only affects unchecked writes.
