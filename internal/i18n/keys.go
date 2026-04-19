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

	// ---- Builtin-provider shared templates ----.
	//
	// Most providers share the same "<verb> requires a <resource>"
	// error shape. Consolidating those into parameterised keys keeps
	// the catalog flat and translator-friendly.

	// ProviderErrRequiresResource — "{{.Provider}} {{.Verb}} requires a {{.Resource}}".
	ProviderErrRequiresResource = "provider.err.requires-resource"

	// ProviderErrRequiresAtLeastOne — "{{.Provider}} {{.Verb}} requires at least one {{.Resource}}".
	ProviderErrRequiresAtLeastOne = "provider.err.requires-at-least-one"

	// ProviderUsageBasic — "Usage: hams {{.Provider}} {{.Verb}} <{{.Placeholder}}>".
	ProviderUsageBasic = "provider.usage.basic"

	// ProviderDryRunWouldRun — "[dry-run] Would run: {{.Cmd}}".
	ProviderDryRunWouldRun = "provider.dry-run.would-run"

	// ProviderDryRunWouldInstall — "[dry-run] Would install: {{.Cmd}}".
	ProviderDryRunWouldInstall = "provider.dry-run.would-install"

	// ProviderDryRunWouldRemove — "[dry-run] Would remove: {{.Cmd}}".
	ProviderDryRunWouldRemove = "provider.dry-run.would-remove"

	// ---- Provider-specific outliers ----.

	// ProviderAptInstallSimulateWarning — apt-get simulate flag detected,
	// skipping auto-record. Not a hamserr.UserFacingError; surfaced via
	// slog.Warn which is in English, but the human-readable portion
	// is routed through i18n.Tf for the integration-test stderr grep.
	ProviderAptInstallSimulateWarning = "provider.apt.install.simulate-warning"

	// ProviderGitCloneRequiresRemote — dedicated key: "git clone
	// requires a remote URL" + the multi-line usage hint block.
	ProviderGitCloneRequiresRemote = "provider.git.clone.requires-remote"
	// ProviderGitCloneUsage — the multi-shape usage block that follows.
	ProviderGitCloneUsage1 = "provider.git.clone.usage1"
	ProviderGitCloneUsage2 = "provider.git.clone.usage2"
	ProviderGitCloneUsage3 = "provider.git.clone.usage3"
	ProviderGitCloneUsage4 = "provider.git.clone.usage4"

	// ProviderGitCloneFlagNotForwarded — rejects a real git flag
	// that isn't forwarded through auto-record yet. Template data:
	// {{.Flag}}.
	ProviderGitCloneFlagNotForwarded = "provider.git.clone.flag-not-forwarded"
	// ProviderGitCloneFileFollowup — "File a follow-up …" hint.
	ProviderGitCloneFileFollowup = "provider.git.clone.file-followup"
	// ProviderGitClonePlainFallback — "Or run plain git clone …" hint.
	ProviderGitClonePlainFallback = "provider.git.clone.plain-fallback"
	// ProviderGitCloneSinglePathExpected — exactly one local path
	// expected; template data: {{.Count}}, {{.Got}}.
	ProviderGitCloneSinglePathExpected = "provider.git.clone.single-path-expected"
	// ProviderGitCloneUsagePositional — "Usage: hams git clone <remote> <local-path>".
	ProviderGitCloneUsagePositional = "provider.git.clone.usage-positional"

	// ProviderGitRequiresSubcommand — "hams git requires a subcommand".
	ProviderGitRequiresSubcommand = "provider.git.requires-subcommand"
	// ProviderGitRecordedSubcommandsHeader — "Recorded subcommands:".
	ProviderGitRecordedSubcommandsHeader = "provider.git.recorded-subcommands-header"
	// ProviderGitPassthroughNote — "Any other subcommand … is passed through".
	ProviderGitPassthroughNote = "provider.git.passthrough-note"

	// ProviderGitConfigRequiresPath — git-config path-based usage error.
	ProviderGitConfigRequiresArgs = "provider.git.config.requires-args"

	// ---- Homebrew-specialised keys ----.

	// ProviderBrewExactOne — "brew {{.Verb}} takes exactly one repository (got {{.Count}} args: {{.Got}})".
	ProviderBrewExactOne = "provider.brew.exact-one"
	// ProviderBrewHintMulti — "To {{.Verb}} multiple repos, run the command once per repo".
	ProviderBrewHintMulti = "provider.brew.hint-multi"

	// ProviderBrewInstallCaskTagConflict — incompatible --cask + --hams-tag.
	// Template data: {{.Tag}}, {{.CaskTag}}.
	ProviderBrewInstallCaskTagConflict = "provider.brew.install.cask-tag-conflict"
	// ProviderBrewInstallCaskTagHint — remediation hint. Template: {{.CaskTag}}.
	ProviderBrewInstallCaskTagHint = "provider.brew.install.cask-tag-hint"

	// ProviderBrewInstallNoTapFormat — "brew install does not support tap-format args ({{.Arg}} looks like user/repo)".
	ProviderBrewInstallNoTapFormat = "provider.brew.install.no-tap-format"
	// ProviderBrewInstallNoTapHint1 — remediation hint. Template: {{.Arg}}.
	ProviderBrewInstallNoTapHint1 = "provider.brew.install.no-tap-hint1"
	// ProviderBrewInstallNoTapHint2 — second remediation hint.
	ProviderBrewInstallNoTapHint2 = "provider.brew.install.no-tap-hint2"

	// ProviderBrewInstallUsage — "Usage: hams brew install <package> [--cask] [--hams-tag=<tag>]".
	ProviderBrewInstallUsage = "provider.brew.install.usage"

	// ---- Bash-specialised keys ----.

	// ProviderBashPlannedV11 — "bash {{.Verb}} is planned for v1.1 …".
	ProviderBashPlannedV11   = "provider.bash.planned-v1-1"
	ProviderBashPlannedHint1 = "provider.bash.planned-hint1"
	ProviderBashPlannedHint2 = "provider.bash.planned-hint2"

	// ProviderBashRequiresSubcommand — "bash requires a subcommand".
	ProviderBashRequiresSubcommand = "provider.bash.requires-subcommand"
	ProviderBashUsageList          = "provider.bash.usage.list"
	ProviderBashUsageRun           = "provider.bash.usage.run"
	ProviderBashUsageRemove        = "provider.bash.usage.remove"

	// ---- Generic outliers ----.

	// ProviderErrExactOne — "{{.Provider}} {{.Verb}} takes exactly one {{.Resource}} (got {{.Count}} args: {{.Got}})".
	ProviderErrExactOne = "provider.err.exact-one"

	// ---- Duti ----.

	// ProviderDutiRequiresArgs — "duti requires arguments".
	ProviderDutiRequiresArgs = "provider.duti.requires-args"
	// ProviderDutiUsageSet — "Usage: hams duti <ext>=<bundle-id>".
	ProviderDutiUsageSet = "provider.duti.usage.set"
	// ProviderDutiUsageList — "       hams duti list".
	ProviderDutiUsageList = "provider.duti.usage.list"
	// ProviderDutiExample — "Example: hams duti pdf=com.adobe.acrobat.pdf".
	ProviderDutiExample = "provider.duti.example"
	// ProviderDutiInvalidResource — parseResourceID error wrapper.
	// Template: {{.Err}}.
	ProviderDutiInvalidResource = "provider.duti.invalid-resource"

	// ---- Defaults ----.

	// ProviderDefaultsRequiresArgs — "defaults {{.Verb}} requires ..."
	// Template: {{.Verb}}, {{.Resource}}.
	ProviderDefaultsRequiresArgs = "provider.defaults.requires-args"
	ProviderDefaultsUsageWrite   = "provider.defaults.usage.write"
	ProviderDefaultsUsageDelete  = "provider.defaults.usage.delete"
	ProviderDefaultsUsageList    = "provider.defaults.usage.list"

	// ---- Ansible ----.

	// ProviderAnsibleUsage — "Usage: hams ansible <playbook-path>".
	ProviderAnsibleRequiresPlaybook = "provider.ansible.requires-playbook"
	ProviderAnsibleUsagePlaybook    = "provider.ansible.usage.playbook"
	ProviderAnsibleExample          = "provider.ansible.example"
	ProviderAnsibleNotFound         = "provider.ansible.not-found"

	// ProviderDefaultsWriteArgsMismatch — specific "write requires exactly 4 args" err.
	// Template: {{.Count}}.
	ProviderDefaultsWriteArgsMismatch  = "provider.defaults.write.args-mismatch"
	ProviderDefaultsWriteHintQuote     = "provider.defaults.write.hint-quote"
	ProviderDefaultsDeleteArgsMismatch = "provider.defaults.delete.args-mismatch"
	ProviderDefaultsDeleteHintRepeat   = "provider.defaults.delete.hint-repeat"
	ProviderDefaultsRootRequiresArgs   = "provider.defaults.root-requires-args"
	ProviderDefaultsUsageWriteExample  = "provider.defaults.usage.write-example"
	ProviderDefaultsUsageDeleteExample = "provider.defaults.usage.delete-example"

	// ProviderAnsibleRequiresPath — "ansible requires a playbook path".
	ProviderAnsibleNoBinary      = "provider.ansible.no-binary"
	ProviderAnsibleNoBinaryHint1 = "provider.ansible.no-binary-hint1"
	ProviderAnsibleNoBinaryHint2 = "provider.ansible.no-binary-hint2"

	// Generic "planned for v1.1" block used by providers with URN-based
	// resources whose CLI editing is deferred (bash, ansible).
	ProviderVerbPlannedV11   = "provider.verb.planned-v1-1"
	ProviderVerbPlannedHint1 = "provider.verb.planned-hint1"
	ProviderVerbPlannedHint2 = "provider.verb.planned-hint2"

	// Ansible root usage.
	ProviderAnsibleRequiresPlaybookOrSubcommand = "provider.ansible.requires-playbook-or-subcommand"
	ProviderAnsibleUsageList                    = "provider.ansible.usage.list"
	ProviderAnsibleUsageAdhoc                   = "provider.ansible.usage.adhoc"
	ProviderAnsibleUsageRun                     = "provider.ansible.usage.run"
	ProviderAnsibleUsageRemove                  = "provider.ansible.usage.remove"

	// ProviderNoStoreConfigured — shared "no store directory configured" error.
	// Fired by every provider's hamsfilePath helper when --store isn't set
	// and cfg.StorePath is empty. Hint is the remediation line below.
	ProviderNoStoreConfigured     = "provider.no-store-configured"
	ProviderNoStoreConfiguredHint = "provider.no-store-configured.hint"

	// ---- Git config internals (follow-up 2.14.a) ----.

	ProviderGitConfigSetRequiresKV          = "provider.git.config.set.requires-kv"
	ProviderGitConfigUsageSet               = "provider.git.config.usage.set"
	ProviderGitConfigUsageBare              = "provider.git.config.usage.bare"
	ProviderGitConfigUsageRemove            = "provider.git.config.usage.remove"
	ProviderGitConfigUsageList              = "provider.git.config.usage.list"
	ProviderGitConfigExampleSet             = "provider.git.config.example.set"
	ProviderGitConfigExampleRemove          = "provider.git.config.example.remove"
	ProviderGitConfigRemoveRequiresKey      = "provider.git.config.remove.requires-key"
	ProviderGitConfigRequiresSubcommandOrKV = "provider.git.config.requires-subcommand-or-kv"
	ProviderGitConfigDryRunSet              = "provider.git.config.dry-run.set"
	ProviderGitConfigDryRunUnset            = "provider.git.config.dry-run.unset"

	// ---- Git clone internals (follow-up 2.14.a) ----.

	ProviderGitCloneSubcommandRequired = "provider.git.clone.subcommand-required"
	ProviderGitCloneUsageAddSub        = "provider.git.clone.usage.add-sub"
	ProviderGitCloneUsageRemoveSub     = "provider.git.clone.usage.remove-sub"
	ProviderGitCloneUsageListSub       = "provider.git.clone.usage.list-sub"
	ProviderGitCloneAddRequiresRemote  = "provider.git.clone.add.requires-remote"
	ProviderGitCloneAddUsage           = "provider.git.clone.add.usage"
	ProviderGitCloneAddExactOne        = "provider.git.clone.add.exact-one"
	ProviderGitCloneAddPosHint         = "provider.git.clone.add.positional-hint"
	ProviderGitCloneAddRequiresPath    = "provider.git.clone.add.requires-path"
	ProviderGitCloneTargetNotRepo      = "provider.git.clone.target-not-repo"
	ProviderGitCloneTargetNotRepoHint1 = "provider.git.clone.target-not-repo.hint1"
	ProviderGitCloneTargetNotRepoHint2 = "provider.git.clone.target-not-repo.hint2"
	ProviderGitCloneDryRunAdd          = "provider.git.clone.dry-run.add"
	ProviderGitCloneDryRunRemoveEntry  = "provider.git.clone.dry-run.remove-entry"
	ProviderGitCloneRemoveRequiresURN  = "provider.git.clone.remove.requires-urn"
	ProviderGitCloneRemoveUsage        = "provider.git.clone.remove.usage"
	ProviderGitCloneNoEntry            = "provider.git.clone.no-entry"
	ProviderGitCloneInvalidResourceID  = "provider.git.clone.invalid-resource-id"

	// ProviderHomebrewListHeader — header printed before the diff in
	// `hams brew list`. Kept as a separate key (not shared with a
	// generic "<Provider> managed packages:" template) because
	// "Homebrew" is a brand name the user identifies by.
	ProviderHomebrewListHeader = "provider.homebrew.list.header"

	// ============================================================
	// Imported from origin/dev in 2026-04-19-i18n-and-log-assertion-
	// expansion. These keys cover the CLI lifecycle (autoinit, ufe,
	// store, config, list, upgrade, sudo, TUI, expanded apply/refresh,
	// git-dispatcher, errors prefix, provider help) that loop's pre-
	// expansion catalog left as hardcoded English. Translations are
	// taken verbatim from origin/dev's locales/*.yaml so the wording
	// matches dev's UX exactly.
	// ============================================================.

	// ---- App metadata ----.

	// AppTitle — top-of-help title string.
	AppTitle = "app.title"

	// ---- Auto-init status lines ----.

	// AutoInitGlobalConfigCreated — Template vars: {{.Path}} {{.Tag}} {{.MachineID}}.
	AutoInitGlobalConfigCreated = "autoinit.global_config_created"
	// AutoInitStoreCreated — Template var: {{.Path}}.
	AutoInitStoreCreated = "autoinit.store_created"
	// AutoInitDryRunGlobalConfig — Template var: {{.Path}}.
	AutoInitDryRunGlobalConfig = "autoinit.dry_run.global_config"
	// AutoInitDryRunStore — Template var: {{.Path}}.
	AutoInitDryRunStore = "autoinit.dry_run.store"

	// ---- No-store-configured UFE family ----.

	UFENoStoreConfigured              = "ufe.no_store_configured"
	UFENoStoreConfiguredSuggestClone  = "ufe.no_store_configured.suggest_clone"
	UFENoStoreConfiguredSuggestSet    = "ufe.no_store_configured.suggest_set"
	UFENoStoreConfiguredSuggestInit   = "ufe.no_store_configured.suggest_init"
	UFENoStoreConfiguredOptOut        = "ufe.no_store_configured.opt_out"
	UFENoStoreConfiguredOptOutSuggest = "ufe.no_store_configured.opt_out_suggest"

	// ---- Apply status (dev-style hyphenated keys, distinct from
	// loop's pre-existing apply.status.* keys above). ----.

	ApplyDryRunHeader     = "apply.dry_run_header"
	ApplyNoProvidersMatch = "apply.no_providers_match"

	// ---- Refresh status (dev-style sibling of apply.no_providers_match) ----.

	RefreshNoProvidersMatch = "refresh.no_providers_match"

	// ---- hams git dispatcher (usage + errors) ----.

	GitUsageHeader             = "git.usage.header"
	GitUsageSuggestMain        = "git.usage.suggest_main"
	GitUsageSuggestSubcommands = "git.usage.suggest_subcommands"
	GitUsageExampleConfig      = "git.usage.example_config"
	GitUsageExampleClone       = "git.usage.example_clone"
	GitUnknownSubcommand       = "git.unknown_subcommand"

	// ---- CLI framework errors (broader hierarchy ported from dev) ----.

	CLIErrNoPositionalArgs              = "cli.err.no-positional-args"
	CLIErrNoPositionalArgsSuggestFilter = "cli.err.no-positional-args.suggest-filter"
	CLIErrNoPositionalArgsSuggestAll    = "cli.err.no-positional-args.suggest-all"

	CLIErrOnlyExceptConflict        = "cli.err.only-except-conflict"
	CLIErrOnlyExceptConflictSuggest = "cli.err.only-except-conflict.suggest"

	CLIErrBootstrapModeConflict        = "cli.err.bootstrap-mode-conflict"
	CLIErrBootstrapModeConflictSuggest = "cli.err.bootstrap-mode-conflict.suggest"

	CLIErrFromRepoStoreConflict = "cli.err.from-repo-store-conflict"

	CLIErrStorePathInvalid           = "cli.err.store-path-invalid"
	CLIErrStorePathInvalidSuggestFix = "cli.err.store-path-invalid.suggest-fix"

	CLIErrProfileNotFound              = "cli.err.profile-not-found"
	CLIErrProfileNotFoundSuggestList   = "cli.err.profile-not-found.suggest-list"
	CLIErrProfileNotFoundSuggestCreate = "cli.err.profile-not-found.suggest-create"

	CLIErrConfigFileMissing               = "cli.err.config-file-missing"
	CLIErrConfigFileMissingSuggestCheck   = "cli.err.config-file-missing.suggest-check"
	CLIErrConfigFileMissingSuggestCreate  = "cli.err.config-file-missing.suggest-create"
	CLIErrConfigFileMissingSuggestDefault = "cli.err.config-file-missing.suggest-default"

	CLIErrOnlyEmpty   = "cli.err.only-empty"
	CLIErrExceptEmpty = "cli.err.except-empty"

	CLIErrUnknownProvider            = "cli.err.unknown-provider"
	CLIErrUnknownProviderSuggestList = "cli.err.unknown-provider.suggest-list"
	CLIErrProfileNotConfigured       = "cli.err.profile-not-configured"

	CLIErrSudoAcquisitionFailed                    = "cli.err.sudo-acquisition-failed"
	CLIErrSudoAcquisitionFailedSuggestReenter      = "cli.err.sudo-acquisition-failed.suggest-reenter"
	CLIErrSudoAcquisitionFailedSuggestPasswordless = "cli.err.sudo-acquisition-failed.suggest-passwordless" //nolint:gosec // G101: message-key constant
	CLIErrSudoAcquisitionFailedSuggestFilter       = "cli.err.sudo-acquisition-failed.suggest-filter"

	CLIErrInterrupted               = "cli.err.interrupted"
	CLIErrInterruptedSuggestInspect = "cli.err.interrupted.suggest-inspect"
	CLIErrInterruptedSuggestRerun   = "cli.err.interrupted.suggest-rerun"

	// ---- Apply extra status lines (dev-style) ----.

	ApplyExecutionOrderHeader               = "apply.execution-order-header"
	ApplyBootstrapDryRun                    = "apply.bootstrap-dry-run"
	ApplyNoProvidersStateOnly               = "apply.no-providers-state-only"
	ApplyNoProvidersFiltered                = "apply.no-providers-filtered"
	ApplyNoProvidersOnlyMissing             = "apply.no-providers-only-missing"
	ApplyNoProvidersProfileLine             = "apply.no-providers-profile-line"
	ApplyNoProvidersSuggestInstall          = "apply.no-providers-suggest-install"
	ApplyNoProvidersProfileDirMissing       = "apply.no-providers-profile-dir-missing"
	ApplyNoProvidersAvailableProfiles       = "apply.no-providers-available-profiles"
	ApplyNoProvidersSuggestProfileFix       = "apply.no-providers-suggest-profile-fix"
	ApplySummaryComplete                    = "apply.summary-complete"
	ApplySummaryFailedWarning               = "apply.summary-failed-warning"
	ApplySummaryFailedWarningSuggest        = "apply.summary-failed-warning.suggest"
	ApplySummarySkippedWarning              = "apply.summary-skipped-warning"
	ApplySummaryStateFailWarning            = "apply.summary-state-fail-warning"
	ApplySummaryStateFailSuggest            = "apply.summary-state-fail-warning.suggest"
	ApplySummaryPartialFailureErr           = "apply.summary-partial-failure-err"
	ApplySummaryPartialFailureSuggestRetry  = "apply.summary-partial-failure-err.suggest-retry"
	ApplySummaryPartialFailureSuggestDebug  = "apply.summary-partial-failure-err.suggest-debug"
	ApplyDryRunNoChanges                    = "apply.dry-run-no-changes"
	ApplyDryRunPerProviderHeader            = "apply.dry-run-per-provider-header"
	ApplyDryRunNoChangesProvider            = "apply.dry-run-no-changes-provider"
	ApplyDryRunUnchangedTail                = "apply.dry-run-unchanged-tail"
	ApplyDryRunStateFailErr                 = "apply.dry-run-state-fail-err"
	ApplyDryRunStateFailErrSuggestPerms     = "apply.dry-run-state-fail-err.suggest-perms"
	ApplyDryRunStateFailErrSuggestNoRefresh = "apply.dry-run-state-fail-err.suggest-no-refresh"
	ApplyDryRunSkippedErr                   = "apply.dry-run-skipped-err"
	ApplyDryRunSkippedErrSuggestFix         = "apply.dry-run-skipped-err.suggest-fix"
	ApplyStateFailDriftLine                 = "apply.state-fail-drift-line"
	ApplyNotFoundProfile                    = "apply.not-found-profile"
	ApplyProfileTagPrompt                   = "apply.profile-tag-prompt"
	ApplyProfileMachineIDPrompt             = "apply.profile-machine-id-prompt"

	// ---- Refresh extra status lines ----.

	RefreshSummaryComplete               = "refresh.summary-complete"
	RefreshSummaryCompleteWithSaveFails  = "refresh.summary-complete-with-save-fails"
	RefreshSummaryCompleteWithProbeFails = "refresh.summary-complete-with-probe-fails"
	RefreshSummarySaveFailWarning        = "refresh.summary-save-fail-warning"
	RefreshInterrupted                   = "refresh.interrupted"
	RefreshStateWrittenHeader            = "refresh.state-written-header"

	// ---- Store commands ----.

	StoreNothingToCommit        = "store.nothing-to-commit"
	StoreStatusPathMissing      = "store.status.path-missing"
	StoreStatusPathMissingLine1 = "store.status.path-missing.line1"
	StoreStatusPathMissingLine2 = "store.status.path-missing.line2"
	StoreStatusPath             = "store.status.path"
	StoreStatusProfileTag       = "store.status.profile-tag"
	StoreStatusMachineID        = "store.status.machine-id"
	StoreStatusProfileDir       = "store.status.profile-dir"
	StoreStatusStateDir         = "store.status.state-dir"
	StoreStatusHamsfiles        = "store.status.hamsfiles"
	StoreStatusHamsfilesMissing = "store.status.hamsfiles-missing"
	StoreStatusGit              = "store.status.git"
	StoreInitDryRunHeader       = "store.init.dry-run-header"
	StoreInitDryRunProfileDir   = "store.init.dry-run-profile-dir"
	StoreInitDryRunStateDir     = "store.init.dry-run-state-dir"
	StoreInitDryRunConfigFile   = "store.init.dry-run-config-file"
	StoreInitDryRunGitignore    = "store.init.dry-run-gitignore"
	StoreInitDryRunPromptNotice = "store.init.dry-run-prompt-notice"
	StoreInitDone               = "store.init.done"
	StoreInitDoneProfileDir     = "store.init.done.profile-dir"
	StoreInitDoneStateDir       = "store.init.done.state-dir"
	StoreInitDoneGitignore      = "store.init.done.gitignore"
	StorePullDryRun             = "store.pull.dry-run"
	StoreCommitDryRun           = "store.commit.dry-run"

	// ---- Config commands ----.

	ConfigHomeLine             = "config.line.config-home"
	ConfigDataHomeLine         = "config.line.data-home"
	ConfigGlobalConfigLine     = "config.line.global-config"
	ConfigLocalOverridesLine   = "config.line.local-overrides"
	ConfigProfileTagLine       = "config.line.profile-tag"
	ConfigMachineIDLine        = "config.line.machine-id"
	ConfigStorePathLine        = "config.line.store-path"
	ConfigStoreRepoLine        = "config.line.store-repo"
	ConfigLLMCLILine           = "config.line.llm-cli"
	ConfigProviderPriorityLine = "config.line.provider-priority"
	ConfigSetDryRun            = "config.set.dry-run"
	ConfigSetDone              = "config.set.done"
	ConfigUnsetDryRun          = "config.unset.dry-run"
	ConfigUnsetDone            = "config.unset.done"
	ConfigOpenDryRun           = "config.open.dry-run"
	ConfigOpenDryRunStub       = "config.open.dry-run.stub"

	// ---- List commands ----.

	ListNoResourcesFilter     = "list.no-resources-filter"
	ListNoResourcesStatus     = "list.no-resources-status"
	ListNoResourcesEmpty      = "list.no-resources-empty"
	ListNoResourcesEmptyLine1 = "list.no-resources-empty.line1"
	ListNoResourcesEmptyLine2 = "list.no-resources-empty.line2"
	ListGroupHeader           = "list.group-header"

	// ---- Self-upgrade ----.

	UpgradeBrewDryRun      = "upgrade.brew.dry-run"
	UpgradeBrewDetected    = "upgrade.brew.detected"
	UpgradeAlreadyUpToDate = "upgrade.already-up-to-date"
	UpgradeDryRun          = "upgrade.dry-run"
	UpgradeDryRunAssetURL  = "upgrade.dry-run.asset-url"
	UpgradeDownloading     = "upgrade.downloading"
	UpgradeSuccess         = "upgrade.success"

	// ---- Version ----.

	VersionInfo = "version.info"

	// ---- Provider shared errors (dev-style hyphenated keys) ----.

	ProviderErrNoStore                       = "provider.err.no-store"
	ProviderErrNoStoreSuggest                = "provider.err.no-store.suggest"
	ProviderErrInstallNeedsPackage           = "provider.err.install-needs-package"
	ProviderErrInstallNeedsPackageUsage      = "provider.err.install-needs-package.usage"
	ProviderErrInstallNeedsPackageBulk       = "provider.err.install-needs-package.bulk"
	ProviderErrInstallNeedsPackageAtLeastOne = "provider.err.install-needs-package.at-least-one"
	ProviderErrRemoveNeedsPackage            = "provider.err.remove-needs-package"
	ProviderErrRemoveNeedsPackageUsage       = "provider.err.remove-needs-package.usage"
	ProviderErrRemoveNeedsPackageAtLeastOne  = "provider.err.remove-needs-package.at-least-one"
	ProviderErrUnknownSubcommand             = "provider.err.unknown-subcommand"

	// ---- Provider dry-run status lines ----.

	ProviderStatusDryRunInstall = "provider.status.dry-run-install"
	ProviderStatusDryRunRemove  = "provider.status.dry-run-remove"
	ProviderStatusDryRunRun     = "provider.status.dry-run-run"

	// ---- Provider list headers (dev-style) ----.

	ProviderListHomebrewHeader = "provider.list.homebrew-header"
	ProviderListGitCloneHeader = "provider.list.git-clone-header"
	ProviderListGitCloneEmpty  = "provider.list.git-clone-empty"

	// ---- git provider extras (dev-style) ----.

	GitConfigSetDryRun   = "git.config.set-dry-run"
	GitConfigUnsetDryRun = "git.config.unset-dry-run"
	GitCloneDryRun       = "git.clone.dry-run"
	GitCloneRemoveDryRun = "git.clone.remove-dry-run"

	// ---- Sudo prompt ----.

	SudoPrompt = "sudo.prompt"

	// ---- TUI fallback ----.

	TUIWarnNoTTY = "tui.warn-no-tty"

	// ---- Bootstrap (repo clone) ----.

	BootstrapCloneDryRun     = "bootstrap.clone-dry-run"
	BootstrapDownloading     = "bootstrap.downloading"
	BootstrapDownloadSuccess = "bootstrap.download-success"
	BootstrapProfileNow      = "bootstrap.profile-now"

	// ---- Errors rendering ----.

	ErrorsPrefix           = "errors.prefix"
	ErrorsSuggestionPrefix = "errors.suggestion-prefix"

	// ---- Provider help ----.

	ProviderHelpTitle      = "provider.help.title"
	ProviderHelpUsage      = "provider.help.usage"
	ProviderHelpUsageLine  = "provider.help.usage-line"
	ProviderHelpDescribed  = "provider.help.described"
	ProviderHelpHamsPrefix = "provider.help.hams-prefix"
	ProviderHelpDoubleDash = "provider.help.double-dash"
)
