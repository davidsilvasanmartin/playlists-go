package search

import (
	"context"
	"errors"
	"testing"

	"github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var nopLogger = zap.NewNop()

// mockMBClient is a testify/mock implementation of musicBrainz.Client
type mockMBClient struct {
	mock.Mock
}

func (m *mockMBClient) Search(ctx context.Context, title string, artist string) ([]musicbrainz.Recording, error) {
	args := m.Called(ctx, title, artist)
	// Some "Return" calls will have nil as the first argument. We need
	// this guard because otherwise casting nil below would crash
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]musicbrainz.Recording), args.Error(1)
}

func TestService_Search_MapsResults(t *testing.T) {
	recordings := []musicbrainz.Recording{
		{
			MBID:           "mbid-001",
			Title:          "Bohemian Rhapsody",
			Artist:         "Queen",
			ArtistMBID:     "artist-001",
			Album:          "A Night at the Opera",
			AlbumMBID:      "release-001",
			ReleaseDate:    "1975-11-21",
			DurationMs:     354000,
			Disambiguation: "studio recording",
		},
	}
	mbClient := new(mockMBClient)
	mbClient.On("Search", mock.Anything, "Bohemian Rhapsody", "Queen").Return(recordings, nil)
	svc := NewService(mbClient, nopLogger)

	results, err := svc.Search(context.Background(), "Bohemian Rhapsody", "Queen")

	require.NoError(t, err)
	require.Len(t, results, 1)
	expected := []Result{
		{
			MBID:           "mbid-001",
			Title:          "Bohemian Rhapsody",
			Artist:         "Queen",
			ArtistMBID:     "artist-001",
			Album:          "A Night at the Opera",
			AlbumMBID:      "release-001",
			ReleaseDate:    "1975-11-21",
			DurationMs:     354000,
			Disambiguation: "studio recording",
		},
	}
	assert.Equal(t, expected, results)
	mbClient.AssertExpectations(t)
}

func TestService_Search_EmptyResults(t *testing.T) {
	mbClient := new(mockMBClient)
	mbClient.On("Search", mock.Anything, "Unknown", "No artist").Return([]musicbrainz.Recording{}, nil)
	svc := NewService(mbClient, nopLogger)

	results, err := svc.Search(context.Background(), "Unknown", "No artist")

	require.NoError(t, err)
	assert.Empty(t, results)
	mbClient.AssertExpectations(t)
}

func TestService_Search_PropagatesError(t *testing.T) {
	mbClient := new(mockMBClient)
	mbClient.On("Search", mock.Anything, "Any Title", "Any Artist").Return(nil, errors.New("network error"))
	svc := NewService(mbClient, nopLogger)

	_, err := svc.Search(context.Background(), "Any Title", "Any Artist")

	assert.Error(t, err)
	mbClient.AssertExpectations(t)
}
