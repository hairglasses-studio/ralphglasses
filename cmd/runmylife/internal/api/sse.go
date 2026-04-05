package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/hairglasses-studio/runmylife/internal/events"
)

func handleSSE(bus *events.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		// Channel to receive events from the bus
		ch := make(chan events.Event, 64)

		// Optional type filter from query param
		typeFilter := events.EventType(r.URL.Query().Get("type"))

		// Subscribe to all events, filter in the handler
		bus.SubscribeAll(func(e events.Event) {
			if typeFilter != "" && e.Type != typeFilter {
				return
			}
			select {
			case ch <- e:
			default:
				// Drop if client is too slow
			}
		})

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-ch:
				data, err := json.Marshal(e)
				if err != nil {
					log.Printf("[sse] marshal error: %v", err)
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
				flusher.Flush()
			}
		}
	}
}

func handleAPIHealth(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		if err := db.PingContext(context.Background()); err != nil {
			status = "db_error"
		}

		var tableCount int
		db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount)

		WriteJSON(w, http.StatusOK, map[string]any{
			"status": status,
			"tables": tableCount,
		})
	}
}
