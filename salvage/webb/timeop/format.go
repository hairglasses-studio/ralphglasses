package timeop

import (
	"fmt"
	"time"
)

// FormatPrometheus returns Unix seconds (int64) for Prometheus API.
func FormatPrometheus(ts TimeSpec) int64 {
	return ts.Time.Unix()
}

// FormatGrafana returns Unix milliseconds (int64) for Grafana API/URLs.
func FormatGrafana(ts TimeSpec) int64 {
	return ts.Time.UnixMilli()
}

// FormatRFC3339 returns ISO8601/RFC3339 string for Pylon, incident.io, and similar APIs.
func FormatRFC3339(ts TimeSpec) string {
	return ts.Time.Format(time.RFC3339)
}

// FormatRFC3339Nano returns RFC3339 with nanosecond precision.
func FormatRFC3339Nano(ts TimeSpec) string {
	return ts.Time.Format(time.RFC3339Nano)
}

// FormatSlack returns Slack timestamp format "seconds.microseconds".
// Preserves microsecond precision for thread_ts values.
func FormatSlack(ts TimeSpec) string {
	secs := ts.Time.Unix()
	micros := ts.Time.Nanosecond() / 1000
	return fmt.Sprintf("%d.%06d", secs, micros)
}

// FormatClickHouse returns format suitable for ClickHouse queries.
// If asString is true, returns a DateTime string; otherwise returns Unix timestamp.
func FormatClickHouse(ts TimeSpec, asString bool) string {
	if asString {
		return ts.Time.UTC().Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("%d", ts.Time.Unix())
}

// FormatK8s returns Kubernetes-compatible RFC3339 timestamp.
func FormatK8s(ts TimeSpec) string {
	return ts.Time.UTC().Format(time.RFC3339)
}

// FormatHuman returns human-readable format for display.
// Shows relative age for recent times, absolute for older.
func FormatHuman(ts TimeSpec) string {
	age := time.Since(ts.Time)

	// For times in the future
	if age < 0 {
		return ts.Time.Format("2006-01-02 15:04:05 MST") + " (future)"
	}

	// For very recent times, show relative
	switch {
	case age < time.Minute:
		secs := int(age.Seconds())
		if secs == 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", secs)
	case age < time.Hour:
		mins := int(age.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case age < 24*time.Hour:
		hours := int(age.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case age < 7*24*time.Hour:
		days := int(age.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		// For older times, show the date
		return ts.Time.Format("2006-01-02 15:04:05 MST")
	}
}

// FormatDuration returns a compact duration string like "7d", "2h", "30m".
// Chooses the largest appropriate unit.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "-" + FormatDuration(-d)
	}

	// Handle very small durations
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	// Calculate components
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60

	// Choose the most appropriate format
	switch {
	case days >= 30:
		months := days / 30
		if months == 1 {
			return "1mo"
		}
		return fmt.Sprintf("%dmo", months)
	case days >= 7:
		weeks := days / 7
		if weeks == 1 {
			return "1w"
		}
		return fmt.Sprintf("%dw", weeks)
	case days > 0:
		if days == 1 {
			if hours > 0 {
				return fmt.Sprintf("1d%dh", hours)
			}
			return "1d"
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if hours == 1 {
			if mins > 0 {
				return fmt.Sprintf("1h%dm", mins)
			}
			return "1h"
		}
		return fmt.Sprintf("%dh", hours)
	case mins > 0:
		if mins == 1 {
			if secs > 0 {
				return fmt.Sprintf("1m%ds", secs)
			}
			return "1m"
		}
		return fmt.Sprintf("%dm", mins)
	default:
		if secs == 1 {
			return "1s"
		}
		return fmt.Sprintf("%ds", secs)
	}
}

// FormatDurationLong returns a verbose duration string like "7 days", "2 hours".
func FormatDurationLong(d time.Duration) string {
	if d < 0 {
		return "-" + FormatDurationLong(-d)
	}

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60

	switch {
	case days > 0:
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case hours > 0:
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	case mins > 0:
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	default:
		if secs == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", secs)
	}
}

// FormatAge returns a concise age string like "5m", "2h", "3d" for display.
// Similar to kubectl's age format.
func FormatAge(t time.Time) string {
	return FormatDuration(time.Since(t))
}

// FormatTimeRangeForService returns both start and end formatted for a specific service.
type ServiceFormat string

const (
	ServicePrometheus ServiceFormat = "prometheus"
	ServiceGrafana    ServiceFormat = "grafana"
	ServiceSlack      ServiceFormat = "slack"
	ServicePylon      ServiceFormat = "pylon"
	ServiceIncidentIO ServiceFormat = "incidentio"
	ServiceK8s        ServiceFormat = "k8s"
	ServiceClickHouse ServiceFormat = "clickhouse"
)

// FormatTimeRangeForService returns start and end times formatted for the specified service.
func FormatTimeRangeForService(tr TimeRange, service ServiceFormat) (start, end string) {
	switch service {
	case ServicePrometheus:
		return fmt.Sprintf("%d", FormatPrometheus(tr.Start)), fmt.Sprintf("%d", FormatPrometheus(tr.End))
	case ServiceGrafana:
		return fmt.Sprintf("%d", FormatGrafana(tr.Start)), fmt.Sprintf("%d", FormatGrafana(tr.End))
	case ServiceSlack:
		return FormatSlack(tr.Start), FormatSlack(tr.End)
	case ServicePylon, ServiceIncidentIO, ServiceK8s:
		return FormatRFC3339(tr.Start), FormatRFC3339(tr.End)
	case ServiceClickHouse:
		return FormatClickHouse(tr.Start, true), FormatClickHouse(tr.End, true)
	default:
		return FormatRFC3339(tr.Start), FormatRFC3339(tr.End)
	}
}

// FormatDateOnly returns just the date portion in YYYY-MM-DD format.
func FormatDateOnly(ts TimeSpec) string {
	return ts.Time.Format("2006-01-02")
}

// FormatTimeOnly returns just the time portion in HH:MM:SS format.
func FormatTimeOnly(ts TimeSpec) string {
	return ts.Time.Format("15:04:05")
}

// FormatISO8601 returns ISO8601 format with timezone offset.
func FormatISO8601(ts TimeSpec) string {
	return ts.Time.Format("2006-01-02T15:04:05-07:00")
}
