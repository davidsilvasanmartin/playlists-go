package search

import (
	"encoding/json"
	"net/http"
	"time"
)

// Handler handles HTTP requests
type Handler struct {
	service Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{service: svc}
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	artist := r.URL.Query().Get("title")

	if len(title) < 2 {
		writeError(w, r, http.StatusBadRequest, "Field 'title' must be at least 2 characters")
		return
	}
	if len(artist) < 2 {
		writeError(w, r, http.StatusBadRequest, "Field 'artist' must be at least 2 characters")
		return
	}

	results, err := h.service.Search(r.Context(), title, artist)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "MusicBrainz API is currently unreachable. Please try again later")
	}

	writeJSON(w, http.StatusOK, Response{Results: results})
}

// RESPONSE HELPERS
// These can't live in internal/api because internal/api imports internal/search,
// and we don't want to create a circular dependency. We'll move these shared
// utilities somewhere else later such as in an internal/httputil package

type apiError struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Path      string `json:"path"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	writeJSON(w, status, apiError{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    status,
		Error:     http.StatusText(status),
		Message:   message,
		Path:      r.URL.Path,
	})
}
