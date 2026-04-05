package finance

import (
	"testing"
)

func TestNormalizeMerchant_Amazon(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"AMZN MKTP", "Amazon"},
		{"AMAZON.COM", "Amazon"},
		{"AMZN *", "Amazon"},
	}
	for _, tt := range tests {
		got := NormalizeMerchant(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeMerchant(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeMerchant_Square(t *testing.T) {
	got := NormalizeMerchant("SQ *BLUE BOTTLE COFFEE")
	if got != "Square: BLUE BOTTLE COFFEE" {
		t.Errorf("Square normalize = %q", got)
	}
}

func TestNormalizeMerchant_Uber(t *testing.T) {
	trips := NormalizeMerchant("UBER TRIP")
	if trips != "Uber" {
		t.Errorf("Uber trip = %q, want Uber", trips)
	}
	eats := NormalizeMerchant("UBER *EATS")
	if eats != "Uber Eats" {
		t.Errorf("Uber eats = %q, want Uber Eats", eats)
	}
}

func TestNormalizeMerchant_Costco(t *testing.T) {
	got := NormalizeMerchant("COSTCO")
	if got != "Costco" {
		t.Errorf("Costco = %q, want Costco", got)
	}
}

func TestNormalizeMerchant_PassThrough(t *testing.T) {
	// Unknown merchant — returned as-is (trimmed)
	got := NormalizeMerchant("LOCAL PIZZA PLACE")
	if got != "LOCAL PIZZA PLACE" {
		t.Errorf("passthrough = %q, want 'LOCAL PIZZA PLACE'", got)
	}
}

func TestNormalizeMerchant_Empty(t *testing.T) {
	got := NormalizeMerchant("")
	if got != "" {
		t.Errorf("empty = %q, want empty", got)
	}
}

func TestCategoryFromMerchant(t *testing.T) {
	tests := []struct {
		merchant string
		want     string
	}{
		{"Amazon", "shopping"},
		{"Uber Eats", "food"},
		{"Uber", "transport"},
		{"Costco", "groceries"},
		{"Spotify", "entertainment"},
		{"Pharmacy", "health"},
		{"Apple", "tech"},
		{"Square: BLUE BOTTLE", "food"},
		{"Toast: BURGER JOINT", "food"},
		{"PayPal: EBAY", "shopping"},
		{"Unknown Place", ""},
	}
	for _, tt := range tests {
		got := CategoryFromMerchant(tt.merchant)
		if got != tt.want {
			t.Errorf("CategoryFromMerchant(%q) = %q, want %q", tt.merchant, got, tt.want)
		}
	}
}

func TestAllPatterns_Compile(t *testing.T) {
	// Regression guard: all 23 patterns should be compiled
	if len(merchantPatterns) != 23 {
		t.Errorf("merchantPatterns count = %d, want 23", len(merchantPatterns))
	}
	// Exercise each pattern
	for _, mp := range merchantPatterns {
		if mp.Pattern == nil {
			t.Errorf("nil pattern for %q", mp.Normalized)
		}
	}
}

func TestNormalizeMerchant_StripsSuffix(t *testing.T) {
	// Verify suffix stripping (card numbers, phone numbers, zip codes)
	got := NormalizeMerchant("STARBUCKS #12345")
	if got != "Starbucks" {
		t.Errorf("with suffix = %q, want Starbucks", got)
	}
}
