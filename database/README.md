# Database Schema Documentation

## Overview

This directory contains the PostgreSQL database schema for the Playlists application. The schema is designed to support a music library management system with MusicBrainz integration, user-defined playlists, tagging, and playable link discovery.

## Files

- `schema.sql` - Complete database schema with tables, indexes, views, and triggers

## Quick Start

### Prerequisites

- PostgreSQL 17.6+ (though 15+ should work)
- Docker Compose (for local development)

### Setup

1. Start the PostgreSQL container:
   ```bash
   docker-compose up -d database
   ```

2. Apply the schema:
   ```bash
   docker exec -i dev-davidsilva-apps-playlists-db psql -U postgres -d playlists < database/schema.sql
   ```

   Or connect directly:
   ```bash
   psql -h localhost -U postgres -d playlists -f database/schema.sql
   ```

## Schema Design

### Key Design Decisions

#### 1. **User-Specific vs. Shared Data**

- **Shared Across Users**: `artists`, `albums`
  - These come from MusicBrainz and represent canonical music metadata
  - Avoids data duplication
  - Referenced by songs via foreign keys

- **User-Specific**: `songs`, `tags`, `playlists`
  - Each user has their own library even if they add the same MusicBrainz recording
  - Allows independent organization and tagging
  - User isolation for privacy and data management

#### 2. **Smart Playlists**

Smart playlists use JSONB columns for flexible criteria:
```json
{
  "genres": ["Rock", "Progressive Rock"],
  "yearRange": {"from": 1970, "to": 1979},
  "tags": ["uuid1", "uuid2"],
  "artists": ["uuid3"],
  "albums": ["uuid4"],
  "minDuration": 180,
  "maxDuration": 600
}
```

The `playlist_songs` junction table is **only used for manual playlists**. Smart playlist contents are computed dynamically based on criteria.

#### 3. **UUID Primary Keys**

All tables use UUID v4 primary keys for:
- Global uniqueness (important for distributed systems)
- Security (non-sequential, unpredictable)
- Easier data migrations and merging

#### 4. **Timestamp Tracking**

All major tables include:
- `created_at` - When the record was created
- `updated_at` - When the record was last modified (auto-updated via trigger)

#### 5. **Link Discovery**

Playable links are discovered by background jobs:
- `link_discovery_jobs` tracks job status to prevent duplicates
- `playable_links` stores discovered links with availability tracking
- Links are periodically checked for availability

### Normalization

The schema follows **3rd Normal Form (3NF)**:
- No transitive dependencies
- Atomic values in all columns
- All non-key attributes depend on the primary key

Example: Instead of storing artist name in the `songs` table, we reference the `artists` table via `artist_id`.

### Indexes

Indexes are strategically placed for:
- Foreign key columns (for join performance)
- Columns used in WHERE clauses (filtering)
- Columns used in ORDER BY clauses (sorting)
- Text search columns (using pg_trgm trigram indexes)
- JSONB columns (using GIN indexes)

## Table Relationships

```
users (1) ----< (many) songs
users (1) ----< (many) playlists
users (1) ----< (many) tags
users (1) ----< (many) refresh_tokens

artists (1) ----< (many) songs
albums (1) ----< (many) songs

songs (many) >----< (many) tags via song_tags
songs (many) >----< (many) playlists via playlist_songs (manual only)
songs (1) ----< (many) playable_links
songs (1) ----< (many) link_discovery_jobs
```

## Views

The schema includes several materialized views for common aggregations:

- `user_stats` - Total songs, playlists, tags per user
- `playlist_stats` - Song count and total duration per playlist
- `tag_stats` - Song count per tag
- `artist_stats` - Song and album count per artist (per user)
- `album_stats` - Song count per album (per user)

These views improve API performance by pre-computing common statistics.

## Constraints

### Check Constraints

- Username: 3-30 chars, alphanumeric + underscore
- Email: Valid email format
- Password: Minimum 8 chars (enforced at application layer)
- Display name: 1-100 chars
- Tag name: 1-50 chars, unique per user
- Color: Valid hex format #RRGGBB
- Durations: Positive integers
- Positions: Positive integers (1-indexed)

### Unique Constraints

- Username (case-insensitive)
- Email (case-insensitive)
- Tag name per user (case-insensitive)
- MusicBrainz IDs (artists, albums)
- Song per user (user_id + musicbrainz_recording_id)
- Playable link per song (song_id + url)
- Position in playlist (playlist_id + position)

## Maintenance

### Regular Tasks

1. **Clean expired refresh tokens** (run daily):
   ```sql
   DELETE FROM refresh_tokens WHERE expires_at < CURRENT_TIMESTAMP;
   ```

2. **Clean old link discovery jobs** (run weekly):
   ```sql
   DELETE FROM link_discovery_jobs
   WHERE status = 'completed'
   AND completed_at < CURRENT_TIMESTAMP - INTERVAL '30 days';
   ```

3. **Vacuum and analyze** (run weekly):
   ```sql
   VACUUM ANALYZE;
   ```

4. **Check for missing indexes**:
   ```sql
   SELECT schemaname, tablename, attname, n_distinct, correlation
   FROM pg_stats
   WHERE schemaname = 'public'
   AND n_distinct > 100
   ORDER BY abs(correlation) DESC;
   ```

### Monitoring

Monitor these metrics:
- Table and index sizes
- Query performance (slow query log)
- Index usage (unused indexes)
- Lock contention
- Connection pool usage

## Security Considerations

1. **Password Storage**: Use bcrypt with salt rounds ≥ 12
2. **JWT Tokens**:
   - Access tokens: 15 min expiry (stateless)
   - Refresh tokens: 7 days expiry (stored in DB)
3. **Input Validation**: All inputs sanitized at application layer
4. **SQL Injection**: Use parameterized queries exclusively
5. **Database User**: Application should use a dedicated user with limited privileges
6. **Row-Level Security**: Consider enabling for multi-tenant scenarios
7. **Encryption**: Consider encrypting sensitive columns at rest

## Migration Strategy

For schema changes:
1. Create timestamped migration files (e.g., `001_add_rating_column.sql`)
2. Test migrations on a copy of production data
3. Use transactions for rollback capability
4. Consider backwards compatibility for zero-downtime deployments

## Performance Tips

1. **Batch Operations**: Use bulk inserts/updates when possible
2. **Connection Pooling**: Configure appropriate pool size
3. **Prepared Statements**: Cache query plans for frequently executed queries
4. **Pagination**: Always paginate large result sets
5. **Indexes**: Monitor and add indexes based on query patterns
6. **EXPLAIN ANALYZE**: Profile slow queries

## Future Enhancements

The schema is designed to accommodate future features:

- **Ratings**: Add `song_ratings` and `playlist_ratings` tables
- **Following**: Add `user_follows` junction table
- **Sharing**: Add `playlist_shares` table
- **Comments**: Add `playlist_comments` table
- **Import/Export**: Track via new `import_jobs` table
- **API Keys**: Add `user_api_keys` table for external service integration
- **Recommendations**: Add `song_recommendations` table for AI features

## Troubleshooting

### Common Issues

**Issue**: Foreign key constraint violation when deleting
**Solution**: Check ON DELETE CASCADE settings or manually delete child records

**Issue**: Slow text search queries
**Solution**: Ensure pg_trgm extension is installed and trigram indexes exist

**Issue**: JSONB query performance
**Solution**: Add GIN indexes on JSONB columns and use appropriate operators

**Issue**: Deadlocks on playlist reordering
**Solution**: Acquire locks in consistent order, consider advisory locks

## Contributing

When modifying the schema:
1. Update this README
2. Add migration files
3. Update tests
4. Document breaking changes
5. Consider backwards compatibility