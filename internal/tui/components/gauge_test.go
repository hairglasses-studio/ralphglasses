package components

import (
	"strings"
	"testing"
)

func TestRepeatRune(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		n    int
		want string
	}{
		{"zero", 'x', 0, ""},
		{"negative", 'x', -3, ""},
		{"one", 'A', 1, "A"},
		{"five", '#', 5, "#####"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(repeatRune(tt.r, tt.n))
			if got != tt.want {
				t.Errorf("repeatRune(%c, %d) = %q, want %q", tt.r, tt.n, got, tt.want)
			}
		})
	}
}

func TestInlineGauge(t *testing.T) {
	tests := []struct {
		name          string
		current, max  float64
		width         int
		wantEmpty     bool   // expect ""
		wantFilled    int    // expected filled rune count (-1 to skip check)
		wantEmptyChar int    // expected empty rune count (-1 to skip check)
		wantContains  string // substring that must appear (rune-level)
	}{
		{
			name: "normal 50 pct",
			current: 50, max: 100, width: 40,
			wantFilled: 20, wantEmptyChar: 20,
		},
		{
			name: "current exceeds max clamps to 100 pct",
			current: 150, max: 100, width: 10,
			wantFilled: 10, wantEmptyChar: 0,
		},
		{
			name: "max is zero returns empty gauge",
			current: 50, max: 0, width: 10,
			wantFilled: 0, wantEmptyChar: 10,
		},
		{
			name:      "width is zero returns empty string",
			current:   50, max: 100, width: 0,
			wantEmpty: true,
		},
		{
			name: "green zone pct below 0.7",
			current: 60, max: 100, width: 10,
			wantFilled: 6, wantEmptyChar: 4,
		},
		{
			name: "yellow zone pct 0.7",
			current: 70, max: 100, width: 10,
			wantFilled: 7, wantEmptyChar: 3,
		},
		{
			name: "yellow zone pct 0.85",
			current: 85, max: 100, width: 20,
			wantFilled: 17, wantEmptyChar: 3,
		},
		{
			name: "red zone pct 0.9",
			current: 90, max: 100, width: 10,
			wantFilled: 9, wantEmptyChar: 1,
		},
		{
			name: "red zone pct 1.0",
			current: 100, max: 100, width: 10,
			wantFilled: 10, wantEmptyChar: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InlineGauge(tt.current, tt.max, tt.width)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected non-empty string")
			}
			// Count gauge runes in the output (which contains ANSI escapes)
			filledCount := strings.Count(got, string(gaugeFilled))
			emptyCount := strings.Count(got, string(gaugeEmpty))
			if tt.wantFilled >= 0 && filledCount != tt.wantFilled {
				t.Errorf("filled count = %d, want %d", filledCount, tt.wantFilled)
			}
			if tt.wantEmptyChar >= 0 && emptyCount != tt.wantEmptyChar {
				t.Errorf("empty count = %d, want %d", emptyCount, tt.wantEmptyChar)
			}
		})
	}
}

func TestInlineSparkline(t *testing.T) {
	tests := []struct {
		name  string
		data  []float64
		width int
		want  string // exact match when non-empty means "check non-empty"
		empty bool
	}{
		{
			name: "empty data", data: nil, width: 10,
			empty: true,
		},
		{
			name: "empty slice", data: []float64{}, width: 10,
			empty: true,
		},
		{
			name: "zero width", data: []float64{1, 2, 3}, width: 0,
			empty: true,
		},
		{
			name: "single value", data: []float64{42}, width: 10,
		},
		{
			name: "data longer than width uses last N", data: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, width: 5,
		},
		{
			name: "all same values", data: []float64{5, 5, 5, 5}, width: 10,
		},
		{
			name: "increasing values", data: []float64{0, 1, 2, 3, 4, 5, 6, 7}, width: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InlineSparkline(tt.data, tt.width)
			if tt.empty {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			if got == "" {
				t.Error("expected non-empty sparkline")
				return
			}
			// Verify sparkline block chars are present
			hasBlock := false
			for _, b := range sparkBlocks {
				if strings.ContainsRune(got, b) {
					hasBlock = true
					break
				}
			}
			if !hasBlock {
				t.Errorf("output %q contains no sparkline block characters", got)
			}
		})
	}
}

func TestInlineSparklineDataTruncation(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	width := 5
	got := InlineSparkline(data, width)
	// Count how many sparkline block chars appear
	count := 0
	for _, r := range got {
		for _, b := range sparkBlocks {
			if r == b {
				count++
				break
			}
		}
	}
	if count != width {
		t.Errorf("expected %d sparkline chars, got %d", width, count)
	}
}

func TestInlineSparklineAllSame(t *testing.T) {
	data := []float64{5, 5, 5, 5}
	got := InlineSparkline(data, 10)
	// All same values => range=0 => normalized=0 => idx=0 => all should be sparkBlocks[0]
	count := strings.Count(got, string(sparkBlocks[0]))
	if count != 4 {
		t.Errorf("expected 4 instances of lowest block char, got %d in %q", count, got)
	}
}

func TestActivityDot(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		frame  int
	}{
		{"inactive", false, 0},
		{"active frame 0", true, 0},
		{"active frame 3", true, 3},
		{"active frame wraps", true, len(brailleFrames) + 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ActivityDot(tt.active, tt.frame)
			if got == "" {
				t.Error("expected non-empty string")
				return
			}
			if !tt.active {
				if !strings.Contains(got, "·") {
					t.Errorf("inactive dot should contain '·', got %q", got)
				}
			} else {
				expectedRune := brailleFrames[tt.frame%len(brailleFrames)]
				if !strings.ContainsRune(got, expectedRune) {
					t.Errorf("expected braille frame %c, got %q", expectedRune, got)
				}
			}
		})
	}
}

func TestGaugeWithLabel(t *testing.T) {
	tests := []struct {
		name             string
		current, max     float64
		barWidth         int
		label            string
		wantLabelSuffix  string
		wantFilledCount  int
	}{
		{
			name: "basic label",
			current: 50, max: 100, barWidth: 20, label: "50/100",
			wantLabelSuffix: "50/100", wantFilledCount: 10,
		},
		{
			name: "empty label",
			current: 0, max: 100, barWidth: 10, label: "",
			wantFilledCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GaugeWithLabel(tt.current, tt.max, tt.barWidth, tt.label)
			if !strings.HasSuffix(got, tt.wantLabelSuffix) {
				t.Errorf("expected suffix %q in %q", tt.wantLabelSuffix, got)
			}
			filledCount := strings.Count(got, string(gaugeFilled))
			if filledCount != tt.wantFilledCount {
				t.Errorf("filled count = %d, want %d", filledCount, tt.wantFilledCount)
			}
		})
	}
}

func TestHealthSparklineGauge(t *testing.T) {
	tests := []struct {
		name      string
		data      []float64
		threshold float64
		width     int
		empty     bool
	}{
		{
			name: "empty data", data: nil, threshold: 5, width: 10,
			empty: true,
		},
		{
			name: "zero width", data: []float64{1, 2}, threshold: 5, width: 0,
			empty: true,
		},
		{
			name: "all below threshold",
			data: []float64{1, 2, 3, 4}, threshold: 10, width: 10,
		},
		{
			name: "all above threshold",
			data: []float64{11, 12, 13, 14}, threshold: 10, width: 10,
		},
		{
			name: "mixed above and below",
			data: []float64{1, 12, 3, 14, 5}, threshold: 10, width: 10,
		},
		{
			name: "data longer than width truncates",
			data: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, threshold: 5, width: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HealthSparkline(tt.data, tt.threshold, tt.width)
			if tt.empty {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			if got == "" {
				t.Error("expected non-empty health sparkline")
				return
			}
			// Verify output contains sparkline block chars
			hasBlock := false
			for _, b := range sparkBlocks {
				if strings.ContainsRune(got, b) {
					hasBlock = true
					break
				}
			}
			if !hasBlock {
				t.Errorf("output %q contains no sparkline block characters", got)
			}
		})
	}
}

func TestHealthSparklineTruncation(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	width := 5
	got := HealthSparkline(data, 5, width)
	count := 0
	for _, r := range got {
		for _, b := range sparkBlocks {
			if r == b {
				count++
				break
			}
		}
	}
	if count != width {
		t.Errorf("expected %d sparkline chars, got %d", width, count)
	}
}
