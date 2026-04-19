package i18n_test

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var testLocaleFS embed.FS

// TestLocalesAreInParity asserts that every locales/*.yaml file declares
// the same set of message IDs. A translator who adds a new key to
// `en.yaml` without mirroring it into `zh-CN.yaml` (or vice-versa)
// would silently ship a locale-specific gap — users on the missing
// side would see the raw key string instead of translated text.
//
// The test discovers locale files dynamically from the embedded FS
// so adding `locales/fr.yaml` or `locales/ja.yaml` in the future
// doesn't require updating the test. All non-English locales are
// diffed against `en.yaml` (the canonical source).
func TestLocalesAreInParity(t *testing.T) {
	entries, err := fs.ReadDir(testLocaleFS, "locales")
	if err != nil {
		t.Fatalf("read locales: %v", err)
	}
	baseline := loadLocaleKeys(t, "locales/en.yaml")
	if len(baseline) == 0 {
		t.Fatalf("en.yaml is empty; canonical source must define at least one key")
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") || name == "en.yaml" {
			continue
		}
		path := "locales/" + name
		keys := loadLocaleKeys(t, path)

		missing := keyDiff(baseline, keys)
		if len(missing) > 0 {
			t.Errorf("%s is missing %d translations present in en.yaml: %v",
				name, len(missing), missing)
		}
		extra := keyDiff(keys, baseline)
		if len(extra) > 0 {
			t.Errorf("%s declares %d keys that en.yaml does not carry (canonical source must win): %v",
				name, len(extra), extra)
		}
	}
}

func loadLocaleKeys(t *testing.T, path string) map[string]struct{} {
	t.Helper()

	body, err := testLocaleFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var raw []struct {
		ID    string `yaml:"id"`
		Other string `yaml:"other"`
	}
	if unErr := yaml.Unmarshal(body, &raw); unErr != nil {
		t.Fatalf("parse %s: %v", path, unErr)
	}
	keys := make(map[string]struct{}, len(raw))
	for _, e := range raw {
		if e.ID == "" {
			continue
		}
		keys[e.ID] = struct{}{}
	}
	return keys
}

// keyDiff returns the alphabetically-sorted list of keys in a that are
// absent from b. Sorting keeps the failure messages reproducible so
// test output diffs cleanly across runs.
func keyDiff(a, b map[string]struct{}) []string {
	var out []string
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
