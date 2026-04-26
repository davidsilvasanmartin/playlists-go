package httputil

import (
	"encoding/json"
	"net/http"
	"time"
)

// APIError is the error format used by the API
type APIError struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Path      string `json:"path"`
}

// WriteJSON writes a JSON response to the given writer. status is the HTTP status code to use
// for the response. v is the value to encode as JSON.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes an error response to the given writer
func WriteError(w http.ResponseWriter, r *http.Request, status int, message string) {
	WriteJSON(w, status, APIError{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    status,
		Error:     http.StatusText(status),
		Message:   message,
		Path:      r.URL.Path,
	})
}
