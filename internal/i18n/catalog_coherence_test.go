package i18n_test

import (
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/i18n"
)

// TestCatalogCoherence_EveryTypedKeyResolves asserts that every
// exported message-key constant declared in `keys.go` resolves to a
// non-key, non-empty string in BOTH locale files.
//
// Coexists with TestLocalesAreInParity (locale_parity_test.go):
//
//   - parity_test.go enforces "every YAML key has a translation in
//     every locale" — a dynamic check that catches orphaned YAML
//     entries.
//   - this test enforces "every typed Go constant has a YAML entry"
//     — a static check that catches keys declared in keys.go but
//     forgotten from the YAML files.
//
// Together the two tests give bidirectional coverage. Adding a new
// constant without adding its YAML entry fails this test; adding a
// YAML entry without a constant fails parity. Either failure surfaces
// at CI before the regression lands in the binary.
//
// Imported from origin/dev's i18n_test.go in
// 2026-04-19-i18n-and-log-assertion-expansion.
func TestCatalogCoherence_EveryTypedKeyResolves(t *testing.T) {
	t.Parallel()
	// Hand-maintained list of every exported constant in keys.go.
	// New constants MUST be appended here. The list is intentionally
	// hand-maintained (rather than reflected) so the diff at PR-time
	// shows the intent: "I am adding a translatable key".
	typedKeys := []string{
		// loop-original CLI errors and apply/store status.
		i18n.CLIErrTagProfileConflict,
		i18n.CLIErrNoStore,
		i18n.CLIErrBootstrapConflict,
		i18n.CLIErrFromRepoVsStore,
		i18n.CLIErrOnlyExceptExclusive,
		i18n.ApplyStatusDryRunPreview,
		i18n.ApplyStatusNoProvidersMatch,
		i18n.ApplyStatusSessionStarted,
		i18n.BootstrapStatusProfileMissing,
		i18n.BootstrapStatusAutoInitialized,
		i18n.BootstrapErrNotConfigured,
		i18n.StoreStatusLabelPath,
		i18n.StoreStatusLabelProfile,
		i18n.StoreStatusLabelMachine,
		i18n.StoreStatusLabelProfileDir,
		i18n.StoreStatusLabelStateDir,
		i18n.StoreStatusLabelHamsfiles,
		i18n.StoreStatusLabelGit,
		// Dev-imported app metadata.
		i18n.AppTitle,
		// Dev-imported autoinit lifecycle.
		i18n.AutoInitGlobalConfigCreated,
		i18n.AutoInitStoreCreated,
		i18n.AutoInitDryRunGlobalConfig,
		i18n.AutoInitDryRunStore,
		// Dev-imported UFE family.
		i18n.UFENoStoreConfigured,
		i18n.UFENoStoreConfiguredSuggestClone,
		i18n.UFENoStoreConfiguredSuggestSet,
		i18n.UFENoStoreConfiguredSuggestInit,
		i18n.UFENoStoreConfiguredOptOut,
		i18n.UFENoStoreConfiguredOptOutSuggest,
		// Dev-imported apply extras.
		i18n.ApplyDryRunHeader,
		i18n.ApplyNoProvidersMatch,
		i18n.ApplyExecutionOrderHeader,
		i18n.ApplyBootstrapDryRun,
		i18n.ApplyNoProvidersStateOnly,
		i18n.ApplyNoProvidersFiltered,
		i18n.ApplyNoProvidersOnlyMissing,
		i18n.ApplyNoProvidersProfileLine,
		i18n.ApplyNoProvidersSuggestInstall,
		i18n.ApplyNoProvidersProfileDirMissing,
		i18n.ApplyNoProvidersAvailableProfiles,
		i18n.ApplyNoProvidersSuggestProfileFix,
		i18n.ApplySummaryComplete,
		i18n.ApplySummaryFailedWarning,
		i18n.ApplySummaryFailedWarningSuggest,
		i18n.ApplySummarySkippedWarning,
		i18n.ApplySummaryStateFailWarning,
		i18n.ApplySummaryStateFailSuggest,
		i18n.ApplySummaryPartialFailureErr,
		i18n.ApplySummaryPartialFailureSuggestRetry,
		i18n.ApplySummaryPartialFailureSuggestDebug,
		i18n.ApplyDryRunNoChanges,
		i18n.ApplyDryRunPerProviderHeader,
		i18n.ApplyDryRunNoChangesProvider,
		i18n.ApplyDryRunUnchangedTail,
		i18n.ApplyDryRunStateFailErr,
		i18n.ApplyDryRunStateFailErrSuggestPerms,
		i18n.ApplyDryRunStateFailErrSuggestNoRefresh,
		i18n.ApplyDryRunSkippedErr,
		i18n.ApplyDryRunSkippedErrSuggestFix,
		i18n.ApplyStateFailDriftLine,
		i18n.ApplyNotFoundProfile,
		i18n.ApplyProfileTagPrompt,
		i18n.ApplyProfileMachineIDPrompt,
		// Dev-imported refresh extras.
		i18n.RefreshNoProvidersMatch,
		i18n.RefreshSummaryComplete,
		i18n.RefreshSummaryCompleteWithSaveFails,
		i18n.RefreshSummaryCompleteWithProbeFails,
		i18n.RefreshSummarySaveFailWarning,
		i18n.RefreshInterrupted,
		i18n.RefreshStateWrittenHeader,
		// Dev-imported store commands.
		i18n.StoreNothingToCommit,
		i18n.StoreStatusPathMissing,
		i18n.StoreStatusPathMissingLine1,
		i18n.StoreStatusPathMissingLine2,
		i18n.StoreStatusPath,
		i18n.StoreStatusProfileTag,
		i18n.StoreStatusMachineID,
		i18n.StoreStatusProfileDir,
		i18n.StoreStatusStateDir,
		i18n.StoreStatusHamsfiles,
		i18n.StoreStatusHamsfilesMissing,
		i18n.StoreStatusGit,
		i18n.StoreInitDryRunHeader,
		i18n.StoreInitDryRunProfileDir,
		i18n.StoreInitDryRunStateDir,
		i18n.StoreInitDryRunConfigFile,
		i18n.StoreInitDryRunGitignore,
		i18n.StoreInitDryRunPromptNotice,
		i18n.StoreInitDone,
		i18n.StoreInitDoneProfileDir,
		i18n.StoreInitDoneStateDir,
		i18n.StoreInitDoneGitignore,
		i18n.StorePullDryRun,
		i18n.StoreCommitDryRun,
		// Dev-imported config commands.
		i18n.ConfigHomeLine,
		i18n.ConfigDataHomeLine,
		i18n.ConfigGlobalConfigLine,
		i18n.ConfigLocalOverridesLine,
		i18n.ConfigProfileTagLine,
		i18n.ConfigMachineIDLine,
		i18n.ConfigStorePathLine,
		i18n.ConfigStoreRepoLine,
		i18n.ConfigLLMCLILine,
		i18n.ConfigProviderPriorityLine,
		i18n.ConfigSetDryRun,
		i18n.ConfigSetDone,
		i18n.ConfigUnsetDryRun,
		i18n.ConfigUnsetDone,
		i18n.ConfigOpenDryRun,
		i18n.ConfigOpenDryRunStub,
		// Dev-imported list commands.
		i18n.ListNoResourcesFilter,
		i18n.ListNoResourcesStatus,
		i18n.ListNoResourcesEmpty,
		i18n.ListNoResourcesEmptyLine1,
		i18n.ListNoResourcesEmptyLine2,
		i18n.ListGroupHeader,
		// Dev-imported self-upgrade.
		i18n.UpgradeBrewDryRun,
		i18n.UpgradeBrewDetected,
		i18n.UpgradeAlreadyUpToDate,
		i18n.UpgradeDryRun,
		i18n.UpgradeDryRunAssetURL,
		i18n.UpgradeDownloading,
		i18n.UpgradeSuccess,
		// Dev-imported version.
		i18n.VersionInfo,
		// Dev-imported provider shared errors.
		i18n.ProviderErrNoStore,
		i18n.ProviderErrNoStoreSuggest,
		i18n.ProviderErrInstallNeedsPackage,
		i18n.ProviderErrInstallNeedsPackageUsage,
		i18n.ProviderErrInstallNeedsPackageBulk,
		i18n.ProviderErrInstallNeedsPackageAtLeastOne,
		i18n.ProviderErrRemoveNeedsPackage,
		i18n.ProviderErrRemoveNeedsPackageUsage,
		i18n.ProviderErrRemoveNeedsPackageAtLeastOne,
		i18n.ProviderErrUnknownSubcommand,
		// Dev-imported provider dry-run status.
		i18n.ProviderStatusDryRunInstall,
		i18n.ProviderStatusDryRunRemove,
		i18n.ProviderStatusDryRunRun,
		// Dev-imported provider list headers.
		i18n.ProviderListHomebrewHeader,
		i18n.ProviderListGitCloneHeader,
		i18n.ProviderListGitCloneEmpty,
		// Dev-imported git provider extras.
		i18n.GitConfigSetDryRun,
		i18n.GitConfigUnsetDryRun,
		i18n.GitCloneDryRun,
		i18n.GitCloneRemoveDryRun,
		// Dev-imported sudo prompt.
		i18n.SudoPrompt,
		// Dev-imported TUI fallback.
		i18n.TUIWarnNoTTY,
		// Dev-imported bootstrap (repo clone).
		i18n.BootstrapCloneDryRun,
		i18n.BootstrapDownloading,
		i18n.BootstrapDownloadSuccess,
		i18n.BootstrapProfileNow,
		// Dev-imported errors rendering.
		i18n.ErrorsPrefix,
		i18n.ErrorsSuggestionPrefix,
		// Dev-imported provider help.
		i18n.ProviderHelpTitle,
		i18n.ProviderHelpUsage,
		i18n.ProviderHelpUsageLine,
		i18n.ProviderHelpDescribed,
		i18n.ProviderHelpHamsPrefix,
		i18n.ProviderHelpDoubleDash,
		// Dev-imported git dispatcher (usage + errors).
		i18n.GitUsageHeader,
		i18n.GitUsageSuggestMain,
		i18n.GitUsageSuggestSubcommands,
		i18n.GitUsageExampleConfig,
		i18n.GitUsageExampleClone,
		i18n.GitUnknownSubcommand,
		// Dev-imported CLI extra errors.
		i18n.CLIErrNoPositionalArgs,
		i18n.CLIErrNoPositionalArgsSuggestFilter,
		i18n.CLIErrNoPositionalArgsSuggestAll,
		i18n.CLIErrOnlyExceptConflict,
		i18n.CLIErrOnlyExceptConflictSuggest,
		i18n.CLIErrBootstrapModeConflict,
		i18n.CLIErrBootstrapModeConflictSuggest,
		i18n.CLIErrFromRepoStoreConflict,
		i18n.CLIErrStorePathInvalid,
		i18n.CLIErrStorePathInvalidSuggestFix,
		i18n.CLIErrProfileNotFound,
		i18n.CLIErrProfileNotFoundSuggestList,
		i18n.CLIErrProfileNotFoundSuggestCreate,
		i18n.CLIErrConfigFileMissing,
		i18n.CLIErrConfigFileMissingSuggestCheck,
		i18n.CLIErrConfigFileMissingSuggestCreate,
		i18n.CLIErrConfigFileMissingSuggestDefault,
		i18n.CLIErrOnlyEmpty,
		i18n.CLIErrExceptEmpty,
		i18n.CLIErrUnknownProvider,
		i18n.CLIErrUnknownProviderSuggestList,
		i18n.CLIErrProfileNotConfigured,
		i18n.CLIErrSudoAcquisitionFailed,
		i18n.CLIErrSudoAcquisitionFailedSuggestReenter,
		i18n.CLIErrSudoAcquisitionFailedSuggestPasswordless,
		i18n.CLIErrSudoAcquisitionFailedSuggestFilter,
		i18n.CLIErrInterrupted,
		i18n.CLIErrInterruptedSuggestInspect,
		i18n.CLIErrInterruptedSuggestRerun,
	}

	for _, locFile := range []string{"locales/en.yaml", "locales/zh-CN.yaml"} {
		data, err := testLocaleFS.ReadFile(locFile)
		if err != nil {
			t.Fatalf("read %s: %v", locFile, err)
		}
		contents := string(data)
		for _, k := range typedKeys {
			marker := "id: " + k + "\n"
			if !strings.Contains(contents, marker) {
				t.Errorf("%s: missing translation for key %q", locFile, k)
			}
		}
	}
}
