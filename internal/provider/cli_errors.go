package provider

import (
	"fmt"

	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/i18n"
)

// UsageRequiresResource builds a UserFacingError for the ubiquitous
// "<provider> <verb> requires a <resource>" message + "Usage: hams
// <provider> <verb> <placeholder>" hint. Every package-class builtin
// provider emits this exact shape; consolidating it here is both the
// CLAUDE.md "shared abstractions" task (#3) AND the i18n-catalog
// wiring (task #2) — one function, 14 adopters.
//
// Example:
//
//	if len(args) == 0 {
//	    return provider.UsageRequiresResource(cliName, "install", "package name", "package")
//	}
func UsageRequiresResource(name, verb, resource, placeholder string) error {
	return hamserr.NewUserError(hamserr.ExitUsageError,
		i18n.Tf(i18n.ProviderErrRequiresResource, map[string]any{
			"Provider": name, "Verb": verb, "Resource": resource,
		}),
		i18n.Tf(i18n.ProviderUsageBasic, map[string]any{
			"Provider": name, "Verb": verb, "Placeholder": placeholder,
		}),
	)
}

// UsageRequiresAtLeastOne is the symmetric "<provider> <verb> requires at
// least one <resource>" variant. Fired when the args list reduces to
// zero valid entries after argument extraction (e.g. `apt install
// -y`, where the extractor drops `-y` and no packages remain).
func UsageRequiresAtLeastOne(name, verb, resource, placeholder string) error {
	return hamserr.NewUserError(hamserr.ExitUsageError,
		i18n.Tf(i18n.ProviderErrRequiresAtLeastOne, map[string]any{
			"Provider": name, "Verb": verb, "Resource": resource,
		}),
		i18n.Tf(i18n.ProviderUsageBasic, map[string]any{
			"Provider": name, "Verb": verb, "Placeholder": placeholder,
		}),
	)
}

// DryRunInstall prints "[dry-run] Would install: <cmd>" to the flags'
// stdout writer, honoring the DI seam so tests can assert the line
// without mutating os.Stdout.
func DryRunInstall(flags *GlobalFlags, cmd string) {
	fmt.Fprintln(flags.Stdout(), i18n.Tf(i18n.ProviderDryRunWouldInstall, map[string]any{"Cmd": cmd}))
}

// DryRunRemove prints "[dry-run] Would remove: <cmd>".
func DryRunRemove(flags *GlobalFlags, cmd string) {
	fmt.Fprintln(flags.Stdout(), i18n.Tf(i18n.ProviderDryRunWouldRemove, map[string]any{"Cmd": cmd}))
}

// DryRunRun prints "[dry-run] Would run: <cmd>". Generic sibling of
// DryRunInstall / DryRunRemove for providers whose operation isn't a
// package install (bash run, git passthrough, etc.).
func DryRunRun(flags *GlobalFlags, cmd string) {
	fmt.Fprintln(flags.Stdout(), i18n.Tf(i18n.ProviderDryRunWouldRun, map[string]any{"Cmd": cmd}))
}
