-- ============================================================================
-- Playlists Application Database Schema
-- ============================================================================
-- PostgreSQL 17.6+
-- This schema supports a music playlist management application with:
-- - User authentication and authorization
-- - MusicBrainz integration for song metadata
-- - Manual and smart (programmatic) playlists
-- - User-defined tags for songs
-- - Playable links discovery (YouTube, SoundCloud, Spotify)
-- ============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";  -- For UUID generation
CREATE EXTENSION IF NOT EXISTS "pg_trgm";    -- For trigram-based text search

-- ============================================================================
-- CUSTOM TYPES
-- ============================================================================

-- User roles for role-based access control
CREATE TYPE user_role AS ENUM ('USER', 'ADMIN');

-- Artist types from MusicBrainz
CREATE TYPE artist_type AS ENUM (
    'Person',
    'Group',
    'Orchestra',
    'Choir',
    'Character',
    'Other'
);

-- Playlist types
CREATE TYPE playlist_type AS ENUM ('manual', 'smart');

-- Playable link services
CREATE TYPE playable_service AS ENUM ('youtube', 'soundcloud', 'spotify', 'navidrome');

-- Link discovery status for background jobs
CREATE TYPE link_status AS ENUM ('pending', 'processing', 'completed', 'failed');

-- ============================================================================
-- CORE TABLES
-- ============================================================================

-- ----------------------------------------------------------------------------
-- Users table
-- ----------------------------------------------------------------------------
-- Stores user accounts with authentication credentials
-- Note: Passwords should be hashed with bcrypt (salt rounds >= 12)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(30) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    role user_role NOT NULL DEFAULT 'USER',
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Constraints
    CONSTRAINT users_username_check CHECK (
        username ~ '^[a-zA-Z0-9_]{3,30}$'
    ),
    CONSTRAINT users_email_check CHECK (
        email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}$'
    ),
    CONSTRAINT users_display_name_check CHECK (
        length(display_name) >= 1 AND length(display_name) <= 100
    )
);

-- Unique constraints
CREATE UNIQUE INDEX users_username_unique_idx ON users (LOWER(username));
CREATE UNIQUE INDEX users_email_unique_idx ON users (LOWER(email));

-- Index for login lookups
CREATE INDEX users_email_idx ON users (email);

COMMENT ON TABLE users IS 'User accounts with authentication credentials';
COMMENT ON COLUMN users.password_hash IS 'bcrypt hash with salt rounds >= 12';
COMMENT ON COLUMN users.last_login_at IS 'Timestamp of last successful login';

-- ----------------------------------------------------------------------------
-- Refresh tokens table
-- ----------------------------------------------------------------------------
-- JWT refresh tokens for authentication
-- Note: Access tokens are stateless JWT (15 min expiry), refresh tokens stored here (7 days)
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,  -- Hash of the actual token
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT refresh_tokens_expires_check CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX refresh_tokens_token_hash_idx ON refresh_tokens (token_hash);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_expires_at_idx ON refresh_tokens (expires_at);

COMMENT ON TABLE refresh_tokens IS 'JWT refresh tokens with 7-day expiry';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'SHA-256 hash of the refresh token';

-- ----------------------------------------------------------------------------
-- Artists table
-- ----------------------------------------------------------------------------
-- Music artists from MusicBrainz
-- Note: Artists are SHARED across users (not user-specific)
CREATE TABLE artists (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    musicbrainz_id VARCHAR(36) NOT NULL,
    name VARCHAR(500) NOT NULL,
    sort_name VARCHAR(500) NOT NULL,
    country CHAR(2),  -- ISO 3166-1 alpha-2 country code
    type artist_type,
    disambiguation TEXT,
    life_span_begin VARCHAR(10),  -- Year or date (YYYY or YYYY-MM-DD)
    life_span_end VARCHAR(10),    -- Year or date, NULL if still active
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX artists_musicbrainz_id_idx ON artists (musicbrainz_id);
CREATE INDEX artists_name_idx ON artists (name);
CREATE INDEX artists_name_trgm_idx ON artists USING gin (name gin_trgm_ops);

COMMENT ON TABLE artists IS 'Music artists from MusicBrainz (shared across all users)';
COMMENT ON COLUMN artists.musicbrainz_id IS 'MusicBrainz artist ID (MBID)';
COMMENT ON COLUMN artists.country IS 'ISO 3166-1 alpha-2 country code';
COMMENT ON COLUMN artists.life_span_begin IS 'Birth/formation year or full date';
COMMENT ON COLUMN artists.life_span_end IS 'Death/dissolution year or date, NULL if still active';

-- ----------------------------------------------------------------------------
-- Albums table
-- ----------------------------------------------------------------------------
-- Music albums from MusicBrainz
-- Note: Albums are SHARED across users (not user-specific)
CREATE TABLE albums (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    musicbrainz_id VARCHAR(36) NOT NULL,
    title VARCHAR(500) NOT NULL,
    release_date DATE,
    track_count INTEGER,
    cover_art_url TEXT,  -- URL to Cover Art Archive
    label VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT albums_track_count_check CHECK (track_count IS NULL OR track_count > 0)
);

CREATE UNIQUE INDEX albums_musicbrainz_id_idx ON albums (musicbrainz_id);
CREATE INDEX albums_title_idx ON albums (title);
CREATE INDEX albums_title_trgm_idx ON albums USING gin (title gin_trgm_ops);
CREATE INDEX albums_release_date_idx ON albums (release_date);

COMMENT ON TABLE albums IS 'Music albums from MusicBrainz (shared across all users)';
COMMENT ON COLUMN albums.musicbrainz_id IS 'MusicBrainz release ID';
COMMENT ON COLUMN albums.cover_art_url IS 'Cover Art Archive URL';

-- ----------------------------------------------------------------------------
-- Songs table
-- ----------------------------------------------------------------------------
-- Songs in user libraries
-- Note: Songs are USER-SPECIFIC. Same MusicBrainz recording can be added by multiple users.
-- This allows users to have independent libraries and apply their own tags.
CREATE TABLE songs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    musicbrainz_recording_id VARCHAR(36) NOT NULL,
    title VARCHAR(500) NOT NULL,
    duration INTEGER,  -- Duration in seconds
    release_date DATE,
    artist_id UUID NOT NULL REFERENCES artists(id) ON DELETE RESTRICT,
    album_id UUID REFERENCES albums(id) ON DELETE SET NULL,
    track_position INTEGER,  -- Position in album
    genres TEXT[],  -- Array of genre strings from MusicBrainz
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT songs_duration_check CHECK (duration IS NULL OR duration > 0),
    CONSTRAINT songs_track_position_check CHECK (track_position IS NULL OR track_position > 0)
);

-- Prevent duplicate songs per user (same MusicBrainz recording)
CREATE UNIQUE INDEX songs_user_musicbrainz_idx ON songs (user_id, musicbrainz_recording_id);

CREATE INDEX songs_user_id_idx ON songs (user_id);
CREATE INDEX songs_artist_id_idx ON songs (artist_id);
CREATE INDEX songs_album_id_idx ON songs (album_id);
CREATE INDEX songs_title_idx ON songs (title);
CREATE INDEX songs_title_trgm_idx ON songs USING gin (title gin_trgm_ops);
CREATE INDEX songs_created_at_idx ON songs (created_at);
CREATE INDEX songs_release_date_idx ON songs (release_date);

-- GIN index for genre array searches
CREATE INDEX songs_genres_idx ON songs USING gin (genres);

COMMENT ON TABLE songs IS 'Songs in user libraries (user-specific, same recording can be added by multiple users)';
COMMENT ON COLUMN songs.musicbrainz_recording_id IS 'MusicBrainz recording ID';
COMMENT ON COLUMN songs.duration IS 'Song duration in seconds';
COMMENT ON COLUMN songs.track_position IS 'Track position on the album';
COMMENT ON COLUMN songs.genres IS 'Array of genre strings from MusicBrainz';
COMMENT ON CONSTRAINT songs_user_musicbrainz_idx ON songs IS 'Prevents duplicate songs per user';

-- ----------------------------------------------------------------------------
-- Tags table
-- ----------------------------------------------------------------------------
-- User-defined tags for organizing songs
-- Note: Tags are USER-SPECIFIC (each user has their own namespace)
CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    color CHAR(7) NOT NULL,  -- Hex color code #RRGGBB
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT tags_name_check CHECK (
        length(name) >= 1 AND length(name) <= 50
    ),
    CONSTRAINT tags_color_check CHECK (
        color ~ '^#[0-9A-Fa-f]{6}$'
    )
);

-- Unique tag name per user
CREATE UNIQUE INDEX tags_user_name_idx ON tags (user_id, LOWER(name));
CREATE INDEX tags_user_id_idx ON tags (user_id);

COMMENT ON TABLE tags IS 'User-defined tags for organizing songs (user-specific)';
COMMENT ON COLUMN tags.color IS 'Hex color code in format #RRGGBB';

-- ----------------------------------------------------------------------------
-- Song-Tags junction table
-- ----------------------------------------------------------------------------
-- Many-to-many relationship between songs and tags
CREATE TABLE song_tags (
    song_id UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (song_id, tag_id)
);

CREATE INDEX song_tags_tag_id_idx ON song_tags (tag_id);

COMMENT ON TABLE song_tags IS 'Many-to-many relationship between songs and tags';

-- ----------------------------------------------------------------------------
-- Playlists table
-- ----------------------------------------------------------------------------
-- User playlists (manual and smart/programmatic)
CREATE TABLE playlists (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    type playlist_type NOT NULL DEFAULT 'manual',
    is_public BOOLEAN NOT NULL DEFAULT FALSE,

    -- Smart playlist configuration (NULL for manual playlists)
    -- JSONB structure: { "genres": [], "yearRange": {"from": 1970, "to": 1979},
    --                    "tags": [], "artists": [], "albums": [],
    --                    "minDuration": 180, "maxDuration": 600 }
    criteria JSONB,

    -- Smart playlist sort configuration (NULL for manual playlists)
    -- JSONB structure: { "field": "releaseDate", "order": "asc" }
    sort_config JSONB,

    -- Smart playlist limit (NULL for manual playlists)
    song_limit INTEGER,

    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT playlists_name_check CHECK (
        length(name) >= 1 AND length(name) <= 200
    ),
    CONSTRAINT playlists_description_check CHECK (
        description IS NULL OR length(description) <= 1000
    ),
    CONSTRAINT playlists_smart_criteria_check CHECK (
        (type = 'manual' AND criteria IS NULL AND sort_config IS NULL AND song_limit IS NULL) OR
        (type = 'smart')
    ),
    CONSTRAINT playlists_song_limit_check CHECK (
        song_limit IS NULL OR song_limit > 0
    )
);

CREATE INDEX playlists_user_id_idx ON playlists (user_id);
CREATE INDEX playlists_type_idx ON playlists (type);
CREATE INDEX playlists_is_public_idx ON playlists (is_public);
CREATE INDEX playlists_created_at_idx ON playlists (created_at);
CREATE INDEX playlists_updated_at_idx ON playlists (updated_at);

-- GIN index for JSONB criteria queries
CREATE INDEX playlists_criteria_idx ON playlists USING gin (criteria);

COMMENT ON TABLE playlists IS 'User playlists (manual and smart/programmatic)';
COMMENT ON COLUMN playlists.type IS 'manual = user-curated, smart = programmatically defined by criteria';
COMMENT ON COLUMN playlists.criteria IS 'Smart playlist criteria (JSONB), NULL for manual playlists';
COMMENT ON COLUMN playlists.sort_config IS 'Smart playlist sort configuration (JSONB), NULL for manual playlists';
COMMENT ON COLUMN playlists.song_limit IS 'Maximum number of songs for smart playlists, NULL for unlimited or manual';

-- ----------------------------------------------------------------------------
-- Playlist-Songs junction table
-- ----------------------------------------------------------------------------
-- Many-to-many relationship between playlists and songs
-- Note: Only used for MANUAL playlists. Smart playlists are computed dynamically.
CREATE TABLE playlist_songs (
    playlist_id UUID NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    song_id UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,  -- 1-indexed position in playlist
    added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (playlist_id, song_id),

    CONSTRAINT playlist_songs_position_check CHECK (position > 0)
);

-- Ensure unique positions within a playlist
CREATE UNIQUE INDEX playlist_songs_position_idx ON playlist_songs (playlist_id, position);
CREATE INDEX playlist_songs_song_id_idx ON playlist_songs (song_id);

COMMENT ON TABLE playlist_songs IS 'Songs in manual playlists with position tracking';
COMMENT ON COLUMN playlist_songs.position IS '1-indexed position in the playlist order';

-- ----------------------------------------------------------------------------
-- Playable Links table
-- ----------------------------------------------------------------------------
-- Links to external services where songs can be played
-- Background jobs discover and verify these links
CREATE TABLE playable_links (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    song_id UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    service playable_service NOT NULL,
    url TEXT NOT NULL,
    title VARCHAR(500),
    description TEXT,
    thumbnail_url TEXT,
    duration INTEGER,  -- Duration in seconds (may differ slightly from song.duration)
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,  -- Manually verified by user or API
    is_available BOOLEAN NOT NULL DEFAULT TRUE,  -- Checked by background jobs
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT playable_links_duration_check CHECK (duration IS NULL OR duration > 0),
    CONSTRAINT playable_links_url_check CHECK (length(url) > 0)
);

-- Prevent duplicate URLs per song
CREATE UNIQUE INDEX playable_links_song_url_idx ON playable_links (song_id, url);

CREATE INDEX playable_links_song_id_idx ON playable_links (song_id);
CREATE INDEX playable_links_service_idx ON playable_links (service);
CREATE INDEX playable_links_is_verified_idx ON playable_links (is_verified);
CREATE INDEX playable_links_is_available_idx ON playable_links (is_available);
CREATE INDEX playable_links_last_checked_at_idx ON playable_links (last_checked_at);

COMMENT ON TABLE playable_links IS 'Links to external services (YouTube, SoundCloud, Spotify) where songs can be played';
COMMENT ON COLUMN playable_links.is_verified IS 'Manually verified by user or official API match';
COMMENT ON COLUMN playable_links.is_available IS 'Availability status checked by background jobs';
COMMENT ON COLUMN playable_links.last_checked_at IS 'Timestamp of last availability check';

-- ----------------------------------------------------------------------------
-- Link Discovery Jobs table
-- ----------------------------------------------------------------------------
-- Tracks background jobs for discovering playable links
-- This helps prevent duplicate jobs and provides status tracking
CREATE TABLE link_discovery_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    song_id UUID NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    status link_status NOT NULL DEFAULT 'pending',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT link_discovery_jobs_dates_check CHECK (
        (started_at IS NULL OR started_at >= created_at) AND
        (completed_at IS NULL OR (started_at IS NOT NULL AND completed_at >= started_at))
    )
);

-- Only allow one active job per song
CREATE UNIQUE INDEX link_discovery_jobs_song_active_idx
    ON link_discovery_jobs (song_id)
    WHERE status IN ('pending', 'processing');

CREATE INDEX link_discovery_jobs_song_id_idx ON link_discovery_jobs (song_id);
CREATE INDEX link_discovery_jobs_status_idx ON link_discovery_jobs (status);
CREATE INDEX link_discovery_jobs_created_at_idx ON link_discovery_jobs (created_at);

COMMENT ON TABLE link_discovery_jobs IS 'Background jobs for discovering playable links';
COMMENT ON CONSTRAINT link_discovery_jobs_song_active_idx ON link_discovery_jobs
    IS 'Ensures only one active (pending/processing) job exists per song';

-- ============================================================================
-- FUNCTIONS AND TRIGGERS
-- ============================================================================

-- ----------------------------------------------------------------------------
-- Automatically update updated_at timestamp
-- ----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to all tables with updated_at column
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_artists_updated_at BEFORE UPDATE ON artists
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_albums_updated_at BEFORE UPDATE ON albums
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_songs_updated_at BEFORE UPDATE ON songs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_tags_updated_at BEFORE UPDATE ON tags
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_playlists_updated_at BEFORE UPDATE ON playlists
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- VIEWS
-- ============================================================================

-- ----------------------------------------------------------------------------
-- User statistics view
-- ----------------------------------------------------------------------------
-- Provides aggregate statistics for users (used by GET /users/me endpoint)
CREATE VIEW user_stats AS
SELECT
    u.id AS user_id,
    COUNT(DISTINCT s.id) AS total_songs,
    COUNT(DISTINCT p.id) AS total_playlists,
    COUNT(DISTINCT t.id) AS total_tags,
    COUNT(DISTINCT CASE WHEN p.is_public = TRUE THEN p.id END) AS total_public_playlists
FROM users u
LEFT JOIN songs s ON s.user_id = u.id
LEFT JOIN playlists p ON p.user_id = u.id
LEFT JOIN tags t ON t.user_id = u.id
GROUP BY u.id;

COMMENT ON VIEW user_stats IS 'Aggregate statistics for users';

-- ----------------------------------------------------------------------------
-- Playlist statistics view
-- ----------------------------------------------------------------------------
-- Provides song count and total duration for playlists
CREATE VIEW playlist_stats AS
SELECT
    p.id AS playlist_id,
    p.type,
    COUNT(ps.song_id) AS song_count,
    COALESCE(SUM(s.duration), 0) AS total_duration
FROM playlists p
LEFT JOIN playlist_songs ps ON ps.playlist_id = p.id
LEFT JOIN songs s ON s.id = ps.song_id
WHERE p.type = 'manual'  -- Manual playlists only; smart playlists computed dynamically
GROUP BY p.id, p.type;

COMMENT ON VIEW playlist_stats IS 'Song count and total duration for manual playlists';

-- ----------------------------------------------------------------------------
-- Tag usage statistics view
-- ----------------------------------------------------------------------------
-- Shows how many songs are associated with each tag
CREATE VIEW tag_stats AS
SELECT
    t.id AS tag_id,
    t.user_id,
    t.name,
    COUNT(st.song_id) AS song_count
FROM tags t
LEFT JOIN song_tags st ON st.tag_id = t.id
GROUP BY t.id, t.user_id, t.name;

COMMENT ON VIEW tag_stats IS 'Song count for each tag';

-- ----------------------------------------------------------------------------
-- Artist statistics view
-- ----------------------------------------------------------------------------
-- Shows song and album counts per artist (for a user's library)
CREATE VIEW artist_stats AS
SELECT
    a.id AS artist_id,
    s.user_id,
    COUNT(DISTINCT s.id) AS song_count,
    COUNT(DISTINCT s.album_id) AS album_count
FROM artists a
INNER JOIN songs s ON s.artist_id = a.id
GROUP BY a.id, s.user_id;

COMMENT ON VIEW artist_stats IS 'Song and album counts per artist in user libraries';

-- ----------------------------------------------------------------------------
-- Album statistics view
-- ----------------------------------------------------------------------------
-- Shows song count per album (for a user's library)
CREATE VIEW album_stats AS
SELECT
    al.id AS album_id,
    s.user_id,
    COUNT(s.id) AS song_count
FROM albums al
INNER JOIN songs s ON s.album_id = al.id
GROUP BY al.id, s.user_id;

COMMENT ON VIEW album_stats IS 'Song count per album in user libraries';

-- ============================================================================
-- INDEXES FOR PERFORMANCE
-- ============================================================================

-- Additional composite indexes for common query patterns

-- Songs with filtering and sorting
CREATE INDEX songs_user_created_idx ON songs (user_id, created_at DESC);
CREATE INDEX songs_user_title_idx ON songs (user_id, title);
CREATE INDEX songs_user_artist_idx ON songs (user_id, artist_id);
CREATE INDEX songs_user_album_idx ON songs (user_id, album_id);

-- Playlists with filtering
CREATE INDEX playlists_user_type_idx ON playlists (user_id, type);
CREATE INDEX playlists_user_updated_idx ON playlists (user_id, updated_at DESC);

-- Efficient tag lookups
CREATE INDEX tags_user_created_idx ON tags (user_id, created_at);

-- ============================================================================
-- SAMPLE DATA COMMENTS
-- ============================================================================

COMMENT ON DATABASE playlists IS 'Playlists application database - music library and playlist management with MusicBrainz integration';

-- ============================================================================
-- SECURITY NOTES
-- ============================================================================

-- 1. Application should use a dedicated database user with limited privileges
-- 2. Password hashes must use bcrypt with salt rounds >= 12
-- 3. JWT tokens: access tokens (15 min), refresh tokens (7 days, stored in DB)
-- 4. All user inputs must be sanitized and parameterized queries used
-- 5. Implement row-level security if multiple tenants share the same database
-- 6. Regular cleanup of expired refresh tokens recommended
-- 7. Consider encrypting sensitive columns (email, etc.) at rest

-- ============================================================================
-- MAINTENANCE NOTES
-- ============================================================================

-- Recommended maintenance tasks:
-- 1. VACUUM ANALYZE regularly for optimal query performance
-- 2. Clean up expired refresh tokens periodically
-- 3. Archive or clean up old link_discovery_jobs
-- 4. Monitor index usage and bloat
-- 5. Update pg_trgm statistics for text search optimization

-- Example cleanup query for expired tokens (run daily):
-- DELETE FROM refresh_tokens WHERE expires_at < CURRENT_TIMESTAMP;

-- Example cleanup query for old completed jobs (run weekly):
-- DELETE FROM link_discovery_jobs
-- WHERE status = 'completed' AND completed_at < CURRENT_TIMESTAMP - INTERVAL '30 days';