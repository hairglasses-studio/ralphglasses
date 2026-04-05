package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const openMeteoBaseURL = "https://api.open-meteo.com/v1/forecast"

type WeatherClient struct {
	httpClient *http.Client
	latitude   float64
	longitude  float64
}

func NewWeatherClient(lat, lon float64) *WeatherClient {
	return &WeatherClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		latitude:   lat,
		longitude:  lon,
	}
}

type CurrentWeather struct {
	Temperature   float64 `json:"temperature_2m"`
	Humidity      float64 `json:"relative_humidity_2m"`
	WindSpeed     float64 `json:"wind_speed_10m"`
	WeatherCode   int     `json:"weather_code"`
	Precipitation float64 `json:"precipitation"`
}

type DailyForecast struct {
	Dates         []string  `json:"time"`
	TempMax       []float64 `json:"temperature_2m_max"`
	TempMin       []float64 `json:"temperature_2m_min"`
	Precipitation []float64 `json:"precipitation_sum"`
	WeatherCodes  []int     `json:"weather_code"`
}

type HourlyForecast struct {
	Times         []string  `json:"time"`
	Temps         []float64 `json:"temperature_2m"`
	Precipitation []float64 `json:"precipitation"`
	Humidity      []float64 `json:"relative_humidity_2m"`
	WeatherCodes  []int     `json:"weather_code"`
}

func (w *WeatherClient) GetCurrent(ctx context.Context) (*CurrentWeather, error) {
	params := url.Values{
		"latitude":  {fmt.Sprintf("%.4f", w.latitude)},
		"longitude": {fmt.Sprintf("%.4f", w.longitude)},
		"current":   {"temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code,precipitation"},
	}
	var result struct {
		Current CurrentWeather `json:"current"`
	}
	if err := w.doGet(ctx, params, &result); err != nil {
		return nil, err
	}
	return &result.Current, nil
}

func (w *WeatherClient) GetDailyForecast(ctx context.Context, days int) (*DailyForecast, error) {
	if days <= 0 || days > 16 {
		days = 7
	}
	params := url.Values{
		"latitude":      {fmt.Sprintf("%.4f", w.latitude)},
		"longitude":     {fmt.Sprintf("%.4f", w.longitude)},
		"daily":         {"temperature_2m_max,temperature_2m_min,precipitation_sum,weather_code"},
		"forecast_days": {fmt.Sprintf("%d", days)},
	}
	var result struct {
		Daily DailyForecast `json:"daily"`
	}
	if err := w.doGet(ctx, params, &result); err != nil {
		return nil, err
	}
	return &result.Daily, nil
}

func (w *WeatherClient) GetHourlyForecast(ctx context.Context, hours int) (*HourlyForecast, error) {
	if hours <= 0 || hours > 48 {
		hours = 24
	}
	params := url.Values{
		"latitude":       {fmt.Sprintf("%.4f", w.latitude)},
		"longitude":      {fmt.Sprintf("%.4f", w.longitude)},
		"hourly":         {"temperature_2m,precipitation,relative_humidity_2m,weather_code"},
		"forecast_hours": {fmt.Sprintf("%d", hours)},
	}
	var result struct {
		Hourly HourlyForecast `json:"hourly"`
	}
	if err := w.doGet(ctx, params, &result); err != nil {
		return nil, err
	}
	return &result.Hourly, nil
}

func (w *WeatherClient) doGet(ctx context.Context, params url.Values, out interface{}) error {
	reqURL := openMeteoBaseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("weather API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weather API error %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// WeatherCodeDescription returns a human-readable weather description.
func WeatherCodeDescription(code int) string {
	switch {
	case code == 0:
		return "Clear sky"
	case code <= 3:
		return "Partly cloudy"
	case code <= 49:
		return "Fog"
	case code <= 59:
		return "Drizzle"
	case code <= 69:
		return "Rain"
	case code <= 79:
		return "Snow"
	case code <= 84:
		return "Rain showers"
	case code <= 86:
		return "Snow showers"
	case code <= 99:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}
