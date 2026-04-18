package i18n

// Message keys used across the CLI layer. Centralizing the identifiers
// here is the single source of truth for translators — any string
// referenced from a key in this file MUST have a corresponding entry in
// every locales/<lang>.yaml file (missing entries fall back to English
// via localizer precedence, see i18n.go).
//
// Naming convention: <capability>.<component>.<short-id>
//
//   - capability: cli, store, apply, provider, config, …
//   - component: err (error message), usage (help text), prompt
//     (interactive prompt), status (info/progress), suggestion
//     (remediation hint).
//   - short-id: terse English descriptor, kebab-case.
//
// This file is pure constants — no formatting, no side effects. All
// lookups go through T() / Tf() so a missing key degrades gracefully
// (returns the key itself, which at least shows up in test output and
// grep).
const (
	// ---- CLI framework errors ----.

	// CLIErrTagProfileConflict — --tag and --profile supplied with
	// different values. Added in 2026-04-18.
	CLIErrTagProfileConflict = "cli.err.tag-profile-conflict"

	// CLIErrNoStore — no store directory resolvable. Points at
	// --from-repo / `hams store init` / editing hams.config.yaml.
	CLIErrNoStore = "cli.err.no-store"

	// CLIErrBootstrapConflict — --bootstrap + --no-bootstrap combo.
	CLIErrBootstrapConflict = "cli.err.bootstrap-conflict"

	// CLIErrFromRepoVsStore — --from-repo + --store combo.
	CLIErrFromRepoVsStore = "cli.err.from-repo-vs-store"

	// CLIErrOnlyExceptExclusive — --only + --except combo.
	CLIErrOnlyExceptExclusive = "cli.err.only-except"

	// ---- Apply path ----.

	// ApplyStatusDryRunPreview — leading prose line before the
	// dry-run action table.
	ApplyStatusDryRunPreview = "apply.status.dry-run-preview"

	// ApplyStatusNoProvidersMatch — no providers matched the
	// --only/--except filters.
	ApplyStatusNoProvidersMatch = "apply.status.no-providers-match"

	// ApplyStatusSessionStarted — emitted by SetupLogging; used by
	// integration tests to assert log output fires.
	ApplyStatusSessionStarted = "apply.status.session-started"

	// ---- Bootstrap / profile init ----.

	// BootstrapStatusProfileMissing — stderr notice shown when
	// profile_tag / machine_id are absent and the user gets prompted.
	BootstrapStatusProfileMissing = "bootstrap.status.profile-missing"

	// BootstrapErrNotConfigured — non-TTY + missing profile fields.
	BootstrapErrNotConfigured = "bootstrap.err.not-configured"

	// BootstrapStatusAutoInitialized — INFO-level log emitted by the
	// auto-init path when the global config is first materialized
	// from --tag on a fresh machine.
	BootstrapStatusAutoInitialized = "bootstrap.status.auto-initialized"

	// ---- Store status ----.

	// StoreStatusLabelPath — "Store path:".
	StoreStatusLabelPath = "store.status.label.path"
	// StoreStatusLabelProfile — "Profile tag:".
	StoreStatusLabelProfile = "store.status.label.profile"
	// StoreStatusLabelMachine — "Machine ID:".
	StoreStatusLabelMachine = "store.status.label.machine"
	// StoreStatusLabelProfileDir — "Profile dir:".
	StoreStatusLabelProfileDir = "store.status.label.profile-dir"
	// StoreStatusLabelStateDir — "State dir:".
	StoreStatusLabelStateDir = "store.status.label.state-dir"
	// StoreStatusLabelHamsfiles — "Hamsfiles:".
	StoreStatusLabelHamsfiles = "store.status.label.hamsfiles"
	// StoreStatusLabelGit — "Git status:".
	StoreStatusLabelGit = "store.status.label.git"
)
