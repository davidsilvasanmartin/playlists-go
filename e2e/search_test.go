//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_Search_HappyPath(t *testing.T) {
	resp, err := http.Get(appURL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=Queen")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body struct {
		Results []struct {
			MBID           string `json:"mbid"`
			Title          string `json:"title"`
			Artist         string `json:"artist"`
			ArtistMBID     string `json:"artistMbid"`
			Album          string `json:"album"`
			AlbumMBID      string `json:"albumMbid"`
			ReleaseDate    string `json:"releaseDate"`
			DurationMs     int    `json:"durationMs"`
			Disambiguation string `json:"disambiguation"`
		} `json:"results"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Results, 1)

	first := body.Results[0]
	assert.Equal(t, "b1a9c0e2-0000-0000-0000-000000000001", first.MBID)
	assert.Equal(t, "Bohemian Rhapsody", first.Title)
	assert.Equal(t, "Queen", first.Artist)
	assert.Equal(t, "0383dadf-2a4e-4d10-a46a-e9e041da8eb3", first.ArtistMBID)
	assert.Equal(t, "A Night at the Opera", first.Album)
	assert.Equal(t, "1dc4c347-a1db-32aa-b14f-bc9cc507b843", first.AlbumMBID)
	assert.Equal(t, "1975-11-21", first.ReleaseDate)
	assert.Equal(t, 354000, first.DurationMs)
	assert.Equal(t, "studio recording", first.Disambiguation)
}

func TestE2E_Search_ValidationError_TitleTooShort(t *testing.T) {
	resp, err := http.Get(appURL + "/api/v1/songs/search?title=X&artist=Queen")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, float64(400), body["status"])
	assert.Contains(t, body["message"], "title")
}

func TestE2E_Search_ValidationError_ArtistTooShort(t *testing.T) {
	resp, err := http.Get(appURL + "/api/v1/songs/search?title=Bohemian+Rhapsody&artist=X")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, float64(400), body["status"])
	assert.Contains(t, body["message"], "artist")
}

func TestE2E_Search_MusicBrainzUnavailable(t *testing.T) {
	// The "TRIGGER_503" stub in mappings/mb-search-unavailable.yaml matches
	// any query containing this string and returns 503.
	resp, err := http.Get(appURL + "/api/v1/songs/search?title=TRIGGER_503&artist=test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
