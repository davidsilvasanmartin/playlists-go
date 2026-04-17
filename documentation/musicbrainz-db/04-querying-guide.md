# Querying Guide

## Common Query Patterns

This guide provides practical examples for querying the MusicBrainz database.

## Artist Queries

### Get Artist by MBID

```sql
SELECT
    a.gid,
    a.name,
    a.sort_name,
    at.name as type,
    ar.name as area,
    g.name as gender,
    a.begin_date_year,
    a.end_date_year,
    a.ended
FROM artist a
LEFT JOIN artist_type at ON at.id = a.type
LEFT JOIN area ar ON ar.id = a.area
LEFT JOIN gender g ON g.id = a.gender
WHERE a.gid = 'b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d';
```

### Search Artists by Name

```sql
-- Simple search
SELECT gid, name, sort_name, comment
FROM artist
WHERE LOWER(name) LIKE LOWER('%beatles%')
LIMIT 10;

-- Full-text search (faster for large datasets)
SELECT gid, name, sort_name,
       ts_rank(to_tsvector('mb_simple', name), query) as rank
FROM artist,
     to_tsquery('mb_simple', 'beatles') query
WHERE to_tsvector('mb_simple', name) @@ query
ORDER BY rank DESC
LIMIT 10;
```

### Get Artist's Aliases

```sql
SELECT
    aa.name,
    aa.sort_name,
    aa.locale,
    aat.name as alias_type,
    aa.primary_for_locale,
    aa.begin_date_year,
    aa.end_date_year
FROM artist a
JOIN artist_alias aa ON aa.artist = a.id
LEFT JOIN artist_alias_type aat ON aat.id = aa.type
WHERE a.gid = 'b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d'
ORDER BY aa.primary_for_locale DESC, aa.name;
```

## Release Queries

### Get Releases by Artist

```sql
SELECT
    rg.gid as release_group_mbid,
    rg.name as release_group,
    rgpt.name as primary_type,
    r.gid as release_mbid,
    r.name as release_name,
    r.barcode,
    rs.name as status,
    date_year,
    date_month,
    date_day
FROM artist a
JOIN artist_credit_name acn ON acn.artist = a.id
JOIN artist_credit ac ON ac.id = acn.artist_credit
JOIN release_group rg ON rg.artist_credit = ac.id
JOIN release r ON r.release_group = rg.id
LEFT JOIN release_group_primary_type rgpt ON rgpt.id = rg.type
LEFT JOIN release_status rs ON rs.id = r.status
LEFT JOIN (
    SELECT release,
           MIN(date_year) as date_year,
           MIN(date_month) as date_month,
           MIN(date_day) as date_day
    FROM release_country
    GROUP BY release
) rc ON rc.release = r.id
WHERE a.gid = 'b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d'
ORDER BY rc.date_year, rc.date_month, rc.date_day, r.name;
```

### Get Complete Release Information

```sql
SELECT
    r.gid as release_mbid,
    r.name as release_name,
    r.barcode,
    rs.name as release_status,
    l.name as language,
    s.name as script,
    rp.name as packaging,
    array_agg(DISTINCT rc.date_year || '-' ||
              COALESCE(rc.date_month::text, '?') || '-' ||
              COALESCE(rc.date_day::text, '?')) as release_dates,
    array_agg(DISTINCT a.name) as countries
FROM release r
LEFT JOIN release_status rs ON rs.id = r.status
LEFT JOIN language l ON l.id = r.language
LEFT JOIN script s ON s.id = r.script
LEFT JOIN release_packaging rp ON rp.id = r.packaging
LEFT JOIN release_country rc ON rc.release = r.id
LEFT JOIN area a ON a.id = rc.country
WHERE r.gid = 'your-release-mbid'
GROUP BY r.id, r.name, r.barcode, rs.name, l.name, s.name, rp.name;
```

### Get Release Labels and Catalog Numbers

```sql
SELECT
    l.name as label_name,
    l.gid as label_mbid,
    rl.catalog_number
FROM release r
JOIN release_label rl ON rl.release = r.id
LEFT JOIN label l ON l.id = rl.label
WHERE r.gid = 'your-release-mbid'
ORDER BY l.name;
```

## Track and Recording Queries

### Get Complete Tracklist for a Release

```sql
SELECT
    m.position as disc_number,
    mf.name as format,
    m.name as disc_title,
    t.position as track_position,
    t.number as track_number,
    t.name as track_title,
    t.length as track_length_ms,
    rec.gid as recording_mbid,
    ac.name as artist_credit
FROM release r
JOIN medium m ON m.release = r.id
LEFT JOIN medium_format mf ON mf.id = m.format
JOIN track t ON t.medium = m.id
JOIN recording rec ON rec.id = t.recording
JOIN artist_credit ac ON ac.id = t.artist_credit
WHERE r.gid = 'your-release-mbid'
ORDER BY m.position, t.position;
```

### Get Recording with Artist Credits

```sql
SELECT
    rec.gid as recording_mbid,
    rec.name as recording_name,
    rec.length,
    rec.video,
    ac.name as artist_credit_string,
    string_agg(a.name, ' & ' ORDER BY acn.position) as individual_artists
FROM recording rec
JOIN artist_credit ac ON ac.id = rec.artist_credit
JOIN artist_credit_name acn ON acn.artist_credit = ac.id
JOIN artist a ON a.id = acn.artist
WHERE rec.gid = 'your-recording-mbid'
GROUP BY rec.id, rec.gid, rec.name, rec.length, rec.video, ac.name;
```

### Get ISRCs for a Recording

```sql
SELECT
    i.isrc,
    i.created
FROM recording r
JOIN isrc i ON i.recording = r.id
WHERE r.gid = 'your-recording-mbid';
```

### Get All Recordings of a Work

```sql
SELECT
    rec.gid as recording_mbid,
    rec.name as recording_name,
    rec.length,
    ac.name as artist_credit,
    COUNT(DISTINCT t.id) as appears_on_tracks
FROM work w
JOIN l_recording_work lrw ON lrw.entity1 = w.id
JOIN recording rec ON rec.id = lrw.entity0
JOIN artist_credit ac ON ac.id = rec.artist_credit
LEFT JOIN track t ON t.recording = rec.id
WHERE w.gid = 'your-work-mbid'
GROUP BY rec.id, rec.gid, rec.name, rec.length, ac.name
ORDER BY COUNT(DISTINCT t.id) DESC;
```

## Work Queries

### Get Work Information

```sql
SELECT
    w.gid,
    w.name,
    wt.name as work_type,
    w.comment,
    array_agg(DISTINCT iswc.iswc) FILTER (WHERE iswc.iswc IS NOT NULL) as iswcs
FROM work w
LEFT JOIN work_type wt ON wt.id = w.type
LEFT JOIN iswc ON iswc.work = w.id
WHERE w.gid = 'your-work-mbid'
GROUP BY w.id, w.gid, w.name, wt.name, w.comment;
```

### Get Composers and Lyricists for a Work

```sql
SELECT
    a.name as artist_name,
    a.gid as artist_mbid,
    lt.name as role
FROM work w
JOIN l_artist_work law ON law.entity1 = w.id
JOIN artist a ON a.id = law.entity0
JOIN link l ON l.id = law.link
JOIN link_type lt ON lt.id = l.link_type
WHERE w.gid = 'your-work-mbid'
  AND lt.name IN ('composer', 'lyricist', 'writer')
ORDER BY lt.name, a.sort_name;
```

## Relationship Queries

### Get All Performers on a Recording

```sql
SELECT
    a.name as performer,
    a.gid as artist_mbid,
    lt.name as relationship,
    lat.name as instrument_or_vocal,
    latv.text_value as additional_info,
    lar.entity0_credit as credited_as
FROM recording rec
JOIN l_artist_recording lar ON lar.entity1 = rec.id
JOIN artist a ON a.id = lar.entity0
JOIN link l ON l.id = lar.link
JOIN link_type lt ON lt.id = l.link_type
LEFT JOIN link_attribute la ON la.link = l.id
LEFT JOIN link_attribute_type lat ON lat.id = la.attribute_type
LEFT JOIN link_attribute_text_value latv
    ON latv.link = l.id AND latv.attribute_type = la.attribute_type
WHERE rec.gid = 'your-recording-mbid'
  AND lt.entity_type0 = 'artist'
  AND lt.entity_type1 = 'recording'
ORDER BY
    CASE lt.name
        WHEN 'performer' THEN 1
        WHEN 'vocal' THEN 2
        WHEN 'instrument' THEN 3
        ELSE 4
    END,
    lar.link_order;
```

### Get Band Members with Timeline

```sql
SELECT
    member.name as member_name,
    member.gid as member_mbid,
    l.begin_date_year || '-' ||
        COALESCE(l.begin_date_month::text, '??') || '-' ||
        COALESCE(l.begin_date_day::text, '??') as joined_date,
    CASE
        WHEN l.ended THEN
            l.end_date_year || '-' ||
            COALESCE(l.end_date_month::text, '??') || '-' ||
            COALESCE(l.end_date_day::text, '??')
        ELSE 'present'
    END as left_date,
    lat.name as instrument
FROM artist band
JOIN l_artist_artist laa ON laa.entity1 = band.id
JOIN artist member ON member.id = laa.entity0
JOIN link l ON l.id = laa.link
JOIN link_type lt ON lt.id = l.link_type
LEFT JOIN link_attribute la ON la.link = l.id
LEFT JOIN link_attribute_type lat ON lat.id = la.attribute_type
WHERE band.gid = 'your-band-mbid'
  AND lt.name = 'member of band'
ORDER BY l.begin_date_year, l.begin_date_month, member.sort_name;
```

### Get URLs for an Entity

```sql
-- For artist
SELECT
    u.url,
    lt.name as url_type
FROM artist a
JOIN l_artist_url lau ON lau.entity0 = a.id
JOIN url u ON u.id = lau.entity1
JOIN link l ON l.id = lau.link
JOIN link_type lt ON lt.id = l.link_type
WHERE a.gid = 'your-artist-mbid'
ORDER BY
    CASE lt.name
        WHEN 'official homepage' THEN 1
        WHEN 'wikipedia' THEN 2
        WHEN 'discogs' THEN 3
        WHEN 'allmusic' THEN 4
        ELSE 5
    END,
    lt.name;
```

## Tag Queries

### Get Popular Tags for an Entity

```sql
-- For artist
SELECT
    t.name as tag,
    at.count,
    t.ref_count as global_usage
FROM artist a
JOIN artist_tag at ON at.artist = a.id
JOIN tag t ON t.id = at.tag
WHERE a.gid = 'your-artist-mbid'
ORDER BY at.count DESC
LIMIT 20;
```

### Find Artists by Tag

```sql
SELECT
    a.gid,
    a.name,
    at.count as tag_count
FROM tag t
JOIN artist_tag at ON at.tag = t.id
JOIN artist a ON a.id = at.artist
WHERE t.name = 'rock'
  AND at.count > 5
ORDER BY at.count DESC
LIMIT 50;
```

## Advanced Queries

### Get Complete Album Information (Everything)

```sql
-- This is a complex query that gets nearly everything about a release
WITH release_info AS (
    SELECT
        r.id,
        r.gid as release_mbid,
        r.name as release_name,
        r.barcode,
        rg.gid as release_group_mbid,
        rg.name as release_group_name,
        rgpt.name as primary_type,
        rs.name as status,
        ac.name as artist_credit
    FROM release r
    JOIN release_group rg ON rg.id = r.release_group
    JOIN artist_credit ac ON ac.id = r.artist_credit
    LEFT JOIN release_group_primary_type rgpt ON rgpt.id = rg.type
    LEFT JOIN release_status rs ON rs.id = r.status
    WHERE r.gid = 'your-release-mbid'
),
tracklist AS (
    SELECT
        ri.id as release_id,
        m.position as disc,
        mf.name as format,
        t.position,
        t.number,
        t.name as track_name,
        t.length,
        rec.gid as recording_mbid,
        tac.name as track_artist_credit
    FROM release_info ri
    JOIN medium m ON m.release = ri.id
    LEFT JOIN medium_format mf ON mf.id = m.format
    JOIN track t ON t.medium = m.id
    JOIN recording rec ON rec.id = t.recording
    JOIN artist_credit tac ON tac.id = t.artist_credit
),
labels AS (
    SELECT
        ri.id as release_id,
        json_agg(
            json_build_object(
                'label', l.name,
                'catalog_number', rl.catalog_number
            )
        ) as label_info
    FROM release_info ri
    JOIN release_label rl ON rl.release = ri.id
    LEFT JOIN label l ON l.id = rl.label
    GROUP BY ri.id
)
SELECT
    ri.*,
    json_agg(
        json_build_object(
            'disc', tl.disc,
            'format', tl.format,
            'position', tl.position,
            'number', tl.number,
            'name', tl.track_name,
            'length', tl.length,
            'recording_mbid', tl.recording_mbid,
            'artist_credit', tl.track_artist_credit
        ) ORDER BY tl.disc, tl.position
    ) as tracks,
    lb.label_info
FROM release_info ri
JOIN tracklist tl ON tl.release_id = ri.id
LEFT JOIN labels lb ON lb.release_id = ri.id
GROUP BY ri.id, ri.release_mbid, ri.release_name, ri.barcode,
         ri.release_group_mbid, ri.release_group_name,
         ri.primary_type, ri.status, ri.artist_credit, lb.label_info;
```

### Search for Recordings by Duration

```sql
-- Find recordings within a duration range (±5 seconds)
SELECT
    rec.gid,
    rec.name,
    rec.length,
    ac.name as artist_credit
FROM recording rec
JOIN artist_credit ac ON ac.id = rec.artist_credit
WHERE rec.length BETWEEN 180000 AND 190000  -- 3:00 to 3:10 (milliseconds)
ORDER BY rec.length
LIMIT 50;
```

### Find Cover Versions of a Work

```sql
WITH original_recordings AS (
    SELECT rec.id
    FROM work w
    JOIN l_recording_work lrw ON lrw.entity1 = w.id
    JOIN recording rec ON rec.id = lrw.entity0
    JOIN artist_credit ac ON ac.id = rec.artist_credit
    WHERE w.gid = 'your-work-mbid'
      AND ac.name = 'Original Artist Name'
)
SELECT
    rec.gid as recording_mbid,
    rec.name as recording_name,
    ac.name as artist_credit,
    COUNT(DISTINCT t.id) as release_count
FROM work w
JOIN l_recording_work lrw ON lrw.entity1 = w.id
JOIN recording rec ON rec.id = lrw.entity0
JOIN artist_credit ac ON ac.id = rec.artist_credit
LEFT JOIN track t ON t.recording = rec.id
WHERE w.gid = 'your-work-mbid'
  AND rec.id NOT IN (SELECT id FROM original_recordings)
GROUP BY rec.id, rec.gid, rec.name, ac.name
ORDER BY release_count DESC;
```

## Performance Tips

1. **Always use MBID (gid) when possible** - It's indexed and unique
2. **Filter early** - Put WHERE clauses on indexed columns
3. **Use LIMIT** - Especially for exploratory queries
4. **Avoid SELECT *** - Specify only needed columns
5. **Use EXPLAIN ANALYZE** - To understand query performance
6. **Consider materialized views** - For frequently-run aggregate queries
7. **Use appropriate JOINs** - LEFT JOIN only when nullable
8. **Index usage** - Queries on name fields should use indexes or full-text search

## Example: EXPLAIN ANALYZE

```sql
EXPLAIN ANALYZE
SELECT a.name, COUNT(r.id)
FROM artist a
JOIN artist_credit_name acn ON acn.artist = a.id
JOIN release_group rg ON rg.artist_credit = acn.artist_credit
JOIN release r ON r.release_group = rg.id
WHERE a.gid = 'your-artist-mbid'
GROUP BY a.name;
```
