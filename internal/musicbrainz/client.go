package musicbrainz

import "context"

// Client is the contract for communicating with the MusicBrainz API.
type Client interface {
	// Search retrieves a list of recordings from the MusicBrainz API based on
	// the provided title and artist name
	Search(ctx context.Context, title string, artist string) ([]Recording, error)
}
