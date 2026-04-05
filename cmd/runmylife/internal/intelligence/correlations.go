package intelligence

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

// Correlation describes a discovered cross-module pattern.
type Correlation struct {
	Type       string  `json:"type"`
	Pattern    string  `json:"pattern"`
	Confidence float64 `json:"confidence"`
	Insight    string  `json:"insight"`
}

// AnalyzeMoodSleep examines the relationship between sleep hours and mood
// over the given number of days using a simple rolling comparison.
func AnalyzeMoodSleep(ctx context.Context, db *sql.DB, days int) []Correlation {
	rows, err := db.QueryContext(ctx,
		`SELECT m.date, AVG(m.mood_score) AS avg_mood, m.sleep_hours
		 FROM mood_log m
		 WHERE m.date >= date('now', ? || ' days')
		   AND m.sleep_hours > 0
		 GROUP BY m.date
		 ORDER BY m.date`,
		fmt.Sprintf("-%d", days))
	if err != nil {
		return nil
	}
	defer rows.Close()

	var lowSleepMoods, highSleepMoods []float64
	var totalLowSleep, totalHighSleep float64
	var nLow, nHigh int

	for rows.Next() {
		var date string
		var avgMood, sleepHours float64
		if rows.Scan(&date, &avgMood, &sleepHours) != nil {
			continue
		}
		if sleepHours < 6 {
			lowSleepMoods = append(lowSleepMoods, avgMood)
			totalLowSleep += avgMood
			nLow++
		} else if sleepHours >= 7 {
			highSleepMoods = append(highSleepMoods, avgMood)
			totalHighSleep += avgMood
			nHigh++
		}
	}

	var correlations []Correlation

	if nLow >= 3 && nHigh >= 3 {
		avgLow := totalLowSleep / float64(nLow)
		avgHigh := totalHighSleep / float64(nHigh)
		diff := avgHigh - avgLow

		if diff > 0.5 {
			confidence := math.Min(diff/3.0, 1.0)
			correlations = append(correlations, Correlation{
				Type:       "mood_sleep",
				Pattern:    fmt.Sprintf("Days with <6h sleep average %.1f mood vs %.1f with 7h+", avgLow, avgHigh),
				Confidence: math.Round(confidence*100) / 100,
				Insight:    fmt.Sprintf("Sleep deficit costs about %.1f mood points. Prioritize sleep on low days.", diff),
			})
		}
	}

	return correlations
}

// AnalyzeFocusEnergy examines whether focus session duration correlates
// with the energy level at the time the session started.
func AnalyzeFocusEnergy(ctx context.Context, db *sql.DB, days int) []Correlation {
	rows, err := db.QueryContext(ctx,
		`SELECT f.category,
		        (julianday(COALESCE(f.ended_at, datetime('now'))) - julianday(f.started_at)) * 1440 AS duration_min,
		        CAST(strftime('%H', f.started_at) AS INTEGER) AS hour
		 FROM focus_sessions f
		 WHERE f.started_at >= date('now', ? || ' days')
		   AND f.category != ''
		 ORDER BY f.started_at`,
		fmt.Sprintf("-%d", days))
	if err != nil {
		return nil
	}
	defer rows.Close()

	var morningDurations, afternoonDurations []float64

	for rows.Next() {
		var category string
		var durationMin float64
		var hour int
		if rows.Scan(&category, &durationMin, &hour) != nil {
			continue
		}
		if durationMin < 5 || durationMin > 480 {
			continue // skip noise
		}
		if hour >= 6 && hour < 12 {
			morningDurations = append(morningDurations, durationMin)
		} else if hour >= 12 && hour < 18 {
			afternoonDurations = append(afternoonDurations, durationMin)
		}
	}

	var correlations []Correlation

	if len(morningDurations) >= 3 && len(afternoonDurations) >= 3 {
		avgMorning := mean(morningDurations)
		avgAfternoon := mean(afternoonDurations)
		diff := avgMorning - avgAfternoon
		pctDiff := diff / avgAfternoon * 100

		if math.Abs(pctDiff) > 15 {
			better := "morning"
			if diff < 0 {
				better = "afternoon"
			}
			confidence := math.Min(math.Abs(pctDiff)/50.0, 1.0)
			correlations = append(correlations, Correlation{
				Type:       "focus_energy",
				Pattern:    fmt.Sprintf("Morning avg %.0fmin vs afternoon avg %.0fmin", avgMorning, avgAfternoon),
				Confidence: math.Round(confidence*100) / 100,
				Insight:    fmt.Sprintf("Focus sessions are %.0f%% longer in the %s. Schedule deep work accordingly.", math.Abs(pctDiff), better),
			})
		}
	}

	return correlations
}

// AnalyzeExerciseMood examines whether exercise on a given day correlates
// with improved mood on the following day.
func AnalyzeExerciseMood(ctx context.Context, db *sql.DB, days int) []Correlation {
	rows, err := db.QueryContext(ctx,
		`SELECT m1.date,
		        m1.exercise_done,
		        (SELECT AVG(m2.mood_score) FROM mood_log m2
		         WHERE m2.date = date(m1.date, '+1 day')) AS next_day_mood
		 FROM mood_log m1
		 WHERE m1.date >= date('now', ? || ' days')
		   AND m1.date < date('now')
		 GROUP BY m1.date`,
		fmt.Sprintf("-%d", days))
	if err != nil {
		return nil
	}
	defer rows.Close()

	var exerciseDayMoods, noExerciseDayMoods []float64

	for rows.Next() {
		var date string
		var exerciseDone int
		var nextDayMood sql.NullFloat64
		if rows.Scan(&date, &exerciseDone, &nextDayMood) != nil {
			continue
		}
		if !nextDayMood.Valid {
			continue
		}
		if exerciseDone == 1 {
			exerciseDayMoods = append(exerciseDayMoods, nextDayMood.Float64)
		} else {
			noExerciseDayMoods = append(noExerciseDayMoods, nextDayMood.Float64)
		}
	}

	var correlations []Correlation

	if len(exerciseDayMoods) >= 3 && len(noExerciseDayMoods) >= 3 {
		avgExercise := mean(exerciseDayMoods)
		avgNoExercise := mean(noExerciseDayMoods)
		diff := avgExercise - avgNoExercise

		if diff > 0.3 {
			confidence := math.Min(diff/2.0, 1.0)
			correlations = append(correlations, Correlation{
				Type:       "exercise_mood",
				Pattern:    fmt.Sprintf("Next-day mood after exercise: %.1f vs %.1f without", avgExercise, avgNoExercise),
				Confidence: math.Round(confidence*100) / 100,
				Insight:    fmt.Sprintf("Exercise correlates with +%.1f mood points the next day.", diff),
			})
		}
	}

	return correlations
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
