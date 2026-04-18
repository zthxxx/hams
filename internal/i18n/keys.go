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
)
