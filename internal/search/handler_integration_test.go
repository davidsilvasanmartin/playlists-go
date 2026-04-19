//go:build integration

package search_test // Different package name so that we can't export symbols private to the search package
import (
	"net/http"

	"github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
	"github.com/davidsilvasanmartin/playlists-go/internal/search"
)

// mbSearchFixture is a minimal valid MusicBrainz search response
const mbSearchFixture = `{
  "recordings": [
    {
      "id": "b1a9c0e2-0000-0000-0000-000000000001",
      "title": "Bohemian Rhapsody",
      "length": 354000,
      "disambiguation": "studio recording",
      "artist-credit": [
        {
          "artist": {
            "id": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
            "name": "Queen"
          }
        }
      ],
      "releases": [
        {
          "id": "1dc4c347-a1db-32aa-b14f-bc9cc507b843",
          "title": "A Night at the Opera",
          "date": "1975-11-21"
        }
      ]
    },
    {
      "id": "b1a9c0e2-0000-0000-0000-000000000002",
      "title": "Bohemian Rhapsody",
      "length": 360000,
      "disambiguation": "live, 1986-07-12: Wembley Stadium, London, UK",
      "artist-credit": [
        {
          "artist": {
            "id": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
            "name": "Queen"
          }
        }
      ],
      "releases": [
        {
          "id": "2ef5c347-0000-0000-0000-bc9cc507b843",
          "title": "Live at Wembley '86",
          "date": "1992-05-26"
        }
      ]
    }
  ]
}`

func newTestApp(mbBaseURL string) http.Handler {
	mbClient := musicbrainz.NewClient(mbBaseURL, "playlists-test/0.0.1 ( test@example.com )")
	svc := search.NewService(mbClient)
	handler := search.NewHandler(svc)

	mux := http.NewServeMux()
	muxh.HandleFunc("GET /api/v1/songs/search", handler.Search)
	return mux
}
