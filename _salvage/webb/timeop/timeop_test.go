package timeop

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	// Fixed reference time for deterministic tests
	refTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	opts := DefaultOptions().WithReference(refTime)

	tests := []struct {
		name       string
		input      string
		wantErr    bool
		checkTime  func(t *testing.T, ts TimeSpec)
		checkRel   bool // Expected IsRelative value
	}{
		// RFC3339 formats
		{
			name:    "RFC3339 UTC",
			input:   "2024-01-01T10:00:00Z",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				if ts.Time.Year() != 2024 || ts.Time.Month() != 1 || ts.Time.Day() != 1 {
					t.Errorf("expected 2024-01-01, got %v", ts.Time)
				}
			},
			checkRel: false,
		},
		{
			name:     "RFC3339 with offset",
			input:    "2024-01-01T10:00:00-08:00",
			wantErr:  false,
			checkRel: false,
		},

		// Relative times
		{
			name:    "relative -1h",
			input:   "-1h",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				expected := refTime.Add(-time.Hour)
				if !ts.Time.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, ts.Time)
				}
			},
			checkRel: true,
		},
		{
			name:     "relative -30m",
			input:    "-30m",
			wantErr:  false,
			checkRel: true,
		},
		{
			name:    "relative -7d",
			input:   "-7d",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				expected := refTime.Add(-7 * 24 * time.Hour)
				if !ts.Time.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, ts.Time)
				}
			},
			checkRel: true,
		},
		{
			name:     "relative -2w",
			input:    "-2w",
			wantErr:  false,
			checkRel: true,
		},

		// Presets
		{
			name:    "preset 1h",
			input:   "1h",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				expected := refTime.Add(-time.Hour)
				if !ts.Time.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, ts.Time)
				}
			},
			checkRel: true,
		},
		{
			name:     "preset 30d",
			input:    "30d",
			wantErr:  false,
			checkRel: true,
		},
		{
			name:     "preset 6mo",
			input:    "6mo",
			wantErr:  false,
			checkRel: true,
		},

		// Unix timestamps
		{
			name:    "unix seconds",
			input:   "1704110400",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				expected := time.Unix(1704110400, 0)
				if !ts.Time.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, ts.Time)
				}
			},
			checkRel: false,
		},
		{
			name:     "unix millis",
			input:    "1704110400000",
			wantErr:  false,
			checkRel: false,
		},

		// Slack timestamps
		{
			name:    "slack timestamp",
			input:   "1234567890.123456",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				if ts.Time.Unix() != 1234567890 {
					t.Errorf("expected unix 1234567890, got %d", ts.Time.Unix())
				}
				// Check microseconds preserved
				micros := ts.Time.Nanosecond() / 1000
				if micros != 123456 {
					t.Errorf("expected 123456 microseconds, got %d", micros)
				}
			},
			checkRel: false,
		},

		// Special values
		{
			name:     "now",
			input:    "now",
			wantErr:  false,
			checkRel: true,
		},
		{
			name:     "empty string",
			input:    "",
			wantErr:  false,
			checkRel: true,
		},
		{
			name:     "NOW uppercase",
			input:    "NOW",
			wantErr:  false,
			checkRel: true,
		},

		// Date only
		{
			name:    "date only",
			input:   "2024-01-15",
			wantErr: false,
			checkTime: func(t *testing.T, ts TimeSpec) {
				if ts.Time.Year() != 2024 || ts.Time.Month() != 1 || ts.Time.Day() != 15 {
					t.Errorf("expected 2024-01-15, got %v", ts.Time)
				}
			},
			checkRel: false,
		},

		// Errors
		{
			name:    "invalid string",
			input:   "not-a-time",
			wantErr: true,
		},
		{
			name:    "negative zero",
			input:   "-0h",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := Parse(tt.input, opts)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if ts.IsRelative != tt.checkRel {
				t.Errorf("IsRelative = %v, want %v", ts.IsRelative, tt.checkRel)
			}

			if tt.checkTime != nil {
				tt.checkTime(t, ts)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	refTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	opts := DefaultOptions().WithReference(refTime).WithAllowFuture()

	tests := []struct {
		name      string
		start     string
		end       string
		wantErr   bool
		checkDur  time.Duration
	}{
		{
			name:     "relative start, now end",
			start:    "-1h",
			end:      "now",
			wantErr:  false,
			checkDur: time.Hour,
		},
		{
			name:     "relative start, empty end",
			start:    "-7d",
			end:      "",
			wantErr:  false,
			checkDur: 7 * 24 * time.Hour,
		},
		{
			name:    "absolute times",
			start:   "2024-01-01T00:00:00Z",
			end:     "2024-01-02T00:00:00Z",
			wantErr: false,
			checkDur: 24 * time.Hour,
		},
		{
			name:    "empty start",
			start:   "",
			end:     "now",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := ParseRange(tt.start, tt.end, opts)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.checkDur > 0 && tr.Duration != tt.checkDur {
				t.Errorf("Duration = %v, want %v", tr.Duration, tt.checkDur)
			}
		})
	}
}

func TestParsePreset(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		want    time.Duration
		wantErr bool
	}{
		{"1h", "1h", time.Hour, false},
		{"6h", "6h", 6 * time.Hour, false},
		{"24h", "24h", 24 * time.Hour, false},
		{"7d", "7d", 7 * 24 * time.Hour, false},
		{"30d", "30d", 30 * 24 * time.Hour, false},
		{"90d", "90d", 90 * 24 * time.Hour, false},
		{"6mo", "6mo", 180 * 24 * time.Hour, false},
		{"custom 15h", "15h", 15 * time.Hour, false},
		{"custom 45d", "45d", 45 * 24 * time.Hour, false},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePreset(tt.preset)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSlackTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSecs  int64
		wantMicro int
		wantErr   bool
	}{
		{
			name:      "valid",
			input:     "1234567890.123456",
			wantSecs:  1234567890,
			wantMicro: 123456,
			wantErr:   false,
		},
		{
			name:    "invalid format",
			input:   "1234567890",
			wantErr: true,
		},
		{
			name:    "invalid seconds",
			input:   "abc.123456",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := ParseSlackTimestamp(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if ts.Time.Unix() != tt.wantSecs {
				t.Errorf("seconds = %d, want %d", ts.Time.Unix(), tt.wantSecs)
			}

			gotMicro := ts.Time.Nanosecond() / 1000
			if gotMicro != tt.wantMicro {
				t.Errorf("microseconds = %d, want %d", gotMicro, tt.wantMicro)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{time.Second, "1s"},
		{30 * time.Second, "30s"},
		{time.Minute, "1m"},
		{5 * time.Minute, "5m"},
		{time.Hour, "1h"},
		{6 * time.Hour, "6h"},
		{24 * time.Hour, "1d"},
		{7 * 24 * time.Hour, "1w"},
		{30 * 24 * time.Hour, "1mo"},
		{90 * 24 * time.Hour, "3mo"},
		{500 * time.Millisecond, "500ms"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatSlackRoundtrip(t *testing.T) {
	original := "1234567890.123456"

	ts, err := ParseSlackTimestamp(original)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	formatted := FormatSlack(ts)
	if formatted != original {
		t.Errorf("roundtrip failed: got %q, want %q", formatted, original)
	}
}

func TestValidateRange(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		start   time.Time
		end     time.Time
		wantErr bool
	}{
		{
			name:    "valid",
			start:   now.Add(-time.Hour),
			end:     now,
			wantErr: false,
		},
		{
			name:    "end before start",
			start:   now,
			end:     now.Add(-time.Hour),
			wantErr: true,
		},
		{
			name:    "zero start",
			start:   time.Time{},
			end:     now,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := TimeRange{
				Start:    FromTime(tt.start),
				End:      FromTime(tt.end),
				Duration: tt.end.Sub(tt.start),
			}
			err := ValidateRange(tr)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewTimeRangeFromPreset(t *testing.T) {
	tr, err := NewTimeRangeFromPreset("7d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Duration != 7*24*time.Hour {
		t.Errorf("duration = %v, want 7d", tr.Duration)
	}

	if !tr.Start.IsRelative {
		t.Error("start should be marked as relative")
	}

	if !tr.End.IsRelative {
		t.Error("end should be marked as relative")
	}
}

func TestDefaultForUseCase(t *testing.T) {
	tests := []struct {
		uc   UseCase
		want time.Duration
	}{
		{UseCaseAlerts, time.Hour},
		{UseCaseLogs, time.Hour},
		{UseCaseMetrics, 24 * time.Hour},
		{UseCaseIncidents, 7 * 24 * time.Hour},
		{UseCaseTickets, 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(string(tt.uc), func(t *testing.T) {
			got := DefaultForUseCase(tt.uc)
			if got != tt.want {
				t.Errorf("DefaultForUseCase(%s) = %v, want %v", tt.uc, got, tt.want)
			}
		})
	}
}

func TestFormatTimeRangeForService(t *testing.T) {
	refTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	tr := TimeRange{
		Start: FromTime(refTime.Add(-time.Hour)),
		End:   FromTime(refTime),
	}

	tests := []struct {
		service   ServiceFormat
		wantStart string
		wantEnd   string
	}{
		{ServicePrometheus, "1705311000", "1705314600"},
		{ServicePylon, "2024-01-15T09:30:00Z", "2024-01-15T10:30:00Z"},
	}

	for _, tt := range tests {
		t.Run(string(tt.service), func(t *testing.T) {
			start, end := FormatTimeRangeForService(tr, tt.service)
			if start != tt.wantStart {
				t.Errorf("start = %q, want %q", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %q, want %q", end, tt.wantEnd)
			}
		})
	}
}
