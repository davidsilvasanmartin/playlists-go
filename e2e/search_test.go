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
			MBID        string `json:"mbid"`
			Title       string `json:"title"`
			Artist      string `json:"artist"`
			Album       string `json:"album"`
			ReleaseDate string `json:"releaseDate"`
			DurationMs  int    `json:"durationMs"`
		} `json:"results"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Results, 1)

	first := body.Results[0]
	assert.Equal(t, "Bohemian Rhapsody", first.Title)
	assert.Equal(t, "Queen", first.Artist)
	assert.Equal(t, "A Night at the Opera", first.Album)
	assert.Equal(t, "1975-11-21", first.ReleaseDate)
	assert.Equal(t, 354000, first.DurationMs)
}
