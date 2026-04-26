package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration values required to run the server.
// mapstructure tags use the full environment variable name (lowercased) so
// that Viper resolves values consistently whether they come from a dotenv
// file or the real process environment.
type Config struct {
	Port        string `mapstructure:"playlists_port"`
	MBBaseURL   string `mapstructure:"playlists_mb_base_url"`
	MBUserAgent string `mapstructure:"playlists_mb_user_agent"`
	LogLevel    string `mapstructure:"playlists_log_level"`
	LogFormat   string `mapstructure:"playlists_log_format"`
}

// requiredKeys lists every Viper key the app needs.
// Each key is the lowercased form of the full environment variable name.
var requiredKeys = []string{
	"playlists_port",
	"playlists_mb_base_url",
	"playlists_mb_user_agent",
	"playlists_log_level",
	"playlists_log_format",
}

// Load populates Config from several sources, in increasing priority order:
//
//  1. .development.env — committed defaults, loaded first
//  2. .env             — personal overrides, gitignored, merged on top
//  3. process env      — injected by Docker / CI, always wins
//
// Both files are optional. If a file does not exist the error is silently
// ignored — this is expected in Docker and CI where neither file is present.
// Any other file error (permission denied, malformed content) is returned.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigType("dotenv")

	// -- 1. Committed defaults --------------------------------------------
	v.SetConfigFile(".development.env")
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read .development.env: %w", err)
	}

	// -- 2. Personal overrides --------------------------------------------
	v.SetConfigFile(".env")
	if err := v.MergeInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("merge .env: %w", err)
	}

	// -- 3. Process environment -------------------------------------------
	// AutomaticEnv uppercases the Viper key to find the env var:
	// "playlists_port" -> PLAYLISTS_PORT
	v.AutomaticEnv()
	// BindEnv must be called for each key so that Unmarshal resolves env vars.
	// AutomaticEnv alone only affects Get* calls; Unmarshal iterates Viper's
	// internal key registry and silently returns empty strings for keys it has
	// never seen from a file or BindEnv/SetDefault — even if the env var exists.
	for _, key := range requiredKeys {
		_ = v.BindEnv(key)
	}

	// -- 4. Validate all required keys are present ------------------------
	var missing []string
	for _, key := range requiredKeys {
		if v.GetString(key) == "" {
			missing = append(missing, strings.ToUpper(key))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	// -- 5. Populate struct -----------------------------------------------
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
