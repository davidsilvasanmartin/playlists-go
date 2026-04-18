package api

import (
	"net/http"

	"github.com/davidsilvasanmartin/playlists-go/internal/search"
	"go.uber.org/zap"
)

// NewRouter builds and returns the application's HTTP mux with all routes registered
// and middlewares applied
func NewRouter(logger *zap.Logger, searchHandler *search.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

	return LoggingMiddleware(logger)(mux)
}
