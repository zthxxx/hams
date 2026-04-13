package i18n

import (
	"testing"
)

func TestDetectLocale_Default(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")

	loc := DetectLocale()
	if loc.Language != "en" || loc.Region != "US" {
		t.Errorf("DetectLocale() = %v, want en_US", loc)
	}
}

func TestDetectLocale_LCALL_Priority(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")

	loc := DetectLocale()
	if loc.Language != "zh" || loc.Region != "CN" {
		t.Errorf("DetectLocale() = %v, want zh_CN (LC_ALL takes precedence)", loc)
	}
}

func TestDetectLocale_LANG_Fallback(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "ja_JP.UTF-8")

	loc := DetectLocale()
	if loc.Language != "ja" || loc.Region != "JP" {
		t.Errorf("DetectLocale() = %v, want ja_JP", loc)
	}
}

func TestParseLocale_Variants(t *testing.T) {
	tests := []struct {
		input    string
		language string
		region   string
	}{
		{"en_US.UTF-8", "en", "US"},
		{"zh_CN", "zh", "CN"},
		{"ja_JP.eucJP", "ja", "JP"},
		{"C", "en", "US"},
		{"POSIX", "en", "US"},
		{"fr", "fr", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			loc, ok := parseLocale(tt.input)
			if !ok {
				t.Fatalf("parseLocale(%q) returned not ok", tt.input)
			}
			if loc.Language != tt.language {
				t.Errorf("Language = %q, want %q", loc.Language, tt.language)
			}
			if loc.Region != tt.region {
				t.Errorf("Region = %q, want %q", loc.Region, tt.region)
			}
		})
	}
}

func TestLocale_String(t *testing.T) {
	loc := Locale{Language: "en", Region: "US"}
	if got := loc.String(); got != "en_US" {
		t.Errorf("String() = %q, want 'en_US'", got)
	}

	loc2 := Locale{Language: "fr"}
	if got := loc2.String(); got != "fr" {
		t.Errorf("String() = %q, want 'fr'", got)
	}
}

func TestLocale_IsSupported(t *testing.T) {
	if !DefaultLocale.IsSupported() {
		t.Error("en_US should be supported")
	}

	zhCN := Locale{Language: "zh", Region: "CN"}
	if !zhCN.IsSupported() {
		t.Error("zh_CN should be supported")
	}

	unsupported := Locale{Language: "ja", Region: "JP"}
	if unsupported.IsSupported() {
		t.Error("ja_JP should not be supported yet")
	}
}

func TestResolveLocaleFile_ExactMatch(t *testing.T) {
	// zh-CN.yaml exists, so zh_CN should resolve to it.
	got := resolveLocaleFile(Locale{Language: "zh", Region: "CN"})
	if got != "locales/zh-CN.yaml" {
		t.Errorf("resolveLocaleFile(zh_CN) = %q, want locales/zh-CN.yaml", got)
	}
}

func TestResolveLocaleFile_FallbackToSibling(t *testing.T) {
	// zh_TW.yaml does not exist, zh.yaml does not exist,
	// but zh-CN.yaml exists as the first zh-*.yaml alphabetically.
	got := resolveLocaleFile(Locale{Language: "zh", Region: "TW"})
	if got != "locales/zh-CN.yaml" {
		t.Errorf("resolveLocaleFile(zh_TW) = %q, want locales/zh-CN.yaml (sibling fallback)", got)
	}
}

func TestResolveLocaleFile_NoMatch(t *testing.T) {
	// No Japanese locale files exist.
	got := resolveLocaleFile(Locale{Language: "ja", Region: "JP"})
	if got != "" {
		t.Errorf("resolveLocaleFile(ja_JP) = %q, want empty string", got)
	}
}

func TestResolveLocaleFile_BaseLanguageOnly(t *testing.T) {
	// zh with no region should fallback to zh-CN.yaml via sibling match.
	got := resolveLocaleFile(Locale{Language: "zh"})
	if got != "locales/zh-CN.yaml" {
		t.Errorf("resolveLocaleFile(zh) = %q, want locales/zh-CN.yaml", got)
	}
}

func TestInit_ZhTW_FallsBackToZhCN(t *testing.T) {
	// zh_TW should load zh-CN.yaml translations via sibling fallback.
	t.Setenv("LC_ALL", "zh_TW.UTF-8")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	Init()

	got := T("app.title")
	want := "hams — 声明式工作站环境管理工具"
	if got != want {
		t.Errorf("T(\"app.title\") with zh_TW = %q, want %q (zh-CN fallback)", got, want)
	}
}

func TestInit_ZhHK_FallsBackToZhCN(t *testing.T) {
	t.Setenv("LC_ALL", "zh_HK.UTF-8")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	Init()

	got := T("app.title")
	want := "hams — 声明式工作站环境管理工具"
	if got != want {
		t.Errorf("T(\"app.title\") with zh_HK = %q, want %q (zh-CN fallback)", got, want)
	}
}

func TestT_DefaultLocale_Passthrough(t *testing.T) {
	// Init with en_US (default).
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	Init()

	got := T("app.title")
	want := "hams — declarative workstation environment management"
	if got != want {
		t.Errorf("T(\"app.title\") = %q, want %q", got, want)
	}
}

func TestT_ZhCN_TranslatedKey(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	Init()

	got := T("app.title")
	want := "hams — 声明式工作站环境管理工具"
	if got != want {
		t.Errorf("T(\"app.title\") = %q, want %q", got, want)
	}
}

func TestT_ZhCN_FallbackForMissingKey(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	Init()

	got := T("some.untranslated.key")
	if got != "some.untranslated.key" {
		t.Errorf("T(\"some.untranslated.key\") = %q, want key passthrough", got)
	}
}

func TestT_NilLocalizer_Passthrough(t *testing.T) {
	// Reset localizer to nil.
	prev := localizer
	localizer = nil
	t.Cleanup(func() { localizer = prev })

	if got := T("hello world"); got != "hello world" {
		t.Errorf("T() = %q, want 'hello world'", got)
	}
}

func TestTf_WithTemplateData(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	Init()

	// Tf with unknown key falls back to the key.
	got := Tf("unknown.key", map[string]any{"name": "test"})
	if got != "unknown.key" {
		t.Errorf("Tf(\"unknown.key\") = %q, want key passthrough", got)
	}
}
