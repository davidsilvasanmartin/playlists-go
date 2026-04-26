package api

import (
	"net/http"

	"github.com/davidsilvasanmartin/playlists-go/internal/httputil"
	"go.uber.org/zap"
)

type searchHandler interface {
	Search(w http.ResponseWriter, r *http.Request)
}

// NewRouter builds and returns the application's HTTP mux with all routes registered
// and middlewares applied
func NewRouter(logger *zap.Logger, sh searchHandler, version string) http.Handler {
	type versionResponse struct {
		Version string `json:"version"`
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, versionResponse{Version: version})
	})
	mux.HandleFunc("GET /api/v1/songs/search", sh.Search)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httputil.WriteError(w, r, http.StatusNotFound, "The requested resource was not found")
	})

	return LoggingMiddleware(logger)(mux)
}
