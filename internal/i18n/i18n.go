// Package i18n handles locale detection and message translation using nicksnyder/go-i18n.
package i18n

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFS embed.FS

// Locale represents the detected user locale.
type Locale struct {
	Language string // e.g., "en", "zh"
	Region   string // e.g., "US", "CN"
}

// DefaultLocale is en_US.
var DefaultLocale = Locale{Language: "en", Region: "US"}

var localizer *i18n.Localizer

// Init detects the locale and initializes the go-i18n localizer.
func Init() {
	locale := DetectLocale()

	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Always load English as the default.
	if _, err := bundle.LoadMessageFileFS(localeFS, "locales/en.yaml"); err != nil {
		slog.Debug("loading embedded English locale failed", "error", err)
	}

	// Load a non-English locale file using the fallback chain:
	// 1. Exact match: lang-REGION.yaml (e.g., zh-TW.yaml)
	// 2. Base language: lang.yaml (e.g., zh.yaml)
	// 3. First lang-*.yaml alphabetically (e.g., zh-CN.yaml)
	// 4. If none found, rely on English only.
	var loadedTag string
	if locale.Language != "en" {
		if file := resolveLocaleFile(locale); file != "" {
			if _, err := bundle.LoadMessageFileFS(localeFS, file); err != nil {
				slog.Debug("loading embedded locale failed", "file", file, "error", err)
			}
			// Extract the BCP-47 tag from the filename (e.g., "locales/zh-CN.yaml" → "zh-CN").
			loadedTag = strings.TrimSuffix(strings.TrimPrefix(file, "locales/"), ".yaml")
		}
	}

	// Build the localizer with fallback chain.
	// Include both the user's locale tag and the actually-loaded file's tag,
	// so go-i18n can match even when they differ (e.g., zh-TW user loads zh-CN).
	userTag := locale.Language
	if locale.Region != "" {
		userTag = locale.Language + "-" + locale.Region
	}
	tags := []string{userTag}
	if loadedTag != "" && loadedTag != userTag {
		tags = append(tags, loadedTag)
	}
	tags = append(tags, "en")
	localizer = i18n.NewLocalizer(bundle, tags...)
}

// resolveLocaleFile finds the best matching locale file for the given locale.
// Fallback chain: lang-REGION.yaml → lang.yaml → first lang-*.yaml alphabetically → "".
func resolveLocaleFile(locale Locale) string {
	// Step 1: exact match (e.g., zh-TW.yaml).
	if locale.Region != "" {
		candidate := "locales/" + locale.Language + "-" + locale.Region + ".yaml"
		if fileExists(candidate) {
			return candidate
		}
	}

	// Step 2: base language (e.g., zh.yaml).
	candidate := "locales/" + locale.Language + ".yaml"
	if fileExists(candidate) {
		return candidate
	}

	// Step 3: first lang-*.yaml alphabetically (e.g., zh-CN.yaml).
	prefix := locale.Language + "-"
	entries, err := fs.ReadDir(localeFS, "locales")
	if err != nil {
		return ""
	}

	var matches []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".yaml") {
			matches = append(matches, name)
		}
	}
	if len(matches) > 0 {
		sort.Strings(matches)
		return "locales/" + matches[0]
	}

	return ""
}

// fileExists checks if a file exists in the embedded locale filesystem.
func fileExists(path string) bool {
	_, err := fs.Stat(localeFS, path)
	return err == nil
}

// DetectLocale reads LC_ALL, LC_CTYPE, LANG environment variables
// in priority order and parses the locale string.
func DetectLocale() Locale {
	// Priority: LC_ALL > LC_CTYPE > LANG.
	for _, env := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if val := os.Getenv(env); val != "" {
			if loc, ok := parseLocale(val); ok {
				return loc
			}
		}
	}
	return DefaultLocale
}

// parseLocale parses a locale string like "en_US.UTF-8" into a Locale.
func parseLocale(s string) (Locale, bool) {
	if s == "" || s == "C" || s == "POSIX" {
		return DefaultLocale, true
	}

	// Strip encoding (e.g., ".UTF-8").
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		s = s[:idx]
	}

	// Split language_REGION.
	parts := strings.SplitN(s, "_", 2)
	if len(parts) == 0 || parts[0] == "" {
		return Locale{}, false
	}

	loc := Locale{Language: strings.ToLower(parts[0])}
	if len(parts) > 1 {
		loc.Region = strings.ToUpper(parts[1])
	}

	return loc, true
}

// String returns the locale as "language_REGION" (e.g., "en_US").
func (l Locale) String() string {
	if l.Region != "" {
		return l.Language + "_" + l.Region
	}
	return l.Language
}

// IsSupported checks if translations exist for this locale.
func (l Locale) IsSupported() bool {
	if l.Language == "en" {
		return true
	}
	return resolveLocaleFile(l) != ""
}

// T returns the translated string for the given message ID.
// Falls back to English if the active locale has no translation for this key.
// Falls back to the key itself if English also has no translation.
func T(msgID string) string {
	if localizer == nil {
		return msgID
	}
	msg, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: msgID})
	if err != nil || msg == "" {
		return msgID
	}
	return msg
}

// Tf returns the translated string with template data interpolation.
// Falls back the same way as T.
func Tf(msgID string, data map[string]any) string {
	if localizer == nil {
		return msgID
	}
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    msgID,
		TemplateData: data,
	})
	if err != nil || msg == "" {
		return msgID
	}
	return msg
}
