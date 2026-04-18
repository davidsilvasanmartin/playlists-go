package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type clientImpl struct {
	httpClient *http.Client
	limiter    *rate.Limiter
	baseURL    string
	userAgent  string
	logger     *zap.Logger
}

// NewClient initializes and returns a new Client for interacting with the MusicBrainz API
func NewClient(baseURL string, userAgent string, logger *zap.Logger) Client {
	return &clientImpl{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Second), 1),
		baseURL:    baseURL,
		userAgent:  userAgent,
		logger:     logger,
	}
}

func (c *clientImpl) Search(ctx context.Context, title string, artist string) ([]Recording, error) {
	c.logger.Debug("starting MusicBrainz search",
		zap.String("title", title),
		zap.String("artist", artist),
	)

	var result []Recording
	err := withRetry(ctx, 3, 2*time.Second, c.logger, func() error {
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

	c.logger.Debug("MusicBrainz search complete",
		zap.String("title", title),
		zap.String("artist", artist),
		zap.Int("recordings", len(result)),
	)
	return result, err
}

func (c *clientImpl) doSearch(ctx context.Context, title string, artist string) ([]Recording, error) {
	// MusicBrainz uses a Lucene-style syntax
	q := fmt.Sprintf(`recording:"%s" AND artistname:"%s"`, title, artist)
	params := url.Values{
		"query": []string{q},
		"fmt":   []string{"json"},
		"limit": []string{"100"},
	}
	endpoint := c.baseURL + "/ws/2/recording?" + params.Encode()

	c.logger.Debug("sending HTTP request to MusicBrainz", zap.String("url", endpoint))

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

// withRetry calls fn up to maxAttempts times, waiting delay between failures.
// It logs each failed attempt and gives up after the last one, returning the
// final error.  Context cancellation is respected between retries.
func withRetry(ctx context.Context, maxAttempts int, delay time.Duration, logger *zap.Logger, fn func() error) error {
	var err error = nil
	for i := range maxAttempts {
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			logger.Warn("MusicBrainz request failed, will retry",
				zap.Int("attempt", i+1),
				zap.Int("maxAttempts", maxAttempts),
				zap.Error(err),
				zap.Duration("retryIn", delay),
			)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	logger.Error("MusicBrainz request failed after all retry attempts",
		zap.Int("attempts", maxAttempts),
		zap.Error(err),
	)
	return err
}
