package musicbrainz

// Recording represents a musical recording with associated metadata
type Recording struct {
	MBID           string
	Title          string
	Artist         string
	ArtistMBID     string
	Album          string
	AlbumMBID      string
	ReleaseDate    string
	DurationMs     int
	Disambiguation string
}

// mbSearchResponse represents the response structure for a music metadata search response
// from the MusicBrainz API
type mbSearchResponse struct {
	Recordings []mbRecording `json:"recordings"`
}

// mbRecording represents a MusicBrainz music recording
type mbRecording struct {
	ID             string      `json:"id"`
	Title          string      `json:"title"`
	Length         int         `json:"length"`
	Disambiguation string      `json:"disambiguation"`
	ArtistCredit   []mbCredit  `json:"artist-credit"`
	Releases       []mbRelease `json:"releases"`
}

// mbCredit represents an artist's credit information associated with a MusicBrainz entity
type mbCredit struct {
	Artist mbArtistInfo `json:"artist"`
}

// mbArtistInfo represents basic information about an artist in MusicBrainz
type mbArtistInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// mbRelease represents a release entity in MusicBrainz
type mbRelease struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date"`
}
