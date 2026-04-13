// Package i18n handles locale detection and message catalog loading for internationalized CLI output.
package i18n

import (
	"os"
	"strings"
)

// Locale represents the detected user locale.
type Locale struct {
	Language string // e.g., "en", "zh"
	Region   string // e.g., "US", "CN"
}

// DefaultLocale is en_US.
var DefaultLocale = Locale{Language: "en", Region: "US"}

// DetectLocale reads LC_ALL, LC_CTYPE, LANG environment variables
// in priority order and parses the locale string.
func DetectLocale() Locale {
	// Priority: LC_ALL > LC_CTYPE > LANG
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
// Currently only en_US is supported.
func (l Locale) IsSupported() bool {
	return l.Language == "en"
}

// T returns the translated string for the given key.
// Currently returns the key itself (en_US passthrough).
// Future: load translations from message catalogs.
func T(key string) string {
	return key
}
