// Package finance provides financial intelligence: merchant normalization,
// spending pattern analysis, and anomaly detection.
package finance

import (
	"regexp"
	"strings"
)

// merchantPattern maps a regex to a normalized merchant name.
type merchantPattern struct {
	Pattern    *regexp.Regexp
	Normalized string
}

// merchantPatterns is the ordered list of normalization rules.
// More specific patterns should come first.
var merchantPatterns = []merchantPattern{
	{regexp.MustCompile(`(?i)^AMZN\s*\*|AMAZON\.COM|AMZN MKTP`), "Amazon"},
	{regexp.MustCompile(`(?i)^SQ\s*\*\s*(.+)`), "Square: $1"},
	{regexp.MustCompile(`(?i)^TST\s*\*\s*(.+)`), "Toast: $1"},
	{regexp.MustCompile(`(?i)^UBER\s*\*?\s*EATS`), "Uber Eats"},
	{regexp.MustCompile(`(?i)^UBER\s*\*?\s*TRIP`), "Uber"},
	{regexp.MustCompile(`(?i)^LYFT\s*\*`), "Lyft"},
	{regexp.MustCompile(`(?i)^DOORDASH`), "DoorDash"},
	{regexp.MustCompile(`(?i)^GRUBHUB`), "Grubhub"},
	{regexp.MustCompile(`(?i)^PAYPAL\s*\*\s*(.+)`), "PayPal: $1"},
	{regexp.MustCompile(`(?i)^VENMO\s*\*`), "Venmo"},
	{regexp.MustCompile(`(?i)^GOOGLE\s*\*`), "Google"},
	{regexp.MustCompile(`(?i)^APPLE\.COM`), "Apple"},
	{regexp.MustCompile(`(?i)^SPOTIFY`), "Spotify"},
	{regexp.MustCompile(`(?i)^NETFLIX`), "Netflix"},
	{regexp.MustCompile(`(?i)^HULU`), "Hulu"},
	{regexp.MustCompile(`(?i)^TARGET\b`), "Target"},
	{regexp.MustCompile(`(?i)^WAL-?MART|WM SUPERCENTER`), "Walmart"},
	{regexp.MustCompile(`(?i)^COSTCO`), "Costco"},
	{regexp.MustCompile(`(?i)^TRADER JOE`), "Trader Joe's"},
	{regexp.MustCompile(`(?i)^WHOLE FOODS`), "Whole Foods"},
	{regexp.MustCompile(`(?i)^STARBUCKS`), "Starbucks"},
	{regexp.MustCompile(`(?i)^SHELL\b|^CHEVRON\b|^EXXON|^BP\b`), "Gas Station"},
	{regexp.MustCompile(`(?i)^CVS\b|^WALGREENS`), "Pharmacy"},
}

// stripSuffixes removes transaction noise like card numbers, location codes, dates.
var stripSuffixes = regexp.MustCompile(`\s+#\d+|\s+\d{3}-\d{3}-\d{4}|\s+\d{2}/\d{2}$|\s+[A-Z]{2}\s+\d{5}$|\s+\d{10,}$`)

// NormalizeMerchant cleans up raw transaction descriptions into readable merchant names.
func NormalizeMerchant(raw string) string {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return clean
	}

	// Strip trailing noise
	clean = stripSuffixes.ReplaceAllString(clean, "")
	clean = strings.TrimSpace(clean)

	// Try pattern matching
	for _, mp := range merchantPatterns {
		if mp.Pattern.MatchString(clean) {
			result := mp.Pattern.ReplaceAllString(clean, mp.Normalized)
			return strings.TrimSpace(result)
		}
	}

	// Fallback: title case the first ~40 chars
	if len(clean) > 40 {
		clean = clean[:40]
	}
	return clean
}

// merchantCategories maps normalized merchant names to spending categories.
var merchantCategories = map[string]string{
	"Amazon":       "shopping",
	"Uber Eats":    "food",
	"DoorDash":     "food",
	"Grubhub":      "food",
	"Uber":         "transport",
	"Lyft":         "transport",
	"Spotify":      "entertainment",
	"Netflix":      "entertainment",
	"Hulu":         "entertainment",
	"Target":       "shopping",
	"Walmart":      "shopping",
	"Costco":       "groceries",
	"Trader Joe's": "groceries",
	"Whole Foods":  "groceries",
	"Starbucks":    "food",
	"Gas Station":  "transport",
	"Pharmacy":     "health",
	"Apple":        "tech",
	"Google":       "tech",
}

// CategoryFromMerchant returns a spending category for a normalized merchant name.
// Returns empty string if no match found.
func CategoryFromMerchant(normalized string) string {
	// Direct lookup
	if cat, ok := merchantCategories[normalized]; ok {
		return cat
	}
	// Check prefix for Square/Toast/PayPal vendor names
	for prefix, cat := range map[string]string{
		"Square:": "food",
		"Toast:":  "food",
		"PayPal:": "shopping",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return cat
		}
	}
	return ""
}
