package config

import (
	"os"

	hamserr "github.com/zthxxx/hams/internal/error"
)

// defaultProfileTag is returned when no CLI flag, config value, or
// other source supplies a tag. Matches the existing fallback in
// sanitizePathSegment, so `<store>/default/` is the historical home
// for profile-less stores.
const defaultProfileTag = "default"

// ResolveCLITagOverride picks a single CLI-supplied profile tag from
// the `--tag` and `--profile` flag values, which are aliases. Returns:
//
//   - "" when neither flag is supplied (caller should fall back to
//     the config's `profile_tag` or the hardcoded "default").
//   - the single supplied value when only one flag is non-empty, OR
//     when both are non-empty and equal.
//   - a UsageError when both are non-empty and disagree, so a user
//     who has `--profile=linux` in an old script cannot silently
//     have it masked by a newly-added `--tag=macOS`.
//
// Accepting the two values as plain strings (rather than
// `*provider.GlobalFlags`) avoids an import cycle: the provider
// package already imports config, so config cannot re-import provider.
func ResolveCLITagOverride(cliTag, cliProfile string) (string, error) {
	if cliTag != "" && cliProfile != "" && cliTag != cliProfile {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			"--tag and --profile are aliases; pass only one",
			"Remove either --tag="+cliTag+" or --profile="+cliProfile,
		)
	}
	if cliTag != "" {
		return cliTag, nil
	}
	return cliProfile, nil
}

// ResolveActiveTag returns the effective profile tag for a command
// invocation, combining CLI override + merged config + the hardcoded
// default "default". It composes ResolveCLITagOverride's result with
// the loaded config.
//
// Precedence (highest wins):
//
//  1. `--tag` / `--profile` on the command line (via
//     ResolveCLITagOverride).
//  2. `profile_tag:` in the merged config.
//  3. The literal string "default".
//
// The returned tag is NOT sanitized — Config.ProfileDir() does that
// when composing the on-disk path. Callers writing tag back to a
// config file should sanitize themselves (sanitizePathSegment).
func ResolveActiveTag(cfg *Config, cliTag, cliProfile string) (string, error) {
	override, err := ResolveCLITagOverride(cliTag, cliProfile)
	if err != nil {
		return "", err
	}
	if override != "" {
		return override, nil
	}
	if cfg != nil && cfg.ProfileTag != "" {
		return cfg.ProfileTag, nil
	}
	return defaultProfileTag, nil
}

// HostnameLookup is the os.Hostname seam used by DeriveMachineID.
// Swapped in tests to return deterministic values without mutating
// the host's real hostname. Production value is os.Hostname.
var HostnameLookup = os.Hostname //nolint:gochecknoglobals // DI seam for tests

// DeriveMachineID returns a sanitized default machine_id for the
// auto-init path. Order: $HAMS_MACHINE_ID env → os.Hostname() →
// "default". Always runs the result through sanitizePathSegment so
// a hostname like "my.box.local" cannot inject path components into
// `.state/<machine-id>/…`.
func DeriveMachineID() string {
	if env := os.Getenv("HAMS_MACHINE_ID"); env != "" {
		return sanitizePathSegment(env, defaultProfileTag)
	}
	name, err := HostnameLookup()
	if err != nil || name == "" {
		return defaultProfileTag
	}
	return sanitizePathSegment(name, defaultProfileTag)
}
