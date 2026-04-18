package api

import (
	"net/http"

	"github.com/davidsilvasanmartin/playlists-go/internal/search"
)

// NewRouter builds and returns the application's HTTP mux with all routes registered
func NewRouter(searchHandler *search.Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/songs/search", searchHandler.Search)

	return mux
}
