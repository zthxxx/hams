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
	Profile string
	Tag     string // --tag override for `hams apply`; empty means defer to config/default.

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

// Stdout returns the configured out writer, defaulting to os.Stdout when nil.
func (f *GlobalFlags) Stdout() io.Writer {
	if f == nil || f.Out == nil {
		return os.Stdout
	}
	return f.Out
}

// Stderr returns the configured err writer, defaulting to os.Stderr when nil.
func (f *GlobalFlags) Stderr() io.Writer {
	if f == nil || f.Err == nil {
		return os.Stderr
	}
	return f.Err
}
