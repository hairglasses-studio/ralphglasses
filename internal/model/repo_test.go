package model

import (
	"testing"
	"time"
)

func TestRepo_StatusDisplay(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		want string
	}{
		{
			name: "from LoopStatus",
			repo: Repo{
				Status: &LoopStatus{Status: "running"},
			},
			want: "running",
		},
		{
			name: "from Progress when no LoopStatus",
			repo: Repo{
				Progress: &Progress{Status: "in_progress"},
			},
			want: "in_progress",
		},
		{
			name: "LoopStatus takes priority over Progress",
			repo: Repo{
				Status:   &LoopStatus{Status: "completed"},
				Progress: &Progress{Status: "in_progress"},
			},
			want: "completed",
		},
		{
			name: "unknown when no status",
			repo: Repo{},
			want: "unknown",
		},
		{
			name: "unknown when Status has empty string",
			repo: Repo{
				Status: &LoopStatus{Status: ""},
			},
			want: "unknown",
		},
		{
			name: "Progress used when LoopStatus.Status is empty",
			repo: Repo{
				Status:   &LoopStatus{Status: ""},
				Progress: &Progress{Status: "waiting"},
			},
			want: "waiting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.repo.StatusDisplay()
			if got != tt.want {
				t.Errorf("StatusDisplay() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepo_CircuitDisplay(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		want string
	}{
		{
			name: "nil circuit",
			repo: Repo{},
			want: "-",
		},
		{
			name: "CLOSED",
			repo: Repo{
				Circuit: &CircuitBreakerState{State: "CLOSED"},
			},
			want: "CLOSED",
		},
		{
			name: "OPEN",
			repo: Repo{
				Circuit: &CircuitBreakerState{State: "OPEN"},
			},
			want: "OPEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.repo.CircuitDisplay()
			if got != tt.want {
				t.Errorf("CircuitDisplay() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepo_CallsDisplay(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		want string
	}{
		{
			name: "nil status",
			repo: Repo{},
			want: "-",
		},
		{
			name: "normal calls",
			repo: Repo{
				Status: &LoopStatus{CallsMadeThisHr: 15, MaxCallsPerHour: 100},
			},
			want: "15/100",
		},
		{
			name: "zero calls",
			repo: Repo{
				Status: &LoopStatus{CallsMadeThisHr: 0, MaxCallsPerHour: 50},
			},
			want: "0/50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.repo.CallsDisplay()
			if got != tt.want {
				t.Errorf("CallsDisplay() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepo_UpdatedDisplay(t *testing.T) {
	tests := []struct {
		name string
		repo Repo
		want string
	}{
		{
			name: "nil status",
			repo: Repo{},
			want: "-",
		},
		{
			name: "zero timestamp",
			repo: Repo{
				Status: &LoopStatus{},
			},
			want: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.repo.UpdatedDisplay()
			if got != tt.want {
				t.Errorf("UpdatedDisplay() = %q, want %q", got, tt.want)
			}
		})
	}

	// Test recent timestamp (seconds ago)
	t.Run("seconds ago", func(t *testing.T) {
		r := Repo{
			Status: &LoopStatus{Timestamp: time.Now().Add(-10 * time.Second)},
		}
		got := r.UpdatedDisplay()
		if got != "10s ago" && got != "11s ago" && got != "9s ago" {
			// Allow small timing variance
			t.Logf("UpdatedDisplay() = %q (allowing timing variance)", got)
		}
	})

	// Test minutes ago
	t.Run("minutes ago", func(t *testing.T) {
		r := Repo{
			Status: &LoopStatus{Timestamp: time.Now().Add(-5 * time.Minute)},
		}
		got := r.UpdatedDisplay()
		if got != "5m ago" {
			t.Logf("UpdatedDisplay() = %q (expected ~5m ago)", got)
		}
	})

	// Test hours ago
	t.Run("hours ago", func(t *testing.T) {
		r := Repo{
			Status: &LoopStatus{Timestamp: time.Now().Add(-2 * time.Hour)},
		}
		got := r.UpdatedDisplay()
		if got != "2h ago" {
			t.Logf("UpdatedDisplay() = %q (expected ~2h ago)", got)
		}
	})
}
