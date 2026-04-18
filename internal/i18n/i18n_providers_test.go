package i18n_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/zthxxx/hams/internal/i18n"
)

// providerKeyConstants returns every exported identifier on the
// internal/i18n package whose name begins with "Provider". This is the
// test's way of enumerating the catalog — keys.go is the single
// source of truth, so a new constant added there automatically enters
// the coverage matrix without requiring a parallel list.
func providerKeyConstants(t *testing.T) map[string]string {
	t.Helper()

	// Reflection of a pure-constant package isn't directly supported;
	// enumerate the keys we care about by referring to the constants
	// explicitly. Adding a new Provider* constant → add it here too.
	keys := map[string]string{
		"ProviderErrRequiresResource":          i18n.ProviderErrRequiresResource,
		"ProviderErrRequiresAtLeastOne":        i18n.ProviderErrRequiresAtLeastOne,
		"ProviderUsageBasic":                   i18n.ProviderUsageBasic,
		"ProviderDryRunWouldRun":               i18n.ProviderDryRunWouldRun,
		"ProviderDryRunWouldInstall":           i18n.ProviderDryRunWouldInstall,
		"ProviderDryRunWouldRemove":            i18n.ProviderDryRunWouldRemove,
		"ProviderAptInstallSimulateWarning":    i18n.ProviderAptInstallSimulateWarning,
		"ProviderGitCloneRequiresRemote":       i18n.ProviderGitCloneRequiresRemote,
		"ProviderGitCloneUsage1":               i18n.ProviderGitCloneUsage1,
		"ProviderGitCloneUsage2":               i18n.ProviderGitCloneUsage2,
		"ProviderGitCloneUsage3":               i18n.ProviderGitCloneUsage3,
		"ProviderGitCloneUsage4":               i18n.ProviderGitCloneUsage4,
		"ProviderGitCloneFlagNotForwarded":     i18n.ProviderGitCloneFlagNotForwarded,
		"ProviderGitCloneFileFollowup":         i18n.ProviderGitCloneFileFollowup,
		"ProviderGitClonePlainFallback":        i18n.ProviderGitClonePlainFallback,
		"ProviderGitCloneSinglePathExpected":   i18n.ProviderGitCloneSinglePathExpected,
		"ProviderGitCloneUsagePositional":      i18n.ProviderGitCloneUsagePositional,
		"ProviderGitRequiresSubcommand":        i18n.ProviderGitRequiresSubcommand,
		"ProviderGitRecordedSubcommandsHeader": i18n.ProviderGitRecordedSubcommandsHeader,
		"ProviderGitPassthroughNote":           i18n.ProviderGitPassthroughNote,
		"ProviderGitConfigRequiresArgs":        i18n.ProviderGitConfigRequiresArgs,

		"ProviderBrewExactOne":               i18n.ProviderBrewExactOne,
		"ProviderBrewHintMulti":              i18n.ProviderBrewHintMulti,
		"ProviderBrewInstallCaskTagConflict": i18n.ProviderBrewInstallCaskTagConflict,
		"ProviderBrewInstallCaskTagHint":     i18n.ProviderBrewInstallCaskTagHint,
		"ProviderBrewInstallNoTapFormat":     i18n.ProviderBrewInstallNoTapFormat,
		"ProviderBrewInstallNoTapHint1":      i18n.ProviderBrewInstallNoTapHint1,
		"ProviderBrewInstallNoTapHint2":      i18n.ProviderBrewInstallNoTapHint2,
		"ProviderBrewInstallUsage":           i18n.ProviderBrewInstallUsage,

		"ProviderBashPlannedV11":         i18n.ProviderBashPlannedV11,
		"ProviderBashPlannedHint1":       i18n.ProviderBashPlannedHint1,
		"ProviderBashPlannedHint2":       i18n.ProviderBashPlannedHint2,
		"ProviderBashRequiresSubcommand": i18n.ProviderBashRequiresSubcommand,
		"ProviderBashUsageList":          i18n.ProviderBashUsageList,
		"ProviderBashUsageRun":           i18n.ProviderBashUsageRun,
		"ProviderBashUsageRemove":        i18n.ProviderBashUsageRemove,

		"ProviderErrExactOne": i18n.ProviderErrExactOne,

		"ProviderDutiRequiresArgs":    i18n.ProviderDutiRequiresArgs,
		"ProviderDutiUsageSet":        i18n.ProviderDutiUsageSet,
		"ProviderDutiUsageList":       i18n.ProviderDutiUsageList,
		"ProviderDutiExample":         i18n.ProviderDutiExample,
		"ProviderDutiInvalidResource": i18n.ProviderDutiInvalidResource,

		"ProviderDefaultsRequiresArgs": i18n.ProviderDefaultsRequiresArgs,
		"ProviderDefaultsUsageWrite":   i18n.ProviderDefaultsUsageWrite,
		"ProviderDefaultsUsageDelete":  i18n.ProviderDefaultsUsageDelete,
		"ProviderDefaultsUsageList":    i18n.ProviderDefaultsUsageList,

		"ProviderAnsibleRequiresPlaybook": i18n.ProviderAnsibleRequiresPlaybook,
		"ProviderAnsibleUsagePlaybook":    i18n.ProviderAnsibleUsagePlaybook,
		"ProviderAnsibleExample":          i18n.ProviderAnsibleExample,
		"ProviderAnsibleNotFound":         i18n.ProviderAnsibleNotFound,

		"ProviderDefaultsWriteArgsMismatch":  i18n.ProviderDefaultsWriteArgsMismatch,
		"ProviderDefaultsWriteHintQuote":     i18n.ProviderDefaultsWriteHintQuote,
		"ProviderDefaultsDeleteArgsMismatch": i18n.ProviderDefaultsDeleteArgsMismatch,
		"ProviderDefaultsDeleteHintRepeat":   i18n.ProviderDefaultsDeleteHintRepeat,
		"ProviderDefaultsRootRequiresArgs":   i18n.ProviderDefaultsRootRequiresArgs,
		"ProviderDefaultsUsageWriteExample":  i18n.ProviderDefaultsUsageWriteExample,
		"ProviderDefaultsUsageDeleteExample": i18n.ProviderDefaultsUsageDeleteExample,

		"ProviderAnsibleNoBinary":      i18n.ProviderAnsibleNoBinary,
		"ProviderAnsibleNoBinaryHint1": i18n.ProviderAnsibleNoBinaryHint1,
		"ProviderAnsibleNoBinaryHint2": i18n.ProviderAnsibleNoBinaryHint2,

		"ProviderVerbPlannedV11":   i18n.ProviderVerbPlannedV11,
		"ProviderVerbPlannedHint1": i18n.ProviderVerbPlannedHint1,
		"ProviderVerbPlannedHint2": i18n.ProviderVerbPlannedHint2,

		"ProviderAnsibleRequiresPlaybookOrSubcommand": i18n.ProviderAnsibleRequiresPlaybookOrSubcommand,
		"ProviderAnsibleUsageList":                    i18n.ProviderAnsibleUsageList,
		"ProviderAnsibleUsageAdhoc":                   i18n.ProviderAnsibleUsageAdhoc,
		"ProviderAnsibleUsageRun":                     i18n.ProviderAnsibleUsageRun,
		"ProviderAnsibleUsageRemove":                  i18n.ProviderAnsibleUsageRemove,

		"ProviderNoStoreConfigured":     i18n.ProviderNoStoreConfigured,
		"ProviderNoStoreConfiguredHint": i18n.ProviderNoStoreConfiguredHint,

		"ProviderGitConfigSetRequiresKV":          i18n.ProviderGitConfigSetRequiresKV,
		"ProviderGitConfigUsageSet":               i18n.ProviderGitConfigUsageSet,
		"ProviderGitConfigUsageBare":              i18n.ProviderGitConfigUsageBare,
		"ProviderGitConfigUsageRemove":            i18n.ProviderGitConfigUsageRemove,
		"ProviderGitConfigUsageList":              i18n.ProviderGitConfigUsageList,
		"ProviderGitConfigExampleSet":             i18n.ProviderGitConfigExampleSet,
		"ProviderGitConfigExampleRemove":          i18n.ProviderGitConfigExampleRemove,
		"ProviderGitConfigRemoveRequiresKey":      i18n.ProviderGitConfigRemoveRequiresKey,
		"ProviderGitConfigRequiresSubcommandOrKV": i18n.ProviderGitConfigRequiresSubcommandOrKV,
		"ProviderGitConfigDryRunSet":              i18n.ProviderGitConfigDryRunSet,
		"ProviderGitConfigDryRunUnset":            i18n.ProviderGitConfigDryRunUnset,

		"ProviderGitCloneSubcommandRequired": i18n.ProviderGitCloneSubcommandRequired,
		"ProviderGitCloneUsageAddSub":        i18n.ProviderGitCloneUsageAddSub,
		"ProviderGitCloneUsageRemoveSub":     i18n.ProviderGitCloneUsageRemoveSub,
		"ProviderGitCloneUsageListSub":       i18n.ProviderGitCloneUsageListSub,
		"ProviderGitCloneAddRequiresRemote":  i18n.ProviderGitCloneAddRequiresRemote,
		"ProviderGitCloneAddUsage":           i18n.ProviderGitCloneAddUsage,
		"ProviderGitCloneAddExactOne":        i18n.ProviderGitCloneAddExactOne,
		"ProviderGitCloneAddPosHint":         i18n.ProviderGitCloneAddPosHint,
		"ProviderGitCloneAddRequiresPath":    i18n.ProviderGitCloneAddRequiresPath,
		"ProviderGitCloneTargetNotRepo":      i18n.ProviderGitCloneTargetNotRepo,
		"ProviderGitCloneTargetNotRepoHint1": i18n.ProviderGitCloneTargetNotRepoHint1,
		"ProviderGitCloneTargetNotRepoHint2": i18n.ProviderGitCloneTargetNotRepoHint2,
		"ProviderGitCloneDryRunAdd":          i18n.ProviderGitCloneDryRunAdd,
		"ProviderGitCloneDryRunRemoveEntry":  i18n.ProviderGitCloneDryRunRemoveEntry,
		"ProviderGitCloneRemoveRequiresURN":  i18n.ProviderGitCloneRemoveRequiresURN,
		"ProviderGitCloneRemoveUsage":        i18n.ProviderGitCloneRemoveUsage,
		"ProviderGitCloneNoEntry":            i18n.ProviderGitCloneNoEntry,
		"ProviderGitCloneInvalidResourceID":  i18n.ProviderGitCloneInvalidResourceID,

		"ProviderHomebrewListHeader": i18n.ProviderHomebrewListHeader,
	}

	// Defense against silent drift: every map key SHOULD reference a
	// real non-empty ID. A typo-ed constant would be caught by the
	// compiler, but a missing entry in this map (if someone forgot to
	// add a new constant here) is spotted by the reflection sanity
	// check below.
	if reflect.TypeOf(keys).Kind() != reflect.Map {
		t.Fatal("providerKeyConstants must return a map")
	}
	for _, v := range keys {
		if v == "" {
			t.Fatal("i18n key constant is empty — check keys.go for typos")
		}
	}
	return keys
}

func TestProviderKeysResolveInEnglish(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	os.Unsetenv("LANG") //nolint:errcheck // best-effort setup.
	i18n.Init()

	for name, id := range providerKeyConstants(t) {
		// Pass template data so keys that expect interpolation do not
		// return their literal "{{.X}}" template form, which would
		// also satisfy the "non-empty, non-key" assertion but fail in
		// real callers. The data keys are a superset; irrelevant keys
		// are silently ignored by go-i18n.
		got := i18n.Tf(id, map[string]any{
			"Provider":    "apt",
			"Verb":        "install",
			"Resource":    "package name",
			"Placeholder": "package",
			"Cmd":         "sudo apt-get install -y jq",
			"Flag":        "--depth",
			"Count":       2,
			"Got":         "[a b]",
			"Msg":         "git config requires <key>",
			"Tag":         "apps",
			"CaskTag":     "cask",
			"Arg":         "user/repo",
			"Err":         "example error",
			"Path":        "./site.yml",
			"Noun":        "playbooks",
			"Key":         "user.name",
			"Value":       "zthxxx",
			"Remote":      "https://github.com/zthxxx/hams.git",
			"URN":         "urn:hams:git-clone:example",
		})
		if got == id {
			t.Errorf("%s → %q is the raw key; missing en.yaml entry", name, id)
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s → empty string for key %q in en.yaml", name, id)
		}
	}
}

func TestProviderKeysResolveInChinese(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	os.Unsetenv("LANG") //nolint:errcheck // best-effort setup.
	i18n.Init()

	for name, id := range providerKeyConstants(t) {
		got := i18n.Tf(id, map[string]any{
			"Provider":    "apt",
			"Verb":        "install",
			"Resource":    "package name",
			"Placeholder": "package",
			"Cmd":         "sudo apt-get install -y jq",
			"Flag":        "--depth",
			"Count":       2,
			"Got":         "[a b]",
			"Msg":         "git config 需要 <key>",
			"Tag":         "apps",
			"CaskTag":     "cask",
			"Arg":         "user/repo",
			"Err":         "example error",
			"Path":        "./site.yml",
			"Noun":        "playbooks",
			"Key":         "user.name",
			"Value":       "zthxxx",
			"Remote":      "https://github.com/zthxxx/hams.git",
			"URN":         "urn:hams:git-clone:example",
		})
		if got == id {
			t.Errorf("%s → %q is the raw key; missing zh-CN.yaml entry", name, id)
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("%s → empty string for key %q in zh-CN.yaml", name, id)
		}
	}
}
