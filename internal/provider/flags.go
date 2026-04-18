package provider

import (
	"io"
	"os"
)

// GlobalFlags holds flags that appear between `hams` and the provider name.
type GlobalFlags struct {
	Debug   bool
	DryRun  bool
	JSON    bool
	NoColor bool
	Config  string
	Store   string
	// Profile is the legacy alias for Tag. Both `--tag` and `--profile`
	// map here so existing CI scripts keep working. The
	// `config.ResolveCLITagOverride` helper collapses Tag + Profile into
	// a single value and loud-errors when they disagree. New code SHOULD
	// read Tag directly.
	Profile string
	// Tag is the canonical `--tag=<tag>` flag. Populated alongside
	// Profile; consult `config.ResolveCLITagOverride(flags.Tag,
	// flags.Profile)` for the resolved value.
	Tag string

	// Out is the sink for user-facing stdout (dry-run previews, list
	// output, diagnostic prose). When nil, callers SHOULD resolve it
	// via Stdout(). The seam exists so unit tests can inject a
	// bytes.Buffer instead of mutating the process-global os.Stdout,
	// which races under -race when t.Parallel() is in play.
	Out io.Writer
	// Err is the sink for user-facing stderr (interactive prompts,
	// bootstrap consent, progress spinners). Nil means fall back to
	// Stderr().
	Err io.Writer
}

// Stdout returns the configured Out writer, defaulting to os.Stdout when nil.
// Calling through this method (rather than reading flags.Out directly) keeps
// tests race-free: t.Parallel() subtests inject per-test bytes.Buffer writers
// without racing on the process-global os.Stdout.
func (f *GlobalFlags) Stdout() io.Writer {
	if f == nil || f.Out == nil {
		return os.Stdout
	}
	return f.Out
}

// Stderr returns the configured Err writer, defaulting to os.Stderr when nil.
// Same rationale as Stdout().
func (f *GlobalFlags) Stderr() io.Writer {
	if f == nil || f.Err == nil {
		return os.Stderr
	}
	return f.Err
}

// EffectiveTag returns the CLI-supplied tag value, collapsing
// flags.Tag and flags.Profile into the single canonical string.
// Does NOT detect conflicts — call config.ResolveCLITagOverride for
// that (ideally once at the top of each CLI action). This method is
// a convenience shim for call-sites that have already resolved or
// that legitimately don't care which of the two was typed
// (e.g., string-formatting an error message).
func (f *GlobalFlags) EffectiveTag() string {
	if f == nil {
		return ""
	}
	if f.Tag != "" {
		return f.Tag
	}
	return f.Profile
}
