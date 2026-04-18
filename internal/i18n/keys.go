package i18n

// Message keys used across the CLI layer. Centralizing the identifiers
// here is the single source of truth for translators — any string
// referenced from a key in this file MUST have a corresponding entry
// in every `locales/<lang>.yaml` file. Missing entries fall back to
// English via localizer precedence; missing English returns the key
// itself (see i18n.go).
//
// Naming convention: `<capability>.<component>.<short-id>`
//
//   - capability: app, autoinit, apply, refresh, git, cli, ufe, …
//   - component:  err (error message), usage (help text), status
//     (info/progress), suggest (remediation hint).
//   - short-id:   terse English descriptor (kebab-case or snake_case,
//     matching the existing YAML entry).
//
// This file is pure constants — no formatting, no side effects. All
// lookups go through T() / Tf() in i18n.go. A catalog-coherence
// unit test (i18n_test.go TestCatalogCoherence) asserts that every
// constant declared here resolves in both en.yaml and zh-CN.yaml, so
// adding a key without its translations fails CI.
const (
	// ---- App metadata ----.

	// AppTitle — top-of-help title string.
	AppTitle = "app.title"

	// ---- Auto-init status lines ----.

	// AutoInitGlobalConfigCreated — stderr notice emitted when
	// EnsureGlobalConfig writes a fresh ~/.config/hams/hams.config.yaml
	// on a pristine host. Template vars: {{.Path}} {{.Tag}}
	// {{.MachineID}}.
	AutoInitGlobalConfigCreated = "autoinit.global_config_created"

	// AutoInitStoreCreated — stderr notice emitted when
	// EnsureStoreReady bootstraps the default store directory.
	// Template var: {{.Path}}.
	AutoInitStoreCreated = "autoinit.store_created"

	// AutoInitDryRunGlobalConfig — dry-run preview line emitted by
	// EnsureGlobalConfig when --dry-run is set so the user sees what
	// WOULD happen without any filesystem side effects. Template var:
	// {{.Path}}.
	AutoInitDryRunGlobalConfig = "autoinit.dry_run.global_config"

	// AutoInitDryRunStore — dry-run preview line emitted by
	// EnsureStoreReady when --dry-run is set. Template var:
	// {{.Path}}.
	AutoInitDryRunStore = "autoinit.dry_run.store"

	// ---- No-store-configured UFE family ----.

	// UFENoStoreConfigured — primary message when a command needs a
	// store but none is resolvable.
	UFENoStoreConfigured = "ufe.no_store_configured"
	// UFENoStoreConfiguredSuggestClone — "run `hams apply --from-repo=X`".
	UFENoStoreConfiguredSuggestClone = "ufe.no_store_configured.suggest_clone"
	// UFENoStoreConfiguredSuggestSet — "set store_path in hams.config.yaml".
	UFENoStoreConfiguredSuggestSet = "ufe.no_store_configured.suggest_set"
	// UFENoStoreConfiguredSuggestInit — "run `hams store init`".
	UFENoStoreConfiguredSuggestInit = "ufe.no_store_configured.suggest_init"
	// UFENoStoreConfiguredOptOut — primary message when user has set
	// HAMS_NO_AUTO_INIT=1 so auto-init is suppressed.
	UFENoStoreConfiguredOptOut = "ufe.no_store_configured.opt_out"
	// UFENoStoreConfiguredOptOutSuggest — "unset HAMS_NO_AUTO_INIT to
	// enable auto-init on first run".
	UFENoStoreConfiguredOptOutSuggest = "ufe.no_store_configured.opt_out_suggest"

	// ---- Apply status ----.

	// ApplyDryRunHeader — leading line of `hams apply --dry-run` output.
	ApplyDryRunHeader = "apply.dry_run_header"
	// ApplyNoProvidersMatch — "no providers matched the --only/--except filters".
	ApplyNoProvidersMatch = "apply.no_providers_match"

	// ---- Refresh status ----.

	// RefreshNoProvidersMatch — refresh sibling of ApplyNoProvidersMatch.
	RefreshNoProvidersMatch = "refresh.no_providers_match"

	// ---- hams git dispatcher (usage + errors) ----.

	// GitUsageHeader — top-level `hams git requires a subcommand` line.
	GitUsageHeader = "git.usage.header"
	// GitUsageSuggestMain — "Recorded subcommands:" intro.
	GitUsageSuggestMain = "git.usage.suggest_main"
	// GitUsageSuggestSubcommands — bullet list of config/clone/remove/list.
	GitUsageSuggestSubcommands = "git.usage.suggest_subcommands"
	// GitUsageExampleConfig — `hams git config user.email ...` example.
	GitUsageExampleConfig = "git.usage.example_config"
	// GitUsageExampleClone — `hams git clone <url> <path>` example.
	GitUsageExampleClone = "git.usage.example_clone"
	// GitUnknownSubcommand — "unknown subcommand %q" with {{.Sub}}.
	GitUnknownSubcommand = "git.unknown_subcommand"

	// ---- CLI framework errors ----.

	// CLIErrTagProfileConflict — `--tag` and `--profile` supplied
	// with different values (emitted by config.ResolveCLITagOverride).
	CLIErrTagProfileConflict = "cli.err.tag-profile-conflict"
)
