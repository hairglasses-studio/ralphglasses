package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/tui/components"
)

type wellnessData struct {
	MoodScore       int
	MoodNotes       string
	EnergyLevel     int
	FocusSessions   int
	FocusMinutes    int
	HabitsCompleted int
	HabitsTotal     int
	WeekMoods       []dayMood
	FocusByDay      []dayFocus
	HabitGrid       habitGrid
}

type dayMood struct {
	Date string
	Avg  float64
}

type dayFocus struct {
	Date    string
	Minutes float64
}

type habitGrid struct {
	Names    []string
	Days     []string
	Grid     [][]int // [habit][day] = 0/1
}

func loadWellnessData(db *sql.DB) wellnessData {
	ctx := context.Background()
	now := time.Now()
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	weekAgo := now.AddDate(0, 0, -7).Format("2006-01-02")

	d := wellnessData{}

	// Today's mood
	var moodNotes sql.NullString
	db.QueryRowContext(ctx,
		`SELECT score, notes FROM mood_log WHERE date(logged_at) = ?
		 ORDER BY logged_at DESC LIMIT 1`, today,
	).Scan(&d.MoodScore, &moodNotes)
	if moodNotes.Valid {
		d.MoodNotes = moodNotes.String
	}

	// Energy from curve
	curve := adhd.BuildEnergyCurve(ctx, db, today)
	if curve != nil {
		d.EnergyLevel = adhd.CurrentEnergyFromCurve(curve)
	}

	// Focus stats
	stats, err := adhd.GetFocusStats(ctx, db, today, tomorrow)
	if err == nil && stats != nil {
		d.FocusSessions = stats.TotalSessions
		d.FocusMinutes = stats.TotalMinutes
	}

	// Habits
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM habit_completions WHERE date(completed_at) = ?`, today,
	).Scan(&d.HabitsCompleted)
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM habits WHERE archived = 0`,
	).Scan(&d.HabitsTotal)

	// Week mood trend
	rows, err := db.QueryContext(ctx,
		`SELECT date(logged_at), AVG(score) FROM mood_log
		 WHERE date(logged_at) >= ? GROUP BY date(logged_at) ORDER BY date(logged_at)`, weekAgo)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dm dayMood
			if rows.Scan(&dm.Date, &dm.Avg) == nil {
				d.WeekMoods = append(d.WeekMoods, dm)
			}
		}
	}

	// Focus by day (7 days)
	focusRows, err := db.QueryContext(ctx,
		`SELECT date(started_at),
		        SUM((julianday(COALESCE(ended_at, datetime('now'))) - julianday(started_at)) * 1440)
		 FROM focus_sessions
		 WHERE started_at >= ?
		 GROUP BY date(started_at)
		 ORDER BY date(started_at)`, weekAgo)
	if err == nil {
		defer focusRows.Close()
		for focusRows.Next() {
			var df dayFocus
			if focusRows.Scan(&df.Date, &df.Minutes) == nil {
				d.FocusByDay = append(d.FocusByDay, df)
			}
		}
	}

	// Habit heatmap data
	habitRows, err := db.QueryContext(ctx,
		`SELECT id, name FROM habits WHERE archived = 0 ORDER BY name LIMIT 8`)
	if err == nil {
		defer habitRows.Close()
		type hab struct{ id, name string }
		var habits []hab
		for habitRows.Next() {
			var h hab
			if habitRows.Scan(&h.id, &h.name) == nil {
				habits = append(habits, h)
			}
		}
		if len(habits) > 0 {
			d.HabitGrid.Names = make([]string, len(habits))
			for i, h := range habits {
				d.HabitGrid.Names[i] = h.name
			}
			d.HabitGrid.Grid = make([][]int, len(habits))
			for i := 6; i >= 0; i-- {
				day := now.AddDate(0, 0, -i)
				dayStr := day.Format("2006-01-02")
				d.HabitGrid.Days = append(d.HabitGrid.Days, day.Format("Mo"))
				for hi, h := range habits {
					if d.HabitGrid.Grid[hi] == nil {
						d.HabitGrid.Grid[hi] = make([]int, 0, 7)
					}
					var count int
					db.QueryRowContext(ctx,
						`SELECT COUNT(*) FROM habit_completions
						 WHERE habit_id = ? AND date(completed_at) = ?`,
						h.id, dayStr).Scan(&count)
					if count > 0 {
						d.HabitGrid.Grid[hi] = append(d.HabitGrid.Grid[hi], 1)
					} else {
						d.HabitGrid.Grid[hi] = append(d.HabitGrid.Grid[hi], 0)
					}
				}
			}
		}
	}

	return d
}

func renderWellness(d wellnessData, width int) string {
	var b strings.Builder
	gaugeWidth := 20

	// Mood gauge
	mood := components.StyledIcon(components.IconMood, colorAccent) + subtitleStyle.Render("Mood") + "\n"
	if d.MoodScore > 0 {
		mood += "  " + components.InvertedGauge(float64(d.MoodScore), 10, gaugeWidth,
			components.WithLabel(fmt.Sprintf("%d/10", d.MoodScore)))
		if d.MoodNotes != "" {
			mood += "\n  " + mutedStyle.Render(d.MoodNotes)
		}
	} else {
		mood += mutedStyle.Render("  Not logged yet today")
	}
	b.WriteString(cardStyle.Width(width).Render(mood))
	b.WriteString("\n")

	// Energy gauge (inverted: low=bad)
	energy := components.StyledIcon(components.IconBolt, colorWarning) + subtitleStyle.Render("Energy") + "\n"
	if d.EnergyLevel > 0 {
		energy += "  " + components.InvertedGauge(float64(d.EnergyLevel), 10, gaugeWidth,
			components.WithLabel(fmt.Sprintf("%d/10", d.EnergyLevel)))
	} else {
		energy += mutedStyle.Render("  No data")
	}
	b.WriteString(cardStyle.Width(width).Render(energy))
	b.WriteString("\n")

	// Focus + Habits side by side
	focus := components.StyledIcon(components.IconFocus, colorSeries1) + subtitleStyle.Render("Focus") + "\n"
	focus += fmt.Sprintf("  %d sessions, %d minutes", d.FocusSessions, d.FocusMinutes)

	habits := "\n" + components.StyledIcon(components.IconRepeat, colorSuccess) + subtitleStyle.Render("Habits") + "\n"
	if d.HabitsTotal > 0 {
		habits += "  " + components.Gauge(float64(d.HabitsCompleted), float64(d.HabitsTotal), gaugeWidth,
			components.WithLabel(fmt.Sprintf("%d/%d", d.HabitsCompleted, d.HabitsTotal)),
			components.WithThresholds(1.1, 1.1), // never warn — more is better
			components.WithGradient(colorWarning, colorSuccess, colorSuccess),
		)
	} else {
		habits += mutedStyle.Render("  No active habits")
	}
	b.WriteString(cardStyle.Width(width).Render(focus + habits))
	b.WriteString("\n")

	// Focus by day bar chart
	if len(d.FocusByDay) > 2 {
		chart := subtitleStyle.Render("Focus Minutes (7 Days)") + "\n"
		var labels []string
		var values []float64
		for _, df := range d.FocusByDay {
			if len(df.Date) >= 10 {
				labels = append(labels, df.Date[8:10])
			} else {
				labels = append(labels, df.Date)
			}
			values = append(values, df.Minutes)
		}
		chartWidth := width - 8
		if chartWidth > 50 {
			chartWidth = 50
		}
		chart += components.BarChart(labels, values, chartWidth, 6, colorSeries1)
		b.WriteString(cardStyle.Width(width).Render(chart))
		b.WriteString("\n")
	}

	// Mood sparkline
	if len(d.WeekMoods) > 2 {
		spark := subtitleStyle.Render("7-Day Mood") + "\n"
		var moodVals []float64
		var moodLabels []string
		for _, dm := range d.WeekMoods {
			moodVals = append(moodVals, dm.Avg)
			if len(dm.Date) >= 10 {
				moodLabels = append(moodLabels, dm.Date[8:10])
			}
		}
		sparkWidth := width - 8
		if sparkWidth > 40 {
			sparkWidth = 40
		}
		spark += components.Sparkline(moodVals, sparkWidth, colorAccent)
		spark += "\n  "
		for _, l := range moodLabels {
			spark += mutedStyle.Render(fmt.Sprintf("%-2s", l))
		}
		b.WriteString(cardStyle.Width(width).Render(spark))
		b.WriteString("\n")
	}

	// Habit heatmap
	if len(d.HabitGrid.Names) > 0 {
		heatmap := subtitleStyle.Render("Habit Completions") + "\n"
		heatmap += components.HabitHeatmap(d.HabitGrid.Names, d.HabitGrid.Days, d.HabitGrid.Grid)
		b.WriteString(cardStyle.Width(width).Render(heatmap))
	}

	return b.String()
}
