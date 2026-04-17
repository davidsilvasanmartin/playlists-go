package search

import (
	"context"

	"github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
)

// Service is the contract for the search business logic
type Service interface {
	Search(ctx context.Context, title string, artist string) ([]Result, error)
}

type service struct {
	mb musicbrainz.Client
}

func NewService(mb musicbrainz.Client) Service {
	return &service{mb: mb}
}

func (s *service) Search(ctx context.Context, title string, artist string) ([]Result, error) {
	recordings, err := s.mb.Search(ctx, title, artist)
	if err != nil {
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

	return results, nil
}
