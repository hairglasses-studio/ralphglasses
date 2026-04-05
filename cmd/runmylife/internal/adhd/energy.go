package adhd

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// EnergyCurvePoint represents one hour's energy level.
type EnergyCurvePoint struct {
	Hour   int
	Level  int
	Source string
}

// DailyEnergyCurve represents a full day's energy profile.
type DailyEnergyCurve struct {
	Date   string
	Points []EnergyCurvePoint
	Peak   int // hour of peak energy
	Trough int // hour of lowest energy
}

// RecordEnergyPoint stores an energy reading for the current hour.
func RecordEnergyPoint(ctx context.Context, db *sql.DB, level int, source string) error {
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	date := time.Now().Format("2006-01-02")
	hour := time.Now().Hour()

	_, err := db.ExecContext(ctx,
		`INSERT INTO daily_energy_curve (date, hour, energy_level, source)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(date, hour) DO UPDATE SET energy_level = ?, source = ?`,
		date, hour, level, source, level, source,
	)
	return err
}

// BuildEnergyCurve constructs a day's energy curve from stored data + defaults.
func BuildEnergyCurve(ctx context.Context, db *sql.DB, date string) *DailyEnergyCurve {
	curve := &DailyEnergyCurve{Date: date}

	// Fill with defaults first
	for h := 0; h < 24; h++ {
		curve.Points = append(curve.Points, EnergyCurvePoint{
			Hour:   h,
			Level:  timeBasedEnergy(h),
			Source: "default",
		})
	}

	// Override with actual data
	rows, err := db.QueryContext(ctx,
		"SELECT hour, energy_level, source FROM daily_energy_curve WHERE date = ? ORDER BY hour",
		date)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var hour, level int
			var source string
			if rows.Scan(&hour, &level, &source) == nil && hour >= 0 && hour < 24 {
				curve.Points[hour] = EnergyCurvePoint{Hour: hour, Level: level, Source: source}
			}
		}
	}

	// Find peak and trough
	peakLevel := 0
	troughLevel := 11
	for _, p := range curve.Points {
		if p.Level > peakLevel {
			peakLevel = p.Level
			curve.Peak = p.Hour
		}
		if p.Level < troughLevel {
			troughLevel = p.Level
			curve.Trough = p.Hour
		}
	}

	return curve
}

// CurrentEnergyFromCurve returns the energy level for the current hour from the curve.
func CurrentEnergyFromCurve(curve *DailyEnergyCurve) int {
	hour := time.Now().Hour()
	if hour >= 0 && hour < len(curve.Points) {
		return curve.Points[hour].Level
	}
	return 5
}

// MatchTaskToEnergy returns whether a task's energy requirement matches current energy.
type EnergyMatch struct {
	TaskID     string
	TaskTitle  string
	Required   string // low/medium/high
	Current    int    // 1-10
	IsMatch    bool
	Suggestion string
}

// MatchTasksToEnergy filters tasks that match the current energy level.
func MatchTasksToEnergy(ctx context.Context, db *sql.DB) ([]EnergyMatch, error) {
	energy := EstimateCurrentEnergy(ctx, db)

	rows, err := db.QueryContext(ctx,
		`SELECT t.id, t.title, COALESCE(tm.energy_required, 'medium')
		 FROM tasks t
		 LEFT JOIN task_metadata tm ON t.id = tm.task_id
		 WHERE t.completed = 0
		 ORDER BY t.priority DESC, t.due_date ASC
		 LIMIT 10`)
	if err != nil {
		return nil, fmt.Errorf("match tasks to energy: %w", err)
	}
	defer rows.Close()

	var matches []EnergyMatch
	for rows.Next() {
		var id, title, required string
		if rows.Scan(&id, &title, &required) != nil {
			continue
		}

		m := EnergyMatch{
			TaskID:   id,
			TaskTitle: title,
			Required: required,
			Current:  energy.Level,
		}

		switch required {
		case "low":
			m.IsMatch = true // low energy tasks always match
			m.Suggestion = "Good for any energy level"
		case "medium":
			m.IsMatch = energy.Level >= 4
			if !m.IsMatch {
				m.Suggestion = "Save for when energy picks up"
			}
		case "high":
			m.IsMatch = energy.Level >= 7
			if !m.IsMatch {
				m.Suggestion = fmt.Sprintf("Wait for higher energy (currently %d/10)", energy.Level)
			}
		default:
			m.IsMatch = true
		}

		matches = append(matches, m)
	}

	return matches, nil
}

// ForecastRemainingEnergy returns predicted energy levels for the rest of today.
func ForecastRemainingEnergy(ctx context.Context, db *sql.DB) []EnergyCurvePoint {
	curve := BuildEnergyCurve(ctx, db, time.Now().Format("2006-01-02"))
	currentHour := time.Now().Hour()

	var remaining []EnergyCurvePoint
	for _, p := range curve.Points {
		if p.Hour > currentHour {
			remaining = append(remaining, p)
		}
	}
	return remaining
}
