package components

import (
	"strings"
	"testing"
)

func TestSparklineEmpty(t *testing.T) {
	result := Sparkline(nil, 10)
	if result != "" {
		t.Errorf("expected empty string for nil values, got %q", result)
	}
	result = Sparkline([]float64{}, 10)
	if result != "" {
		t.Errorf("expected empty string for empty values, got %q", result)
	}
}

func TestSparklineSingleValue(t *testing.T) {
	result := Sparkline([]float64{42}, 10)
	// Single value means all-same, should use middle char
	expected := string(sparkChars[len(sparkChars)/2])
	if result != expected {
		t.Errorf("expected %q for single value, got %q", expected, result)
	}
}

func TestSparklineIncreasing(t *testing.T) {
	values := []float64{0, 1, 2, 3, 4, 5, 6, 7}
	result := Sparkline(values, 20)
	runes := []rune(result)
	if len(runes) != 8 {
		t.Fatalf("expected 8 chars, got %d", len(runes))
	}
	// First char should be lowest, last should be highest
	if runes[0] != sparkChars[0] {
		t.Errorf("first char should be %c, got %c", sparkChars[0], runes[0])
	}
	if runes[len(runes)-1] != sparkChars[len(sparkChars)-1] {
		t.Errorf("last char should be %c, got %c", sparkChars[len(sparkChars)-1], runes[len(runes)-1])
	}
}

func TestSparklineAllSame(t *testing.T) {
	values := []float64{5, 5, 5, 5}
	result := Sparkline(values, 10)
	expected := strings.Repeat(string(sparkChars[len(sparkChars)/2]), 4)
	if result != expected {
		t.Errorf("expected %q for all-same values, got %q", expected, result)
	}
}

func TestSparklineTruncatesToWidth(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	width := 5
	result := Sparkline(values, width)
	runes := []rune(result)
	if len(runes) != width {
		t.Errorf("expected %d chars after truncation, got %d", width, len(runes))
	}
}

func TestSparklineWithLabelNoData(t *testing.T) {
	result := SparklineWithLabel("Tokens", nil, 10, "k")
	expected := "Tokens: (no data)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSparklineWithLabelFormatting(t *testing.T) {
	values := []float64{1.0, 2.0, 3.0}
	result := SparklineWithLabel("Cost", values, 10, "$")
	if !strings.HasPrefix(result, "Cost: ") {
		t.Errorf("expected prefix 'Cost: ', got %q", result)
	}
	if !strings.HasSuffix(result, "(3.0$)") {
		t.Errorf("expected suffix '(3.0$)', got %q", result)
	}
}
