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

	// CLIErrNoPositionalArgs — emitted by `hams apply`/`hams refresh`
	// when the user passes a stray positional. Template vars:
	// {{.Cmd}} {{.Arg}}.
	CLIErrNoPositionalArgs = "cli.err.no-positional-args"
	// CLIErrNoPositionalArgsSuggestFilter — filter suggestion. Template var: {{.Cmd}}.
	CLIErrNoPositionalArgsSuggestFilter = "cli.err.no-positional-args.suggest-filter"
	// CLIErrNoPositionalArgsSuggestAll — "all" suggestion. Template vars: {{.Verb}} {{.Cmd}}.
	CLIErrNoPositionalArgsSuggestAll = "cli.err.no-positional-args.suggest-all"

	// CLIErrOnlyExceptConflict — `--only` and `--except` together.
	CLIErrOnlyExceptConflict = "cli.err.only-except-conflict"
	// CLIErrOnlyExceptConflictSuggest — follow-up hint.
	CLIErrOnlyExceptConflictSuggest = "cli.err.only-except-conflict.suggest"

	// CLIErrBootstrapModeConflict — `--bootstrap` and `--no-bootstrap` together.
	CLIErrBootstrapModeConflict = "cli.err.bootstrap-mode-conflict"
	// CLIErrBootstrapModeConflictSuggest — follow-up hint.
	CLIErrBootstrapModeConflictSuggest = "cli.err.bootstrap-mode-conflict.suggest"

	// CLIErrFromRepoStoreConflict — `--from-repo` and `--store` together.
	CLIErrFromRepoStoreConflict = "cli.err.from-repo-store-conflict"

	// CLIErrStorePathInvalid — store_path does not exist or is not a directory. Template var: {{.Path}}.
	CLIErrStorePathInvalid = "cli.err.store-path-invalid"
	// CLIErrStorePathInvalidSuggestFix — fix hint.
	CLIErrStorePathInvalidSuggestFix = "cli.err.store-path-invalid.suggest-fix"

	// CLIErrProfileNotFound — profile directory missing. Template vars: {{.Tag}} {{.Dir}}.
	CLIErrProfileNotFound = "cli.err.profile-not-found"
	// CLIErrProfileNotFoundSuggestList — suggest `ls`. Template var: {{.Store}}.
	CLIErrProfileNotFoundSuggestList = "cli.err.profile-not-found.suggest-list"
	// CLIErrProfileNotFoundSuggestCreate — suggest `mkdir -p`. Template var: {{.Dir}}.
	CLIErrProfileNotFoundSuggestCreate = "cli.err.profile-not-found.suggest-create"

	// CLIErrConfigFileMissing — `--config=<path>` file not found. Template var: {{.Path}}.
	CLIErrConfigFileMissing = "cli.err.config-file-missing"
	// CLIErrConfigFileMissingSuggestCheck — suggest spelling check.
	CLIErrConfigFileMissingSuggestCheck = "cli.err.config-file-missing.suggest-check"
	// CLIErrConfigFileMissingSuggestCreate — suggest `touch`. Template var: {{.Path}}.
	CLIErrConfigFileMissingSuggestCreate = "cli.err.config-file-missing.suggest-create"
	// CLIErrConfigFileMissingSuggestDefault — suggest dropping --config.
	CLIErrConfigFileMissingSuggestDefault = "cli.err.config-file-missing.suggest-default"

	// CLIErrOnlyEmpty — `--only` empty after whitespace trim.
	CLIErrOnlyEmpty = "cli.err.only-empty"
	// CLIErrExceptEmpty — `--except` empty after whitespace trim.
	CLIErrExceptEmpty = "cli.err.except-empty"

	// CLIErrUnknownProvider — provider names not in registry. Template var: {{.Names}}.
	CLIErrUnknownProvider = "cli.err.unknown-provider"
	// CLIErrUnknownProviderSuggestList — list available. Template var: {{.Names}}.
	CLIErrUnknownProviderSuggestList = "cli.err.unknown-provider.suggest-list"

	// CLIErrProfileNotConfigured — non-TTY fallback missing config. Template var: {{.Missing}}.
	CLIErrProfileNotConfigured = "cli.err.profile-not-configured"

	// CLIErrSudoAcquisitionFailed — sudo canceled/timeout. Template var: {{.Err}}.
	CLIErrSudoAcquisitionFailed = "cli.err.sudo-acquisition-failed"
	// CLIErrSudoAcquisitionFailedSuggestReenter — re-run hint.
	CLIErrSudoAcquisitionFailedSuggestReenter = "cli.err.sudo-acquisition-failed.suggest-reenter"
	// CLIErrSudoAcquisitionFailedSuggestPasswordless — passwordless hint.
	CLIErrSudoAcquisitionFailedSuggestPasswordless = "cli.err.sudo-acquisition-failed.suggest-passwordless" //nolint:gosec // G101: this is a message-key constant, not a credential
	// CLIErrSudoAcquisitionFailedSuggestFilter — filter hint.
	CLIErrSudoAcquisitionFailedSuggestFilter = "cli.err.sudo-acquisition-failed.suggest-filter"

	// CLIErrInterrupted — apply interrupted. Template var: {{.Err}}.
	CLIErrInterrupted = "cli.err.interrupted"
	// CLIErrInterruptedSuggestInspect — inspect hint.
	CLIErrInterruptedSuggestInspect = "cli.err.interrupted.suggest-inspect"
	// CLIErrInterruptedSuggestRerun — re-run hint.
	CLIErrInterruptedSuggestRerun = "cli.err.interrupted.suggest-rerun"

	// ---- Apply extra status lines ----.

	// ApplyExecutionOrderHeader — dry-run header.
	ApplyExecutionOrderHeader = "apply.execution-order-header"
	// ApplyBootstrapDryRun — dry-run bootstrap. Template vars: {{.Provider}} {{.Script}}.
	ApplyBootstrapDryRun = "apply.bootstrap-dry-run"
	// ApplyNoProvidersStateOnly — no state-only without --prune-orphans.
	ApplyNoProvidersStateOnly = "apply.no-providers-state-only"
	// ApplyNoProvidersFiltered — filter excluded all.
	ApplyNoProvidersFiltered = "apply.no-providers-filtered"
	// ApplyNoProvidersOnlyMissing — Template vars: {{.Names}} {{.Verb}}.
	ApplyNoProvidersOnlyMissing = "apply.no-providers-only-missing"
	// ApplyNoProvidersProfileLine — Template vars: {{.Tag}} {{.Dir}}.
	ApplyNoProvidersProfileLine = "apply.no-providers-profile-line"
	// ApplyNoProvidersSuggestInstall — Template var: {{.Provider}}.
	ApplyNoProvidersSuggestInstall = "apply.no-providers-suggest-install"
	// ApplyNoProvidersProfileDirMissing — Template vars: {{.Dir}} {{.Tag}}.
	ApplyNoProvidersProfileDirMissing = "apply.no-providers-profile-dir-missing"
	// ApplyNoProvidersAvailableProfiles — Template var: {{.List}}.
	ApplyNoProvidersAvailableProfiles = "apply.no-providers-available-profiles"
	// ApplyNoProvidersSuggestProfileFix — fix hint.
	ApplyNoProvidersSuggestProfileFix = "apply.no-providers-suggest-profile-fix"
	// ApplySummaryComplete — Template vars: {{.Installed}} {{.Updated}} {{.Removed}} {{.Skipped}} {{.Failed}} {{.ElapsedMs}}.
	ApplySummaryComplete = "apply.summary-complete"
	// ApplySummaryFailedWarning — Template vars: {{.Count}} {{.Names}}.
	ApplySummaryFailedWarning = "apply.summary-failed-warning"
	// ApplySummaryFailedWarningSuggest — suggest debug.
	ApplySummaryFailedWarningSuggest = "apply.summary-failed-warning.suggest"
	// ApplySummarySkippedWarning — Template vars: {{.Count}} {{.Names}}.
	ApplySummarySkippedWarning = "apply.summary-skipped-warning"
	// ApplySummaryStateFailWarning — Template vars: {{.Count}} {{.Names}}.
	ApplySummaryStateFailWarning = "apply.summary-state-fail-warning"
	// ApplySummaryStateFailSuggest — suggest.
	ApplySummaryStateFailSuggest = "apply.summary-state-fail-warning.suggest"
	// ApplySummaryPartialFailureErr — Template vars: {{.Failed}} {{.Skipped}} {{.SaveFailed}}.
	ApplySummaryPartialFailureErr = "apply.summary-partial-failure-err"
	// ApplySummaryPartialFailureSuggestRetry — retry hint.
	ApplySummaryPartialFailureSuggestRetry = "apply.summary-partial-failure-err.suggest-retry"
	// ApplySummaryPartialFailureSuggestDebug — debug hint.
	ApplySummaryPartialFailureSuggestDebug = "apply.summary-partial-failure-err.suggest-debug"
	// ApplyDryRunNoChanges — Template var: {{.ElapsedMs}}.
	ApplyDryRunNoChanges = "apply.dry-run-no-changes"
	// ApplyDryRunPerProviderHeader — Template vars: {{.DisplayName}} {{.Name}}.
	ApplyDryRunPerProviderHeader = "apply.dry-run-per-provider-header"
	// ApplyDryRunNoChangesProvider — Template vars: {{.Count}} {{.Noun}}.
	ApplyDryRunNoChangesProvider = "apply.dry-run-no-changes-provider"
	// ApplyDryRunUnchangedTail — Template vars: {{.Count}} {{.Noun}}.
	ApplyDryRunUnchangedTail = "apply.dry-run-unchanged-tail"
	// ApplyDryRunStateFailErr — Template var: {{.Count}}.
	ApplyDryRunStateFailErr = "apply.dry-run-state-fail-err"
	// ApplyDryRunStateFailErrSuggestPerms — permissions hint.
	ApplyDryRunStateFailErrSuggestPerms = "apply.dry-run-state-fail-err.suggest-perms"
	// ApplyDryRunStateFailErrSuggestNoRefresh — no-refresh hint.
	ApplyDryRunStateFailErrSuggestNoRefresh = "apply.dry-run-state-fail-err.suggest-no-refresh"
	// ApplyDryRunSkippedErr — Template var: {{.Count}}.
	ApplyDryRunSkippedErr = "apply.dry-run-skipped-err"
	// ApplyDryRunSkippedErrSuggestFix — fix hint.
	ApplyDryRunSkippedErrSuggestFix = "apply.dry-run-skipped-err.suggest-fix"
	// ApplyStateFailDriftLine — drift warning body.
	ApplyStateFailDriftLine = "apply.state-fail-drift-line"
	// ApplyNotFoundProfile — TTY notice.
	ApplyNotFoundProfile = "apply.not-found-profile"
	// ApplyProfileTagPrompt — TTY prompt.
	ApplyProfileTagPrompt = "apply.profile-tag-prompt"
	// ApplyProfileMachineIDPrompt — TTY prompt.
	ApplyProfileMachineIDPrompt = "apply.profile-machine-id-prompt"

	// ---- Refresh extra status lines ----.

	// RefreshSummaryComplete — Template vars: {{.Count}} {{.Noun}} {{.ElapsedMs}}.
	RefreshSummaryComplete = "refresh.summary-complete"
	// RefreshSummaryCompleteWithSaveFails — Template vars: {{.Count}} {{.Noun}} {{.FailCount}} {{.FailList}} {{.ElapsedMs}}.
	RefreshSummaryCompleteWithSaveFails = "refresh.summary-complete-with-save-fails"
	// RefreshSummaryCompleteWithProbeFails — Template vars: {{.Count}} {{.Total}} {{.Noun}} {{.ProbeFailCount}} {{.ProbeFailList}} {{.ElapsedMs}}.
	RefreshSummaryCompleteWithProbeFails = "refresh.summary-complete-with-probe-fails"
	// RefreshSummarySaveFailWarning — Template vars: {{.Count}} {{.Names}}.
	RefreshSummarySaveFailWarning = "refresh.summary-save-fail-warning"
	// RefreshInterrupted — Template vars: {{.Done}} {{.Total}} {{.Noun}} {{.ElapsedMs}}.
	RefreshInterrupted = "refresh.interrupted"
	// RefreshStateWrittenHeader — Template vars: {{.Count}} {{.Noun}}.
	RefreshStateWrittenHeader = "refresh.state-written-header"

	// ---- Store commands ----.

	// StoreNothingToCommit — clean tree.
	StoreNothingToCommit = "store.nothing-to-commit"
	// StoreStatusPathMissing — Template var: {{.Path}}.
	StoreStatusPathMissing = "store.status.path-missing"
	// StoreStatusPathMissingLine1 — follow-up.
	StoreStatusPathMissingLine1 = "store.status.path-missing.line1"
	// StoreStatusPathMissingLine2 — follow-up.
	StoreStatusPathMissingLine2 = "store.status.path-missing.line2"
	// StoreStatusPath — Template var: {{.Path}}.
	StoreStatusPath = "store.status.path"
	// StoreStatusProfileTag — Template var: {{.Tag}}.
	StoreStatusProfileTag = "store.status.profile-tag"
	// StoreStatusMachineID — Template var: {{.ID}}.
	StoreStatusMachineID = "store.status.machine-id"
	// StoreStatusProfileDir — Template var: {{.Dir}}.
	StoreStatusProfileDir = "store.status.profile-dir"
	// StoreStatusStateDir — Template var: {{.Dir}}.
	StoreStatusStateDir = "store.status.state-dir"
	// StoreStatusHamsfiles — Template var: {{.Count}}.
	StoreStatusHamsfiles = "store.status.hamsfiles"
	// StoreStatusHamsfilesMissing — profile dir missing variant.
	StoreStatusHamsfilesMissing = "store.status.hamsfiles-missing"
	// StoreStatusGit — Template var: {{.Status}}.
	StoreStatusGit = "store.status.git"
	// StoreInitDryRunHeader — Template var: {{.Path}}.
	StoreInitDryRunHeader = "store.init.dry-run-header"
	// StoreInitDryRunProfileDir — Template var: {{.Path}}.
	StoreInitDryRunProfileDir = "store.init.dry-run-profile-dir"
	// StoreInitDryRunStateDir — Template var: {{.Path}}.
	StoreInitDryRunStateDir = "store.init.dry-run-state-dir"
	// StoreInitDryRunConfigFile — Template var: {{.Path}}.
	StoreInitDryRunConfigFile = "store.init.dry-run-config-file"
	// StoreInitDryRunGitignore — Template var: {{.Path}}.
	StoreInitDryRunGitignore = "store.init.dry-run-gitignore"
	// StoreInitDryRunPromptNotice — TTY notice.
	StoreInitDryRunPromptNotice = "store.init.dry-run-prompt-notice"
	// StoreInitDone — Template var: {{.Path}}.
	StoreInitDone = "store.init.done"
	// StoreInitDoneProfileDir — Template var: {{.Path}}.
	StoreInitDoneProfileDir = "store.init.done.profile-dir"
	// StoreInitDoneStateDir — Template var: {{.Path}}.
	StoreInitDoneStateDir = "store.init.done.state-dir"
	// StoreInitDoneGitignore — Template var: {{.Path}}.
	StoreInitDoneGitignore = "store.init.done.gitignore"
	// StorePullDryRun — Template var: {{.Path}}.
	StorePullDryRun = "store.pull.dry-run"
	// StoreCommitDryRun — Template vars: {{.Path}} {{.Msg}}.
	StoreCommitDryRun = "store.commit.dry-run"

	// ---- Config commands ----.

	// ConfigHomeLine — Template var: {{.Path}}.
	ConfigHomeLine = "config.line.config-home"
	// ConfigDataHomeLine — Template var: {{.Path}}.
	ConfigDataHomeLine = "config.line.data-home"
	// ConfigGlobalConfigLine — Template var: {{.Path}}.
	ConfigGlobalConfigLine = "config.line.global-config"
	// ConfigLocalOverridesLine — Template var: {{.Path}}.
	ConfigLocalOverridesLine = "config.line.local-overrides"
	// ConfigProfileTagLine — Template var: {{.Tag}}.
	ConfigProfileTagLine = "config.line.profile-tag"
	// ConfigMachineIDLine — Template var: {{.ID}}.
	ConfigMachineIDLine = "config.line.machine-id"
	// ConfigStorePathLine — Template var: {{.Path}}.
	ConfigStorePathLine = "config.line.store-path"
	// ConfigStoreRepoLine — Template var: {{.Repo}}.
	ConfigStoreRepoLine = "config.line.store-repo"
	// ConfigLLMCLILine — Template var: {{.Path}}.
	ConfigLLMCLILine = "config.line.llm-cli"
	// ConfigProviderPriorityLine — Template var: {{.List}}.
	ConfigProviderPriorityLine = "config.line.provider-priority"
	// ConfigSetDryRun — Template vars: {{.Key}} {{.Value}} {{.Target}}.
	ConfigSetDryRun = "config.set.dry-run"
	// ConfigSetDone — Template vars: {{.Key}} {{.Value}} {{.Target}}.
	ConfigSetDone = "config.set.done"
	// ConfigUnsetDryRun — Template vars: {{.Key}} {{.Target}}.
	ConfigUnsetDryRun = "config.unset.dry-run"
	// ConfigUnsetDone — Template vars: {{.Key}} {{.Target}}.
	ConfigUnsetDone = "config.unset.done"
	// ConfigOpenDryRun — Template vars: {{.Path}} {{.Editor}}.
	ConfigOpenDryRun = "config.open.dry-run"
	// ConfigOpenDryRunStub — "(file does not exist; would be created with a stub header)".
	ConfigOpenDryRunStub = "config.open.dry-run.stub"

	// ---- List commands ----.

	// ListNoResourcesFilter — no filter match.
	ListNoResourcesFilter = "list.no-resources-filter"
	// ListNoResourcesStatus — Template var: {{.Status}}.
	ListNoResourcesStatus = "list.no-resources-status"
	// ListNoResourcesEmpty — no managed resources.
	ListNoResourcesEmpty = "list.no-resources-empty"
	// ListNoResourcesEmptyLine1 — helper line.
	ListNoResourcesEmptyLine1 = "list.no-resources-empty.line1"
	// ListNoResourcesEmptyLine2 — helper line.
	ListNoResourcesEmptyLine2 = "list.no-resources-empty.line2"
	// ListGroupHeader — Template vars: {{.DisplayName}} {{.Count}} {{.Noun}}.
	ListGroupHeader = "list.group-header"

	// ---- Self-upgrade ----.

	// UpgradeBrewDryRun — brew upgrade dry-run notice.
	UpgradeBrewDryRun = "upgrade.brew.dry-run"
	// UpgradeBrewDetected — brew upgrade start.
	UpgradeBrewDetected = "upgrade.brew.detected"
	// UpgradeAlreadyUpToDate — Template var: {{.Version}}.
	UpgradeAlreadyUpToDate = "upgrade.already-up-to-date"
	// UpgradeDryRun — Template vars: {{.Asset}} {{.Current}} {{.Target}}.
	UpgradeDryRun = "upgrade.dry-run"
	// UpgradeDryRunAssetURL — Template var: {{.URL}}.
	UpgradeDryRunAssetURL = "upgrade.dry-run.asset-url"
	// UpgradeDownloading — Template vars: {{.Asset}} {{.Current}} {{.Target}}.
	UpgradeDownloading = "upgrade.downloading"
	// UpgradeSuccess — Template vars: {{.Current}} {{.Target}}.
	UpgradeSuccess = "upgrade.success"

	// ---- Version ----.

	// VersionInfo — Template var: {{.Info}}.
	VersionInfo = "version.info"

	// ---- Provider shared errors ----.

	// ProviderErrNoStore — no store directory configured.
	ProviderErrNoStore = "provider.err.no-store"
	// ProviderErrNoStoreSuggest — set store_path hint.
	ProviderErrNoStoreSuggest = "provider.err.no-store.suggest"

	// ProviderErrInstallNeedsPackage — Template var: {{.Provider}}.
	ProviderErrInstallNeedsPackage = "provider.err.install-needs-package"
	// ProviderErrInstallNeedsPackageUsage — Template var: {{.Provider}}.
	ProviderErrInstallNeedsPackageUsage = "provider.err.install-needs-package.usage"
	// ProviderErrInstallNeedsPackageBulk — Template var: {{.Provider}}.
	ProviderErrInstallNeedsPackageBulk = "provider.err.install-needs-package.bulk"
	// ProviderErrInstallNeedsPackageAtLeastOne — Template var: {{.Provider}}.
	ProviderErrInstallNeedsPackageAtLeastOne = "provider.err.install-needs-package.at-least-one"

	// ProviderErrRemoveNeedsPackage — Template var: {{.Provider}}.
	ProviderErrRemoveNeedsPackage = "provider.err.remove-needs-package"
	// ProviderErrRemoveNeedsPackageUsage — Template var: {{.Provider}}.
	ProviderErrRemoveNeedsPackageUsage = "provider.err.remove-needs-package.usage"
	// ProviderErrRemoveNeedsPackageAtLeastOne — Template var: {{.Provider}}.
	ProviderErrRemoveNeedsPackageAtLeastOne = "provider.err.remove-needs-package.at-least-one"

	// ProviderErrUnknownSubcommand — Template vars: {{.Provider}} {{.Sub}}.
	ProviderErrUnknownSubcommand = "provider.err.unknown-subcommand"

	// ---- Provider dry-run status lines ----.

	// ProviderStatusDryRunInstall — Template var: {{.Cmd}}.
	ProviderStatusDryRunInstall = "provider.status.dry-run-install"
	// ProviderStatusDryRunRemove — Template var: {{.Cmd}}.
	ProviderStatusDryRunRemove = "provider.status.dry-run-remove"
	// ProviderStatusDryRunRun — Template var: {{.Cmd}}.
	ProviderStatusDryRunRun = "provider.status.dry-run-run"

	// ---- Provider list headers ----.

	// ProviderListHomebrewHeader — list header.
	ProviderListHomebrewHeader = "provider.list.homebrew-header"
	// ProviderListGitCloneHeader — list header.
	ProviderListGitCloneHeader = "provider.list.git-clone-header"
	// ProviderListGitCloneEmpty — empty tracker hint.
	ProviderListGitCloneEmpty = "provider.list.git-clone-empty"

	// ---- git provider extras ----.

	// GitConfigSetDryRun — Template vars: {{.Key}} {{.Value}}.
	GitConfigSetDryRun = "git.config.set-dry-run"
	// GitConfigUnsetDryRun — Template var: {{.Key}}.
	GitConfigUnsetDryRun = "git.config.unset-dry-run"
	// GitCloneDryRun — Template vars: {{.Remote}} {{.Path}}.
	GitCloneDryRun = "git.clone.dry-run"
	// GitCloneRemoveDryRun — Template var: {{.ID}}.
	GitCloneRemoveDryRun = "git.clone.remove-dry-run"

	// ---- Sudo prompt ----.

	// SudoPrompt — "hams needs sudo for some operations.".
	SudoPrompt = "sudo.prompt"

	// ---- TUI fallback ----.

	// TUIWarnNoTTY — Template var: {{.Feature}}.
	TUIWarnNoTTY = "tui.warn-no-tty"

	// ---- Bootstrap (repo clone) ----.

	// BootstrapCloneDryRun — Template var: {{.Repo}}.
	BootstrapCloneDryRun = "bootstrap.clone-dry-run"
	// BootstrapDownloading — Template var: {{.Path}}.
	BootstrapDownloading = "bootstrap.downloading"
	// BootstrapDownloadSuccess — "Download Hams Store success".
	BootstrapDownloadSuccess = "bootstrap.download-success"
	// BootstrapProfileNow — Template var: {{.Path}}.
	BootstrapProfileNow = "bootstrap.profile-now"

	// ---- Errors rendering ----.

	// ErrorsPrefix — "Error: " prefix.
	ErrorsPrefix = "errors.prefix"
	// ErrorsSuggestionPrefix — "  suggestion: " prefix.
	ErrorsSuggestionPrefix = "errors.suggestion-prefix"

	// ---- Provider help ----.

	// ProviderHelpTitle — Template vars: {{.Name}} {{.Description}}.
	ProviderHelpTitle = "provider.help.title"
	// ProviderHelpUsage — "Usage:".
	ProviderHelpUsage = "provider.help.usage"
	// ProviderHelpUsageLine — Template var: {{.Name}}.
	ProviderHelpUsageLine = "provider.help.usage-line"
	// ProviderHelpDescribed — Template var: {{.DisplayName}}.
	ProviderHelpDescribed = "provider.help.described"
	// ProviderHelpHamsPrefix — Flags with --hams- prefix hint.
	ProviderHelpHamsPrefix = "provider.help.hams-prefix"
	// ProviderHelpDoubleDash — Use -- to force-forward hint.
	ProviderHelpDoubleDash = "provider.help.double-dash"
)
