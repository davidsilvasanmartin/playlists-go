package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

type clientImpl struct {
	httpClient *http.Client
	limiter    *rate.Limiter
	baseURL    string
	userAgent  string
}

// NewClient initializes and returns a new Client for interacting with the MusicBrainz API
func NewClient(baseURL string, userAgent string) Client {
	return &clientImpl{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Second), 1),
		baseURL:    baseURL,
		userAgent:  userAgent,
	}
}

func (c *clientImpl) Search(ctx context.Context, title string, artist string) ([]Recording, error) {
	var result []Recording
	err := withRetry(ctx, 3, 2*time.Second, func() error {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		recordings, err := c.doSearch(ctx, title, artist)
		if err != nil {
			return err
		}
		result = recordings
		return nil
	})
	return result, err
}

func (c *clientImpl) doSearch(ctx context.Context, title string, artist string) ([]Recording, error) {
	// MusicBrainz uses a Lucene-style syntax
	q := fmt.Sprintf(`recording:"%s" AND artistname:"%s"`, title, artist)
	params := url.Values{
		"query": []string{q},
		"fmt":   []string{"json"},
	}
	endpoint := c.baseURL + "/ws/2/recording?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MusicBrainz returned status %d", resp.StatusCode)
	}

	var mbResp mbSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&mbResp); err != nil {
		return nil, err
	}

	return mapRecordings(mbResp.Recordings), nil
}

// mapRecordings converts raw MusicBrainz recordings to the domain type
func mapRecordings(raw []mbRecording) []Recording {
	out := make([]Recording, 0, len(raw))
	for _, r := range raw {
		rec := Recording{
			MBID:           r.ID,
			Title:          r.Title,
			DurationMs:     r.Length,
			Disambiguation: r.Disambiguation,
		}
		if len(r.ArtistCredit) > 0 {
			// We are just using the first one, we may have to revisit this
			rec.ArtistMBID = r.ArtistCredit[0].Artist.ID
			rec.Artist = r.ArtistCredit[0].Artist.Name
		}
		if len(r.Releases) > 0 {
			// We are just using the first one, we may have to revisit this
			rec.AlbumMBID = r.Releases[0].ID
			rec.Album = r.Releases[0].Title
			rec.ReleaseDate = r.Releases[0].Date
		}
		out = append(out, rec)
	}
	return out
}

func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, fn func() error) error {
	var err error = nil
	for i := range maxAttempts {
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return err
}
