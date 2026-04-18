package main

import (
	"log"
	"net/http"
	"os"

	"github.com/davidsilvasanmartin/playlists-go/internal/api"
	"github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
	"github.com/davidsilvasanmartin/playlists-go/internal/search"
	"github.com/joho/godotenv"
)

func main() {
	// -- config ----------------
	// Load committed defaults first, then let .env override personal values
	// Both files are optional - errors are silently discarded
	// In Docker neither file exists; env vars are injected by the container runtime
	_ = godotenv.Load(".development.env")
	_ = godotenv.Overload(".env")

	// TODO we are setting defaults here which we may forget about; maybe it's better to just use Viper
	port := getEnv("PLAYLISTS_PORT", "8080")
	mbBaseURL := getEnv("PLAYLISTS_MB_BASE_URL", "https://musicbrainz.org")
	mbUserAgent := mustGetEnv("PLAYLISTS_MB_USER_AGENT")

	// -- dependencies ----------------
	mbClient := musicbrainz.NewClient(mbBaseURL, mbUserAgent)
	searchService := search.NewService(mbClient)
	searchHandler := search.NewHandler(searchService)

	// -- routing ----------------
	mux := api.NewRouter(searchHandler)

	// -- server ----------------
	addr := ":" + port
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustGetEnv returns the value of an environment variable. If the variable is
// not set, or if it is set but empty, it will cause the program to crash
func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return v
}
