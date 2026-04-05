// Package api provides REST API endpoints for the runmylife dashboard,
// finances, wellness, and ADHD data. Mounted on the existing MCP HTTP mux.
package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// Response is the standard JSON envelope for all API responses.
type Response struct {
	Data  any            `json:"data"`
	Error *string        `json:"error"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// WriteJSON sends a successful JSON response.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := Response{Data: data}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[api] encode error: %v", err)
	}
}

// WriteJSONMeta sends a successful JSON response with metadata.
func WriteJSONMeta(w http.ResponseWriter, status int, data any, meta map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := Response{Data: data, Meta: meta}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[api] encode error: %v", err)
	}
}

// WriteError sends an error JSON response.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := Response{Error: &msg}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[api] encode error: %v", err)
	}
}
