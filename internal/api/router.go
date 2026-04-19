package api

import (
	"encoding/json"
	"net/http"

	"github.com/davidsilvasanmartin/playlists-go/internal/search"
	"go.uber.org/zap"
)

// NewRouter builds and returns the application's HTTP mux with all routes registered
// and middlewares applied
func NewRouter(logger *zap.Logger, searchHandler *search.Handler, version string) http.Handler {
	type versionResponse struct {
		Version string `json:"version"`
	}
	body, _ := json.Marshal(versionResponse{Version: version})

	mux := http.NewServeMux()

	// TODO write this into separate function
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

	return LoggingMiddleware(logger)(mux)
}
