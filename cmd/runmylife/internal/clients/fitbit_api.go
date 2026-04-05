package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const fitbitAPIBaseURL = "https://api.fitbit.com/1/user/-"

// FitbitClient is a REST client for the Fitbit Web API.
type FitbitClient struct {
	httpClient *http.Client
	token      string
}

// FitbitDailyStats holds daily activity stats.
type FitbitDailyStats struct {
	Date             string `json:"date"`
	Steps            int    `json:"steps"`
	Calories         int    `json:"calories"`
	ActiveMinutes    int    `json:"active_minutes"`
	Distance         float64 `json:"distance"`
	RestingHeartRate int    `json:"resting_heart_rate"`
}

// FitbitSleep holds sleep data for a night.
type FitbitSleep struct {
	ID         string `json:"id"`
	Date       string `json:"date"`
	DurationMs int    `json:"duration_ms"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	Efficiency int    `json:"efficiency"`
}

// FitbitActivity holds a logged activity.
type FitbitActivity struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	DurationMs int     `json:"duration_ms"`
	Calories   int     `json:"calories"`
	Distance   float64 `json:"distance"`
	StartTime  string  `json:"start_time"`
}

// NewFitbitClient creates a new Fitbit API client.
func NewFitbitClient(token string) *FitbitClient {
	return &FitbitClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// TodayStats returns today's activity summary.
func (f *FitbitClient) TodayStats(ctx context.Context) (*FitbitDailyStats, error) {
	return f.DailyStats(ctx, time.Now().Format("2006-01-02"))
}

// DailyStats returns activity stats for a specific date.
func (f *FitbitClient) DailyStats(ctx context.Context, date string) (*FitbitDailyStats, error) {
	var raw struct {
		Summary struct {
			Steps               int     `json:"steps"`
			CaloriesOut         int     `json:"caloriesOut"`
			VeryActiveMinutes   int     `json:"veryActiveMinutes"`
			FairlyActiveMinutes int     `json:"fairlyActiveMinutes"`
			Distances           []struct {
				Activity string  `json:"activity"`
				Distance float64 `json:"distance"`
			} `json:"distances"`
			RestingHeartRate int `json:"restingHeartRate"`
		} `json:"summary"`
	}
	if err := f.doGet(ctx, fmt.Sprintf("/activities/date/%s.json", date), &raw); err != nil {
		return nil, err
	}
	var dist float64
	for _, d := range raw.Summary.Distances {
		if d.Activity == "total" {
			dist = d.Distance
			break
		}
	}
	return &FitbitDailyStats{
		Date: date, Steps: raw.Summary.Steps, Calories: raw.Summary.CaloriesOut,
		ActiveMinutes: raw.Summary.VeryActiveMinutes + raw.Summary.FairlyActiveMinutes,
		Distance: dist, RestingHeartRate: raw.Summary.RestingHeartRate,
	}, nil
}

// SleepLog returns sleep data for a date.
func (f *FitbitClient) SleepLog(ctx context.Context, date string) ([]FitbitSleep, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	var raw struct {
		Sleep []struct {
			LogID      int64  `json:"logId"`
			DateOfSleep string `json:"dateOfSleep"`
			Duration   int    `json:"duration"`
			StartTime  string `json:"startTime"`
			EndTime    string `json:"endTime"`
			Efficiency int    `json:"efficiency"`
		} `json:"sleep"`
	}
	if err := f.doGet(ctx, fmt.Sprintf("/sleep/date/%s.json", date), &raw); err != nil {
		return nil, err
	}
	var sleeps []FitbitSleep
	for _, s := range raw.Sleep {
		sleeps = append(sleeps, FitbitSleep{
			ID: fmt.Sprintf("%d", s.LogID), Date: s.DateOfSleep, DurationMs: s.Duration,
			StartTime: s.StartTime, EndTime: s.EndTime, Efficiency: s.Efficiency,
		})
	}
	return sleeps, nil
}

// RecentActivities returns recent activity logs.
func (f *FitbitClient) RecentActivities(ctx context.Context, limit int) ([]FitbitActivity, error) {
	if limit <= 0 {
		limit = 10
	}
	var raw struct {
		Activities []struct {
			LogID          int64   `json:"logId"`
			ActivityName   string  `json:"activityName"`
			Duration       int     `json:"activeDuration"`
			Calories       int     `json:"calories"`
			Distance       float64 `json:"distance"`
			StartTime      string  `json:"startTime"`
			OriginalStartTime string `json:"originalStartTime"`
		} `json:"activities"`
	}
	if err := f.doGet(ctx, fmt.Sprintf("/activities/list.json?beforeDate=%s&sort=desc&limit=%d&offset=0",
		time.Now().AddDate(0, 0, 1).Format("2006-01-02"), limit), &raw); err != nil {
		return nil, err
	}
	var activities []FitbitActivity
	for _, a := range raw.Activities {
		activities = append(activities, FitbitActivity{
			ID: fmt.Sprintf("%d", a.LogID), Type: a.ActivityName, DurationMs: a.Duration,
			Calories: a.Calories, Distance: a.Distance, StartTime: a.OriginalStartTime,
		})
	}
	return activities, nil
}

func (f *FitbitClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fitbitAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fitbit API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "fitbit"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
