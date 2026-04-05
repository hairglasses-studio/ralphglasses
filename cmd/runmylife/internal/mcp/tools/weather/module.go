// Package weather provides MCP tools for weather information via Open-Meteo.
package weather

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for weather information.
type Module struct{}

func (m *Module) Name() string        { return "weather" }
func (m *Module) Description() string { return "Weather information via Open-Meteo API" }

var weatherHints = map[string]string{
	"current/now":     "Get current weather conditions",
	"forecast/daily":  "Get daily forecast (up to 16 days)",
	"forecast/hourly": "Get hourly forecast (up to 48 hours)",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("weather").
		Domain("current", common.ActionRegistry{
			"now": handleCurrentNow,
		}).
		Domain("forecast", common.ActionRegistry{
			"daily":  handleForecastDaily,
			"hourly": handleForecastHourly,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_weather",
				mcp.WithDescription(
					"Weather gateway. Current conditions and forecasts via Open-Meteo.\n\n"+
						dispatcher.DescribeActionsWithHints(weatherHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: current, forecast")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithNumber("days", mcp.Description("Forecast days (1-16, default 7)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "weather",
			Subcategory:         "gateway",
			Tags:                []string{"weather", "forecast", "location"},
			Complexity:          tools.ComplexitySimple,
			IsWrite:             false,
			CircuitBreakerGroup: "weather_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// loadWeatherClient creates a WeatherClient from config location settings.
func loadWeatherClient() (*clients.WeatherClient, *config.Location, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	if cfg.Location == nil {
		return nil, nil, fmt.Errorf("location not configured")
	}
	loc := cfg.Location
	client := clients.NewWeatherClient(loc.Latitude, loc.Longitude)
	return client, loc, nil
}

func handleCurrentNow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, loc, err := loadWeatherClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Configure location in ~/.config/runmylife/config.json",
			"Set latitude, longitude, city, and timezone"), nil
	}

	current, err := client.GetCurrent(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	md := common.NewMarkdownBuilder().Title("Current Weather")
	md.Bold("Location", loc.City)
	md.Bold("Temperature", fmt.Sprintf("%.1f\u00b0C", current.Temperature))
	md.Bold("Humidity", fmt.Sprintf("%.0f%%", current.Humidity))
	md.Bold("Wind Speed", fmt.Sprintf("%.1f km/h", current.WindSpeed))
	md.Bold("Conditions", clients.WeatherCodeDescription(current.WeatherCode))
	md.Bold("Precipitation", fmt.Sprintf("%.1f mm", current.Precipitation))

	return tools.TextResult(md.String()), nil
}

func handleForecastDaily(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, loc, err := loadWeatherClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Configure location in ~/.config/runmylife/config.json",
			"Set latitude, longitude, city, and timezone"), nil
	}

	days := common.GetIntParam(req, "days", 7)
	forecast, err := client.GetDailyForecast(ctx, days)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Daily Forecast — %s", loc.City))

	headers := []string{"Date", "High", "Low", "Precip", "Conditions"}
	var rows [][]string
	for i := range forecast.Dates {
		if i >= len(forecast.TempMax) || i >= len(forecast.TempMin) ||
			i >= len(forecast.Precipitation) || i >= len(forecast.WeatherCodes) {
			break
		}
		rows = append(rows, []string{
			forecast.Dates[i],
			fmt.Sprintf("%.1f\u00b0C", forecast.TempMax[i]),
			fmt.Sprintf("%.1f\u00b0C", forecast.TempMin[i]),
			fmt.Sprintf("%.1f mm", forecast.Precipitation[i]),
			clients.WeatherCodeDescription(forecast.WeatherCodes[i]),
		})
	}

	if len(rows) == 0 {
		md.EmptyList("forecast data")
	} else {
		md.Table(headers, rows)
	}

	return tools.TextResult(md.String()), nil
}

func handleForecastHourly(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, loc, err := loadWeatherClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Configure location in ~/.config/runmylife/config.json",
			"Set latitude, longitude, city, and timezone"), nil
	}

	forecast, err := client.GetHourlyForecast(ctx, 24)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Hourly Forecast — %s", loc.City))

	headers := []string{"Time", "Temp", "Precip", "Humidity", "Conditions"}
	var rows [][]string
	maxRows := 24
	for i := range forecast.Times {
		if i >= maxRows {
			break
		}
		if i >= len(forecast.Temps) || i >= len(forecast.Precipitation) ||
			i >= len(forecast.Humidity) || i >= len(forecast.WeatherCodes) {
			break
		}
		rows = append(rows, []string{
			forecast.Times[i],
			fmt.Sprintf("%.1f\u00b0C", forecast.Temps[i]),
			fmt.Sprintf("%.1f mm", forecast.Precipitation[i]),
			fmt.Sprintf("%.0f%%", forecast.Humidity[i]),
			clients.WeatherCodeDescription(forecast.WeatherCodes[i]),
		})
	}

	if len(rows) == 0 {
		md.EmptyList("hourly forecast data")
	} else {
		md.Table(headers, rows)
	}

	return tools.TextResult(md.String()), nil
}
