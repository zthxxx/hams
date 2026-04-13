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

func TestT_Passthrough(t *testing.T) {
	if got := T("hello world"); got != "hello world" {
		t.Errorf("T() = %q, want 'hello world'", got)
	}
}

func TestT_ZhCN_TranslatedKey(t *testing.T) {
	prev := activeLocale
	t.Cleanup(func() { activeLocale = prev })

	activeLocale = Locale{Language: "zh", Region: "CN"}

	got := T("app.title")
	want := "hams — 声明式工作站环境管理工具"
	if got != want {
		t.Errorf("T(\"app.title\") = %q, want %q", got, want)
	}
}

func TestT_ZhCN_FallbackForMissingKey(t *testing.T) {
	prev := activeLocale
	t.Cleanup(func() { activeLocale = prev })

	activeLocale = Locale{Language: "zh", Region: "CN"}

	got := T("some.untranslated.key")
	if got != "some.untranslated.key" {
		t.Errorf("T(\"some.untranslated.key\") = %q, want key passthrough", got)
	}
}
