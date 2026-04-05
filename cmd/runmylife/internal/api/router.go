package api

import (
	"database/sql"
	"net/http"

	"github.com/hairglasses-studio/runmylife/internal/events"
)

// MountRoutes registers all REST API routes on the given mux.
// The mux is shared with the MCP SSE/HTTP transport.
// apiToken enables Bearer auth on write endpoints; empty string disables auth.
func MountRoutes(mux *http.ServeMux, db *sql.DB, bus *events.Bus, emitter *events.Emitter, apiToken string) {
	// Wrap read routes with middleware (no auth)
	wrap := func(h http.HandlerFunc) http.Handler {
		return CORSMiddleware(LoggingMiddleware(h))
	}

	// Wrap write routes with auth + middleware
	authMw := AuthMiddleware(apiToken)
	wrapAuth := func(h http.HandlerFunc) http.Handler {
		return CORSMiddleware(LoggingMiddleware(authMw(h)))
	}

	// Dashboard (read)
	mux.Handle("GET /api/v1/dashboard/today", wrap(handleDashboardToday(db)))
	mux.Handle("GET /api/v1/dashboard/adhd", wrap(handleDashboardADHD(db)))

	// Finances (read)
	mux.Handle("GET /api/v1/finances/summary", wrap(handleFinanceSummary(db)))
	mux.Handle("GET /api/v1/finances/transactions", wrap(handleFinanceTransactions(db)))

	// Wellness (read)
	mux.Handle("GET /api/v1/wellness/today", wrap(handleWellnessToday(db)))
	mux.Handle("GET /api/v1/wellness/week", wrap(handleWellnessWeek(db)))

	// SSE event stream
	if bus != nil {
		mux.Handle("GET /api/v1/sse", wrap(handleSSE(bus)))
	}

	// Health
	mux.Handle("GET /api/v1/health", wrap(handleAPIHealth(db)))

	// Entity read endpoints (no auth)
	mux.Handle("GET /api/v1/tasks", wrap(handleListTasks(db)))
	mux.Handle("GET /api/v1/tasks/{id}", wrap(handleGetTask(db)))
	mux.Handle("GET /api/v1/habits", wrap(handleListHabits(db)))
	mux.Handle("GET /api/v1/habits/{id}", wrap(handleGetHabit(db)))
	mux.Handle("GET /api/v1/mood", wrap(handleListMood(db)))
	mux.Handle("GET /api/v1/focus/sessions", wrap(handleListFocusSessions(db)))
	mux.Handle("GET /api/v1/notifications", wrap(handleListNotifications(db)))

	// Write endpoints (auth-protected)
	wc := &WriteContext{DB: db, Emitter: emitter}
	mux.Handle("POST /api/v1/tasks", wrapAuth(handleCreateTask(wc)))
	mux.Handle("PATCH /api/v1/tasks/{id}/complete", wrapAuth(handleCompleteTask(wc)))
	mux.Handle("PATCH /api/v1/tasks/{id}", wrapAuth(handleUpdateTask(wc)))
	mux.Handle("DELETE /api/v1/tasks/{id}", wrapAuth(handleDeleteTask(wc)))
	mux.Handle("POST /api/v1/mood", wrapAuth(handleLogMood(wc)))
	mux.Handle("POST /api/v1/habits", wrapAuth(handleCreateHabit(wc)))
	mux.Handle("POST /api/v1/habits/{id}/complete", wrapAuth(handleCompleteHabit(wc)))
	mux.Handle("PATCH /api/v1/habits/{id}", wrapAuth(handleUpdateHabit(wc)))
	mux.Handle("POST /api/v1/focus/start", wrapAuth(handleStartFocus(wc)))
	mux.Handle("POST /api/v1/focus/end", wrapAuth(handleEndFocus(wc)))
	mux.Handle("POST /api/v1/jobs", wrapAuth(handleEnqueueJob(wc)))
}
