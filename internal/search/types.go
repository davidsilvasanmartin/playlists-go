package search

// Result is a single item in the search response
// TODO this is a recording, need to come up with better naming
type Result struct {
	MBID           string `json:"mbid"`
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	ArtistMBID     string `json:"artistMbid"`
	Album          string `json:"album"`
	AlbumMBID      string `json:"albumMbid"`
	ReleaseDate    string `json:"releaseDate"`
	DurationMs     int    `json:"durationMs"`
	Disambiguation string `json:"disambiguation"`
}

// Response is the envelope returned by GET /api/v1/songs/search
type Response struct {
	Results []Result `json:"results"`
}
