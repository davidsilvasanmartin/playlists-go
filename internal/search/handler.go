package search

import (
	"net/http"

	"github.com/davidsilvasanmartin/playlists-go/internal/httputil"
	"go.uber.org/zap"
)

// Handler handles HTTP requests
type Handler struct {
	service Service
	logger  *zap.Logger
}

func NewHandler(svc Service, logger *zap.Logger) *Handler {
	return &Handler{service: svc, logger: logger}
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	artist := r.URL.Query().Get("artist")

	if len(title) < 2 {
		h.logger.Debug("validation failed: title too short",
			zap.String("title", title),
		)
		httputil.WriteError(w, r, http.StatusBadRequest, "Field 'title' must be at least 2 characters")
		return
	}
	if len(artist) < 2 {
		h.logger.Debug("validation failed: artist too short",
			zap.String("artist", artist),
		)
		httputil.WriteError(w, r, http.StatusBadRequest, "Field 'artist' must be at least 2 characters")
		return
	}

	results, err := h.service.Search(r.Context(), title, artist)
	if err == nil {
		httputil.WriteJSON(w, http.StatusOK, Response{Results: results})
	} else {
		h.logger.Error("search service error",
			zap.String("title", title),
			zap.String("artist", artist),
			zap.Error(err),
		)
		httputil.WriteError(w, r, http.StatusServiceUnavailable, "MusicBrainz API is currently unreachable. Please try again later")
	}
}
