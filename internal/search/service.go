package search

import (
	"context"

	"github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
	"go.uber.org/zap"
)

// Service is the contract for the search business logic
type Service interface {
	Search(ctx context.Context, title string, artist string) ([]Result, error)
}

type service struct {
	mb     musicbrainz.Client
	logger *zap.Logger
}

func NewService(mb musicbrainz.Client, logger *zap.Logger) Service {
	return &service{mb: mb, logger: logger}
}

func (s *service) Search(ctx context.Context, title string, artist string) ([]Result, error) {
	s.logger.Debug("service.Search called",
		zap.String("title", title),
		zap.String("artist", artist),
	)

	recordings, err := s.mb.Search(ctx, title, artist)
	if err != nil {
		s.logger.Debug("MusicBrainz client returned an error",
			zap.String("title", title),
			zap.String("artist", artist),
			zap.Error(err),
		)
		return nil, err
	}

	results := make([]Result, len(recordings))
	for i, r := range recordings {
		results[i] = Result{
			MBID:           r.MBID,
			Title:          r.Title,
			Artist:         r.Artist,
			ArtistMBID:     r.ArtistMBID,
			Album:          r.Album,
			AlbumMBID:      r.AlbumMBID,
			ReleaseDate:    r.ReleaseDate,
			DurationMs:     r.DurationMs,
			Disambiguation: r.Disambiguation,
		}
	}

	s.logger.Debug("service.Search returning results",
		zap.String("title", title),
		zap.String("artist", artist),
		zap.Int("count", len(results)),
	)
	return results, nil
}
