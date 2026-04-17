package provider

import (
	"fmt"
	"log/slog"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/state"
)

// AcquireMutationLock acquires the single-writer state lock per
// `cli-architecture/spec.md` §"Lock file for single-writer enforcement".
// The spec explicitly mandates lock acquisition during ANY mutating
// operation: `apply`, `refresh`, AND every provider `install` / `remove`
// CLI handler. Pre-cycle-221 only `runApply` acquired the lock,
// allowing a `hams cargo install ripgrep` to race with an in-flight
// `hams apply` and clobber state writes.
//
// Caller pattern:
//
//	release, err := provider.AcquireMutationLock(cfg.StateDir(), "cargo install")
//	if err != nil {
//	    return err
//	}
//	defer release()
//
// On lock conflict, returns an `ExitLockError` (code 3) wrapping the
// underlying lock-file message (PID, command, started-at). On
// successful acquisition, returns a release closure that swallows
// (but logs) any release error — release failure shouldn't mask the
// caller's primary error path.
//
// `cmdName` is the human-readable command label written to the lock
// file (e.g., "cargo install", "refresh"). It surfaces in the error
// message of the next caller that hits the conflict, so make it
// specific enough to identify which command is currently holding the
// lock.
//
// SHALL NOT be called in dry-run mode — dry-run is a pure preview
// per the global `--dry-run` flag's "no side effects" contract;
// lock acquisition would write the .lock file and contradict that.
// Callers MUST gate this on `!flags.DryRun`.
func AcquireMutationLock(stateDir, cmdName string) (release func(), err error) {
	lock := state.NewLock(stateDir)
	if lockErr := lock.Acquire(cmdName); lockErr != nil {
		return nil, hamserr.NewUserError(hamserr.ExitLockError, lockErr.Error(),
			fmt.Sprintf("Remove %s/.lock if the previous run crashed", stateDir),
		)
	}
	return func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			slog.Error("failed to release state lock", "error", releaseErr, "stateDir", stateDir)
		}
	}, nil
}

// AcquireMutationLockFromCfg is the per-provider convenience wrapper
// for `AcquireMutationLock` used by every CLI install/remove handler
// (cycle 222 sweep). It pulls `stateDir` from `cfg.StateDir()` and
// short-circuits to a no-op release when `flags.DryRun` is true so
// dry-run keeps its "zero side effects" contract (acquiring would
// itself create the .lock file).
//
// Caller pattern at the top of every CLI handler, after the args
// validation + dry-run preview but before any runner.Install /
// hf.Write / sf.Save:
//
//	release, lockErr := provider.AcquireMutationLockFromCfg(
//	    p.effectiveConfig(flags), flags, "cargo install")
//	if lockErr != nil {
//	    return lockErr
//	}
//	defer release()
//
// Passing a nil flags is treated as "dry-run mode": returns the
// no-op release. This keeps callers from having to nil-check before
// invoking us.
func AcquireMutationLockFromCfg(cfg *config.Config, flags *GlobalFlags, cmdName string) (release func(), err error) {
	if flags == nil || flags.DryRun {
		return func() {}, nil
	}
	if cfg == nil {
		return nil, hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured (cfg is nil)",
			"Set store_path in hams config or pass --store",
		)
	}
	return AcquireMutationLock(cfg.StateDir(), cmdName)
}
