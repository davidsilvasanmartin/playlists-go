package musicbrainz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var nopLogger = zap.NewNop()

func TestWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	err := withRetry(context.Background(), 3, time.Millisecond, nopLogger, func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestWithRetry_RetriesAndSucceeds(t *testing.T) {
	calls := 0
	err := withRetry(context.Background(), 3, time.Millisecond, nopLogger, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient error")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	sentinel := errors.New("always fails")
	calls := 0
	err := withRetry(context.Background(), 3, time.Millisecond, nopLogger, func() error {
		calls++
		return sentinel
	})
	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 3, calls)
}

func TestWithRetry_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := withRetry(ctx, 5, time.Millisecond, nopLogger, func() error {
		calls++
		if calls == 2 {
			cancel() // cancel after the first attempt
		}
		return errors.New("error")
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 2, calls)
}

func TestMapRecordings_FullData(t *testing.T) {
	raw := []mbRecording{
		{
			ID:             "mbid-001",
			Title:          "Bohemian Rhapsody",
			Length:         345678,
			Disambiguation: "studio recording",
			ArtistCredit: []mbCredit{
				{Artist: mbArtistInfo{ID: "artist-001", Name: "Queen"}},
			},
			Releases: []mbRelease{
				{ID: "release-001", Title: "A Night at the Opera", Date: "1975-11-21"},
			},
		},
	}

	got := mapRecordings(raw)

	expected := []Recording{
		{
			MBID:           "mbid-001",
			Title:          "Bohemian Rhapsody",
			Artist:         "Queen",
			ArtistMBID:     "artist-001",
			Album:          "A Night at the Opera",
			AlbumMBID:      "release-001",
			ReleaseDate:    "1975-11-21",
			DurationMs:     345678,
			Disambiguation: "studio recording",
		},
	}
	assert.Equal(t, expected, got)
}

func TestMapRecordings_Empty(t *testing.T) {
	got := mapRecordings(nil)
	assert.Empty(t, got)
}

func TestMapRecordings_NoArtistOrRelease(t *testing.T) {
	raw := []mbRecording{
		{ID: "mbid-002", Title: "Unknown"},
	}
	got := mapRecordings(raw)
	require.Len(t, got, 1)
	expected := []Recording{
		{
			MBID:  "mbid-002",
			Title: "Unknown",
			// The rest of the values will be zeroed out (empty strings or number 0)
		},
	}
	assert.Equal(t, expected, got)
}
