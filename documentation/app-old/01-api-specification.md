# Playlists REST API Specification

## Version: 1.0.0

## Table of Contents

1. [Overview](#overview)
2. [General API Information](#general-api-information)
3. [Authentication](#authentication)
4. [Error Handling](#error-handling)
5. [Endpoints](#endpoints)
   - [Authentication & Users](#authentication--users)
   - [Songs](#songs)
   - [Playlists](#playlists)
   - [Tags](#tags)
   - [Artists](#artists)
   - [Albums](#albums)
   - [Playable Links](#playable-links)
6. [Data Models](#data-models)

---

## Overview

The Playlists API provides a comprehensive REST interface for managing music collections, playlists, and metadata. The API integrates with MusicBrainz for song metadata enrichment and supports background processes for discovering playable content links.

### Base URL

```
https://api.playlists.example.com/v1
```

For local development:
```
http://localhost:8080/api/v1
```

---

## General API Information

### HTTP Methods

- `GET` - Retrieve resources
- `POST` - Create new resources
- `PUT` - Replace entire resources
- `PATCH` - Partially update resources
- `DELETE` - Remove resources

### Content Type

All requests and responses use JSON:
```
Content-Type: application/json
```

### API Versioning

API version is specified in the URL path (`/v1`). Major breaking changes will increment the version number.

### Pagination

List endpoints support pagination using query parameters:

```
GET /api/v1/songs?page=1&limit=20
```

**Parameters:**
- `page` (integer, default: 1) - Page number (1-indexed)
- `limit` (integer, default: 20, max: 100) - Items per page

**Response Format:**
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 150,
    "totalPages": 8,
    "hasNext": true,
    "hasPrev": false
  }
}
```

### Sorting

Use the `sort` query parameter:
```
GET /api/v1/songs?sort=-createdAt,title
```

- Prefix with `-` for descending order
- Multiple sort fields separated by commas

### Filtering

Use query parameters for filtering:
```
GET /api/v1/songs?genre=rock&year=2020
```

### Rate Limiting

- Rate limits are applied per user/IP address
- Current limit: 1000 requests per hour for authenticated users, 100 for unauthenticated

**Response Headers:**
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 950
X-RateLimit-Reset: 1640000000
```

When rate limit is exceeded, the API returns `429 Too Many Requests`.

---

## Authentication

The API uses JWT (JSON Web Tokens) for authentication.

### Authentication Flow

1. Register or login to receive access and refresh tokens
2. Include access token in the `Authorization` header for subsequent requests
3. Use refresh token to obtain new access token when expired

### Authorization Header Format

```
Authorization: Bearer <access_token>
```

### Token Expiration

- Access tokens: 15 minutes
- Refresh tokens: 7 days

---

## Error Handling

### Standard Error Response Format

```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "The requested song was not found",
    "details": {
      "resource": "song",
      "id": "123e4567-e89b-12d3-a456-426614174000"
    },
    "timestamp": "2025-12-19T10:30:00Z",
    "path": "/api/v1/songs/123e4567-e89b-12d3-a456-426614174000"
  }
}
```

### HTTP Status Codes

| Code | Description |
|------|-------------|
| 200 | OK - Request succeeded |
| 201 | Created - Resource successfully created |
| 204 | No Content - Request succeeded with no response body |
| 400 | Bad Request - Invalid request format or parameters |
| 401 | Unauthorized - Authentication required or failed |
| 403 | Forbidden - Authenticated but lacking permissions |
| 404 | Not Found - Resource does not exist |
| 409 | Conflict - Resource conflict (e.g., duplicate) |
| 422 | Unprocessable Entity - Validation errors |
| 429 | Too Many Requests - Rate limit exceeded |
| 500 | Internal Server Error - Server error |
| 503 | Service Unavailable - Service temporarily unavailable |

### Common Error Codes

| Error Code | Description |
|------------|-------------|
| `INVALID_REQUEST` | Request validation failed |
| `AUTHENTICATION_REQUIRED` | Authentication token missing |
| `INVALID_TOKEN` | Token is invalid or expired |
| `INSUFFICIENT_PERMISSIONS` | User lacks required permissions |
| `RESOURCE_NOT_FOUND` | Requested resource does not exist |
| `RESOURCE_ALREADY_EXISTS` | Resource with same identifier exists |
| `VALIDATION_ERROR` | Input validation failed |
| `EXTERNAL_SERVICE_ERROR` | External service (MusicBrainz, YouTube) error |
| `RATE_LIMIT_EXCEEDED` | Too many requests |
| `INTERNAL_ERROR` | Unexpected server error |

### Validation Error Response

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": {
      "fields": [
        {
          "field": "email",
          "message": "Invalid email format",
          "value": "invalid-email"
        },
        {
          "field": "password",
          "message": "Password must be at least 8 characters",
          "value": null
        }
      ]
    },
    "timestamp": "2025-12-19T10:30:00Z",
    "path": "/api/v1/auth/register"
  }
}
```

---

## Endpoints

### Authentication & Users

#### POST /auth/register

Register a new user account.

**Authentication:** Not required

**Request Body:**
```json
{
  "username": "johndoe",
  "email": "john.doe@example.com",
  "password": "SecurePass123!",
  "displayName": "John Doe"
}
```

**Validation Rules:**
- `username`: 3-30 characters, alphanumeric and underscore only, unique
- `email`: Valid email format, unique
- `password`: Minimum 8 characters, must contain uppercase, lowercase, and number
- `displayName`: 1-100 characters

**Success Response (201 Created):**
```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "johndoe",
    "email": "john.doe@example.com",
    "displayName": "John Doe",
    "role": "USER",
    "createdAt": "2025-12-19T10:30:00Z",
    "updatedAt": "2025-12-19T10:30:00Z"
  },
  "tokens": {
    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expiresIn": 900
  }
}
```

**Error Responses:**

409 Conflict - Username or email already exists:
```json
{
  "error": {
    "code": "RESOURCE_ALREADY_EXISTS",
    "message": "A user with this email already exists",
    "details": {
      "field": "email",
      "value": "john.doe@example.com"
    }
  }
}
```

---

#### POST /auth/login

Authenticate and receive tokens.

**Authentication:** Not required

**Request Body:**
```json
{
  "email": "john.doe@example.com",
  "password": "SecurePass123!"
}
```

**Success Response (200 OK):**
```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "johndoe",
    "email": "john.doe@example.com",
    "displayName": "John Doe",
    "role": "USER",
    "createdAt": "2025-12-19T10:30:00Z",
    "lastLoginAt": "2025-12-19T10:30:00Z"
  },
  "tokens": {
    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expiresIn": 900
  }
}
```

**Error Responses:**

401 Unauthorized - Invalid credentials:
```json
{
  "error": {
    "code": "INVALID_CREDENTIALS",
    "message": "Invalid email or password"
  }
}
```

---

#### POST /auth/refresh

Refresh access token using refresh token.

**Authentication:** Not required (uses refresh token)

**Request Body:**
```json
{
  "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Success Response (200 OK):**
```json
{
  "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expiresIn": 900
}
```

**Error Responses:**

401 Unauthorized - Invalid or expired refresh token:
```json
{
  "error": {
    "code": "INVALID_TOKEN",
    "message": "Refresh token is invalid or expired"
  }
}
```

---

#### POST /auth/logout

Logout and invalidate tokens.

**Authentication:** Required

**Request Body:**
```json
{
  "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Success Response (204 No Content)**

---

#### GET /users/me

Get current authenticated user's profile.

**Authentication:** Required

**Success Response (200 OK):**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "username": "johndoe",
  "email": "john.doe@example.com",
  "displayName": "John Doe",
  "role": "USER",
  "stats": {
    "totalSongs": 245,
    "totalPlaylists": 12,
    "totalTags": 8
  },
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T10:30:00Z",
  "lastLoginAt": "2025-12-19T10:30:00Z"
}
```

---

#### PATCH /users/me

Update current user's profile.

**Authentication:** Required

**Request Body:**
```json
{
  "displayName": "John D.",
  "email": "newemail@example.com"
}
```

**Success Response (200 OK):**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "username": "johndoe",
  "email": "newemail@example.com",
  "displayName": "John D.",
  "role": "USER",
  "updatedAt": "2025-12-19T11:00:00Z"
}
```

---

#### GET /users/{userId}

Get user profile by ID (public information only, unless admin).

**Authentication:** Optional (more info if authenticated)

**Path Parameters:**
- `userId` (UUID) - User ID

**Success Response (200 OK):**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "username": "johndoe",
  "displayName": "John Doe",
  "stats": {
    "totalPublicPlaylists": 5
  },
  "createdAt": "2025-12-19T10:30:00Z"
}
```

---

### Songs

#### POST /songs/search

Search for songs in MusicBrainz database.

**Authentication:** Required

**Request Body:**
```json
{
  "query": "Bohemian Rhapsody Queen",
  "artist": "Queen",
  "title": "Bohemian Rhapsody",
  "limit": 10
}
```

**Query Parameters:**
- At least one of: `query`, `artist`, or `title` is required
- `limit` (optional, default: 10, max: 50)

**Success Response (200 OK):**
```json
{
  "results": [
    {
      "musicbrainzId": "b1a9c0e9-d987-4042-ae91-78d6a3267d69",
      "title": "Bohemian Rhapsody",
      "artist": {
        "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
        "name": "Queen"
      },
      "album": {
        "musicbrainzId": "6defd963-fe91-4550-b18e-82c685603c2b",
        "title": "A Night at the Opera"
      },
      "duration": 354,
      "releaseDate": "1975-10-31",
      "genres": ["Progressive Rock", "Hard Rock"],
      "score": 100
    },
    {
      "musicbrainzId": "different-recording-id",
      "title": "Bohemian Rhapsody",
      "artist": {
        "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
        "name": "Queen"
      },
      "album": {
        "musicbrainzId": "live-album-id",
        "title": "Live Killers"
      },
      "duration": 365,
      "releaseDate": "1979-06-22",
      "genres": ["Progressive Rock"],
      "score": 95
    }
  ],
  "total": 2
}
```

**Error Responses:**

503 Service Unavailable - MusicBrainz service error:
```json
{
  "error": {
    "code": "EXTERNAL_SERVICE_ERROR",
    "message": "MusicBrainz service is temporarily unavailable",
    "details": {
      "service": "musicbrainz"
    }
  }
}
```

---

#### POST /songs

Add a song to the user's library using MusicBrainz data.

**Authentication:** Required

**Request Body:**
```json
{
  "musicbrainzRecordingId": "b1a9c0e9-d987-4042-ae91-78d6a3267d69"
}
```

**Success Response (201 Created):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "musicbrainzRecordingId": "b1a9c0e9-d987-4042-ae91-78d6a3267d69",
  "title": "Bohemian Rhapsody",
  "duration": 354,
  "releaseDate": "1975-10-31",
  "artist": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
    "name": "Queen",
    "sortName": "Queen",
    "country": "GB",
    "type": "Group"
  },
  "album": {
    "id": "770e8400-e29b-41d4-a716-446655440002",
    "musicbrainzId": "6defd963-fe91-4550-b18e-82c685603c2b",
    "title": "A Night at the Opera",
    "releaseDate": "1975-11-21",
    "coverArtUrl": "https://coverartarchive.org/release/..."
  },
  "genres": ["Progressive Rock", "Hard Rock"],
  "tags": [],
  "links": {
    "status": "pending",
    "count": 0
  },
  "addedBy": "123e4567-e89b-12d3-a456-426614174000",
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T10:30:00Z"
}
```

**Error Responses:**

409 Conflict - Song already in library:
```json
{
  "error": {
    "code": "RESOURCE_ALREADY_EXISTS",
    "message": "This song is already in your library",
    "details": {
      "songId": "550e8400-e29b-41d4-a716-446655440000"
    }
  }
}
```

---

#### GET /songs

List songs in the user's library.

**Authentication:** Required

**Query Parameters:**
- `page` (integer, default: 1)
- `limit` (integer, default: 20, max: 100)
- `sort` (string, default: "-createdAt") - Options: `title`, `artist`, `album`, `createdAt`, `releaseDate`
- `search` (string) - Search in title, artist, album
- `genre` (string) - Filter by genre
- `artist` (string) - Filter by artist name
- `album` (string) - Filter by album name
- `tagId` (UUID) - Filter by tag ID
- `year` (integer) - Filter by release year

**Example Request:**
```
GET /songs?search=queen&sort=title&limit=10
```

**Success Response (200 OK):**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Bohemian Rhapsody",
      "artist": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "name": "Queen"
      },
      "album": {
        "id": "770e8400-e29b-41d4-a716-446655440002",
        "title": "A Night at the Opera",
        "coverArtUrl": "https://coverartarchive.org/release/..."
      },
      "duration": 354,
      "releaseDate": "1975-10-31",
      "genres": ["Progressive Rock", "Hard Rock"],
      "tags": [
        {
          "id": "880e8400-e29b-41d4-a716-446655440003",
          "name": "Favorites",
          "color": "#FF5733"
        }
      ],
      "links": {
        "status": "completed",
        "count": 3
      },
      "createdAt": "2025-12-19T10:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 245,
    "totalPages": 13,
    "hasNext": true,
    "hasPrev": false
  }
}
```

---

#### GET /songs/{songId}

Get detailed information about a specific song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID

**Success Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "musicbrainzRecordingId": "b1a9c0e9-d987-4042-ae91-78d6a3267d69",
  "title": "Bohemian Rhapsody",
  "duration": 354,
  "releaseDate": "1975-10-31",
  "artist": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
    "name": "Queen",
    "sortName": "Queen",
    "country": "GB",
    "type": "Group",
    "disambiguation": "",
    "lifeSpan": {
      "begin": "1970",
      "end": null
    }
  },
  "album": {
    "id": "770e8400-e29b-41d4-a716-446655440002",
    "musicbrainzId": "6defd963-fe91-4550-b18e-82c685603c2b",
    "title": "A Night at the Opera",
    "releaseDate": "1975-11-21",
    "trackCount": 12,
    "trackPosition": 11,
    "coverArtUrl": "https://coverartarchive.org/release/...",
    "label": "EMI"
  },
  "genres": ["Progressive Rock", "Hard Rock"],
  "tags": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "name": "Favorites",
      "color": "#FF5733"
    }
  ],
  "playableLinks": [
    {
      "id": "990e8400-e29b-41d4-a716-446655440004",
      "service": "youtube",
      "url": "https://www.youtube.com/watch?v=fJ9rUzIMcZQ",
      "title": "Queen – Bohemian Rhapsody (Official Video)",
      "thumbnail": "https://i.ytimg.com/vi/fJ9rUzIMcZQ/default.jpg",
      "verified": true,
      "createdAt": "2025-12-19T10:35:00Z"
    }
  ],
  "playlists": [
    {
      "id": "aa0e8400-e29b-41d4-a716-446655440005",
      "name": "Classic Rock",
      "position": 5
    }
  ],
  "addedBy": "123e4567-e89b-12d3-a456-426614174000",
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T10:35:00Z"
}
```

**Error Responses:**

404 Not Found:
```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "Song not found",
    "details": {
      "songId": "550e8400-e29b-41d4-a716-446655440000"
    }
  }
}
```

---

#### DELETE /songs/{songId}

Remove a song from the user's library.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID

**Success Response (204 No Content)**

**Notes:**
- Deleting a song will remove it from all user's playlists
- Orphaned tags (tags not used by any other song) can optionally be deleted via query parameter `?deleteOrphanedTags=true`

---

#### POST /songs/{songId}/tags

Add tags to a song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID

**Request Body:**
```json
{
  "tagIds": [
    "880e8400-e29b-41d4-a716-446655440003",
    "880e8400-e29b-41d4-a716-446655440004"
  ]
}
```

**Success Response (200 OK):**
```json
{
  "songId": "550e8400-e29b-41d4-a716-446655440000",
  "tags": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "name": "Favorites",
      "color": "#FF5733"
    },
    {
      "id": "880e8400-e29b-41d4-a716-446655440004",
      "name": "Road Trip",
      "color": "#33FF57"
    }
  ]
}
```

---

#### DELETE /songs/{songId}/tags/{tagId}

Remove a tag from a song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID
- `tagId` (UUID) - Tag ID

**Success Response (204 No Content)**

---

### Playlists

#### POST /playlists

Create a new playlist.

**Authentication:** Required

**Request Body:**
```json
{
  "name": "Classic Rock Favorites",
  "description": "The best classic rock songs from the 70s and 80s",
  "type": "manual",
  "public": false
}
```

**Playlist Types:**
- `manual` - User manually adds/removes songs
- `smart` - Programmatically defined by criteria

**Validation Rules:**
- `name`: 1-200 characters, required
- `description`: 0-1000 characters, optional
- `type`: enum [`manual`, `smart`], default: `manual`
- `public`: boolean, default: `false`

**Success Response (201 Created):**
```json
{
  "id": "aa0e8400-e29b-41d4-a716-446655440005",
  "name": "Classic Rock Favorites",
  "description": "The best classic rock songs from the 70s and 80s",
  "type": "manual",
  "public": false,
  "owner": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "johndoe",
    "displayName": "John Doe"
  },
  "songCount": 0,
  "totalDuration": 0,
  "coverImages": [],
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T10:30:00Z"
}
```

---

#### POST /playlists/smart

Create a smart (programmatic) playlist.

**Authentication:** Required

**Request Body:**
```json
{
  "name": "70s Rock",
  "description": "All rock songs from the 1970s",
  "public": false,
  "criteria": {
    "genres": ["Rock", "Hard Rock", "Progressive Rock"],
    "yearRange": {
      "from": 1970,
      "to": 1979
    },
    "tags": ["880e8400-e29b-41d4-a716-446655440003"],
    "artists": ["660e8400-e29b-41d4-a716-446655440001"],
    "minDuration": 180,
    "maxDuration": 600
  },
  "sort": {
    "field": "releaseDate",
    "order": "asc"
  },
  "limit": 100
}
```

**Criteria Options:**
- `genres` (array of strings) - Include songs matching any genre
- `yearRange` (object) - Filter by release year range
- `tags` (array of UUIDs) - Include songs with any of these tags
- `artists` (array of UUIDs) - Include songs by any of these artists
- `albums` (array of UUIDs) - Include songs from any of these albums
- `minDuration` (integer) - Minimum song duration in seconds
- `maxDuration` (integer) - Maximum song duration in seconds

**Success Response (201 Created):**
```json
{
  "id": "bb0e8400-e29b-41d4-a716-446655440006",
  "name": "70s Rock",
  "description": "All rock songs from the 1970s",
  "type": "smart",
  "public": false,
  "owner": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "johndoe",
    "displayName": "John Doe"
  },
  "criteria": {
    "genres": ["Rock", "Hard Rock", "Progressive Rock"],
    "yearRange": {
      "from": 1970,
      "to": 1979
    }
  },
  "songCount": 42,
  "totalDuration": 15240,
  "coverImages": [
    "https://coverartarchive.org/release/...",
    "https://coverartarchive.org/release/...",
    "https://coverartarchive.org/release/...",
    "https://coverartarchive.org/release/..."
  ],
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T10:30:00Z"
}
```

**Notes:**
- Smart playlists are automatically updated when songs matching criteria are added/removed
- Songs in smart playlists cannot be manually reordered

---

#### GET /playlists

List user's playlists.

**Authentication:** Required

**Query Parameters:**
- `page` (integer, default: 1)
- `limit` (integer, default: 20, max: 100)
- `sort` (string, default: "-updatedAt") - Options: `name`, `createdAt`, `updatedAt`, `songCount`
- `type` (string) - Filter by type: `manual` or `smart`
- `public` (boolean) - Filter by public/private

**Success Response (200 OK):**
```json
{
  "data": [
    {
      "id": "aa0e8400-e29b-41d4-a716-446655440005",
      "name": "Classic Rock Favorites",
      "description": "The best classic rock songs from the 70s and 80s",
      "type": "manual",
      "public": false,
      "songCount": 28,
      "totalDuration": 7980,
      "coverImages": [
        "https://coverartarchive.org/release/...",
        "https://coverartarchive.org/release/...",
        "https://coverartarchive.org/release/...",
        "https://coverartarchive.org/release/..."
      ],
      "createdAt": "2025-12-19T10:30:00Z",
      "updatedAt": "2025-12-19T12:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 12,
    "totalPages": 1,
    "hasNext": false,
    "hasPrev": false
  }
}
```

---

#### GET /playlists/{playlistId}

Get detailed information about a specific playlist.

**Authentication:** Required (or public playlist)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID

**Query Parameters:**
- `includeSongs` (boolean, default: false) - Include full song list

**Success Response (200 OK):**
```json
{
  "id": "aa0e8400-e29b-41d4-a716-446655440005",
  "name": "Classic Rock Favorites",
  "description": "The best classic rock songs from the 70s and 80s",
  "type": "manual",
  "public": false,
  "owner": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "johndoe",
    "displayName": "John Doe"
  },
  "songCount": 28,
  "totalDuration": 7980,
  "coverImages": [
    "https://coverartarchive.org/release/...",
    "https://coverartarchive.org/release/...",
    "https://coverartarchive.org/release/...",
    "https://coverartarchive.org/release/..."
  ],
  "songs": [
    {
      "position": 1,
      "song": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "title": "Bohemian Rhapsody",
        "artist": {
          "id": "660e8400-e29b-41d4-a716-446655440001",
          "name": "Queen"
        },
        "album": {
          "id": "770e8400-e29b-41d4-a716-446655440002",
          "title": "A Night at the Opera",
          "coverArtUrl": "https://coverartarchive.org/release/..."
        },
        "duration": 354,
        "releaseDate": "1975-10-31"
      },
      "addedAt": "2025-12-19T10:45:00Z"
    }
  ],
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T12:00:00Z"
}
```

---

#### GET /playlists/{playlistId}/songs

Get songs in a playlist with pagination.

**Authentication:** Required (or public playlist)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID

**Query Parameters:**
- `page` (integer, default: 1)
- `limit` (integer, default: 50, max: 100)

**Success Response (200 OK):**
```json
{
  "playlistId": "aa0e8400-e29b-41d4-a716-446655440005",
  "data": [
    {
      "position": 1,
      "song": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "title": "Bohemian Rhapsody",
        "artist": {
          "id": "660e8400-e29b-41d4-a716-446655440001",
          "name": "Queen"
        },
        "album": {
          "id": "770e8400-e29b-41d4-a716-446655440002",
          "title": "A Night at the Opera",
          "coverArtUrl": "https://coverartarchive.org/release/..."
        },
        "duration": 354
      },
      "addedAt": "2025-12-19T10:45:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 28,
    "totalPages": 1,
    "hasNext": false,
    "hasPrev": false
  }
}
```

---

#### PATCH /playlists/{playlistId}

Update playlist details.

**Authentication:** Required (must be owner)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID

**Request Body:**
```json
{
  "name": "Ultimate Classic Rock",
  "description": "Updated description",
  "public": true
}
```

**Success Response (200 OK):**
```json
{
  "id": "aa0e8400-e29b-41d4-a716-446655440005",
  "name": "Ultimate Classic Rock",
  "description": "Updated description",
  "type": "manual",
  "public": true,
  "songCount": 28,
  "updatedAt": "2025-12-19T13:00:00Z"
}
```

**Notes:**
- For smart playlists, criteria can be updated which will trigger re-evaluation

---

#### DELETE /playlists/{playlistId}

Delete a playlist.

**Authentication:** Required (must be owner)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID

**Success Response (204 No Content)**

---

#### POST /playlists/{playlistId}/songs

Add songs to a playlist.

**Authentication:** Required (must be owner)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID

**Request Body:**
```json
{
  "songIds": [
    "550e8400-e29b-41d4-a716-446655440000",
    "550e8400-e29b-41d4-a716-446655440001"
  ],
  "position": 5
}
```

**Parameters:**
- `songIds` (array of UUIDs, required) - Songs to add
- `position` (integer, optional) - Insert position (1-indexed). If not specified, songs are appended to the end

**Success Response (200 OK):**
```json
{
  "playlistId": "aa0e8400-e29b-41d4-a716-446655440005",
  "added": [
    {
      "position": 5,
      "songId": "550e8400-e29b-41d4-a716-446655440000"
    },
    {
      "position": 6,
      "songId": "550e8400-e29b-41d4-a716-446655440001"
    }
  ],
  "songCount": 30
}
```

**Error Responses:**

400 Bad Request - Smart playlist:
```json
{
  "error": {
    "code": "INVALID_OPERATION",
    "message": "Cannot manually add songs to a smart playlist"
  }
}
```

409 Conflict - Song already in playlist:
```json
{
  "error": {
    "code": "RESOURCE_ALREADY_EXISTS",
    "message": "One or more songs are already in this playlist",
    "details": {
      "duplicateSongIds": ["550e8400-e29b-41d4-a716-446655440000"]
    }
  }
}
```

---

#### DELETE /playlists/{playlistId}/songs/{songId}

Remove a song from a playlist.

**Authentication:** Required (must be owner)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID
- `songId` (UUID) - Song ID

**Success Response (204 No Content)**

---

#### PUT /playlists/{playlistId}/songs/reorder

Reorder songs in a playlist.

**Authentication:** Required (must be owner)

**Path Parameters:**
- `playlistId` (UUID) - Playlist ID

**Request Body (Move single song):**
```json
{
  "songId": "550e8400-e29b-41d4-a716-446655440000",
  "fromPosition": 10,
  "toPosition": 3
}
```

**Request Body (Reorder all songs):**
```json
{
  "songIds": [
    "550e8400-e29b-41d4-a716-446655440002",
    "550e8400-e29b-41d4-a716-446655440000",
    "550e8400-e29b-41d4-a716-446655440001"
  ]
}
```

**Success Response (200 OK):**
```json
{
  "playlistId": "aa0e8400-e29b-41d4-a716-446655440005",
  "message": "Playlist reordered successfully"
}
```

---

### Tags

#### GET /tags

List all tags for the current user.

**Authentication:** Required

**Query Parameters:**
- `page` (integer, default: 1)
- `limit` (integer, default: 50, max: 100)
- `sort` (string, default: "name") - Options: `name`, `createdAt`, `songCount`

**Success Response (200 OK):**
```json
{
  "data": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "name": "Favorites",
      "color": "#FF5733",
      "songCount": 42,
      "createdAt": "2025-12-19T10:30:00Z",
      "updatedAt": "2025-12-19T10:30:00Z"
    },
    {
      "id": "880e8400-e29b-41d4-a716-446655440004",
      "name": "Road Trip",
      "color": "#33FF57",
      "songCount": 18,
      "createdAt": "2025-12-19T11:00:00Z",
      "updatedAt": "2025-12-19T11:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 8,
    "totalPages": 1,
    "hasNext": false,
    "hasPrev": false
  }
}
```

---

#### POST /tags

Create a new tag.

**Authentication:** Required

**Request Body:**
```json
{
  "name": "Workout",
  "color": "#3357FF"
}
```

**Validation Rules:**
- `name`: 1-50 characters, unique per user, required
- `color`: Valid hex color code (e.g., #RRGGBB), optional (auto-generated if not provided)

**Success Response (201 Created):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440005",
  "name": "Workout",
  "color": "#3357FF",
  "songCount": 0,
  "createdAt": "2025-12-19T14:00:00Z",
  "updatedAt": "2025-12-19T14:00:00Z"
}
```

**Error Responses:**

409 Conflict - Tag name already exists:
```json
{
  "error": {
    "code": "RESOURCE_ALREADY_EXISTS",
    "message": "A tag with this name already exists",
    "details": {
      "name": "Workout"
    }
  }
}
```

---

#### GET /tags/{tagId}

Get tag details including associated songs.

**Authentication:** Required

**Path Parameters:**
- `tagId` (UUID) - Tag ID

**Query Parameters:**
- `includeSongs` (boolean, default: false) - Include songs with this tag

**Success Response (200 OK):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "name": "Favorites",
  "color": "#FF5733",
  "songCount": 42,
  "songs": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Bohemian Rhapsody",
      "artist": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "name": "Queen"
      },
      "album": {
        "id": "770e8400-e29b-41d4-a716-446655440002",
        "title": "A Night at the Opera"
      }
    }
  ],
  "createdAt": "2025-12-19T10:30:00Z",
  "updatedAt": "2025-12-19T10:30:00Z"
}
```

---

#### PATCH /tags/{tagId}

Update tag details.

**Authentication:** Required

**Path Parameters:**
- `tagId` (UUID) - Tag ID

**Request Body:**
```json
{
  "name": "Favorite Songs",
  "color": "#FF6633"
}
```

**Success Response (200 OK):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "name": "Favorite Songs",
  "color": "#FF6633",
  "songCount": 42,
  "updatedAt": "2025-12-19T15:00:00Z"
}
```

---

#### DELETE /tags/{tagId}

Delete a tag.

**Authentication:** Required

**Path Parameters:**
- `tagId` (UUID) - Tag ID

**Success Response (204 No Content)**

**Notes:**
- Deleting a tag removes it from all songs but does not delete the songs

---

### Artists

#### GET /artists

List artists in the user's library.

**Authentication:** Required

**Query Parameters:**
- `page` (integer, default: 1)
- `limit` (integer, default: 20, max: 100)
- `sort` (string, default: "name") - Options: `name`, `songCount`, `country`
- `search` (string) - Search artist names
- `country` (string) - Filter by country code (ISO 3166-1 alpha-2)

**Success Response (200 OK):**
```json
{
  "data": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
      "name": "Queen",
      "sortName": "Queen",
      "country": "GB",
      "type": "Group",
      "songCount": 15,
      "albumCount": 8
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 45,
    "totalPages": 3,
    "hasNext": true,
    "hasPrev": false
  }
}
```

---

#### GET /artists/{artistId}

Get detailed information about an artist.

**Authentication:** Required

**Path Parameters:**
- `artistId` (UUID) - Artist ID

**Success Response (200 OK):**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
  "name": "Queen",
  "sortName": "Queen",
  "country": "GB",
  "type": "Group",
  "disambiguation": "",
  "lifeSpan": {
    "begin": "1970",
    "end": null
  },
  "songCount": 15,
  "albumCount": 8,
  "songs": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Bohemian Rhapsody",
      "album": {
        "id": "770e8400-e29b-41d4-a716-446655440002",
        "title": "A Night at the Opera"
      },
      "duration": 354,
      "releaseDate": "1975-10-31"
    }
  ],
  "albums": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "title": "A Night at the Opera",
      "releaseDate": "1975-11-21",
      "songCount": 5
    }
  ]
}
```

**Notes:**
- Only return albums and songs of that artist that exist in our database.

---

### Albums

#### GET /albums

List albums in the user's library.

**Authentication:** Required

**Query Parameters:**
- `page` (integer, default: 1)
- `limit` (integer, default: 20, max: 100)
- `sort` (string, default: "-releaseDate") - Options: `title`, `releaseDate`, `songCount`
- `search` (string) - Search album titles
- `artistId` (UUID) - Filter by artist
- `year` (integer) - Filter by release year

**Success Response (200 OK):**
```json
{
  "data": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440002",
      "musicbrainzId": "6defd963-fe91-4550-b18e-82c685603c2b",
      "title": "A Night at the Opera",
      "releaseDate": "1975-11-21",
      "artist": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "name": "Queen"
      },
      "trackCount": 12,
      "songCount": 5,
      "coverArtUrl": "https://coverartarchive.org/release/...",
      "label": "EMI"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 67,
    "totalPages": 4,
    "hasNext": true,
    "hasPrev": false
  }
}
```

---

#### GET /albums/{albumId}

Get detailed information about an album.

**Authentication:** Required

**Path Parameters:**
- `albumId` (UUID) - Album ID

**Success Response (200 OK):**
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440002",
  "musicbrainzId": "6defd963-fe91-4550-b18e-82c685603c2b",
  "title": "A Night at the Opera",
  "releaseDate": "1975-11-21",
  "artist": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "musicbrainzId": "0383dadf-2a4e-4d10-a46a-e9e041da8eb3",
    "name": "Queen"
  },
  "trackCount": 12,
  "songCount": 5,
  "coverArtUrl": "https://coverartarchive.org/release/...",
  "label": "EMI",
  "songs": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Bohemian Rhapsody",
      "trackPosition": 11,
      "duration": 354
    }
  ]
}
```

**Notes:**
- Only return songs of that album that exist in our database.


---

### Playable Links

#### GET /songs/{songId}/links

Get all playable links for a song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID

**Query Parameters:**
- `service` (string) - Filter by service (e.g., `youtube`, `soundcloud`)
- `verified` (boolean) - Filter by verification status

**Success Response (200 OK):**
```json
{
  "songId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "lastCheckedAt": "2025-12-19T10:35:00Z",
  "links": [
    {
      "id": "990e8400-e29b-41d4-a716-446655440004",
      "service": "youtube",
      "url": "https://www.youtube.com/watch?v=fJ9rUzIMcZQ",
      "title": "Queen – Bohemian Rhapsody (Official Video)",
      "description": "Official video for Bohemian Rhapsody",
      "thumbnail": "https://i.ytimg.com/vi/fJ9rUzIMcZQ/default.jpg",
      "duration": 354,
      "verified": true,
      "available": true,
      "createdAt": "2025-12-19T10:35:00Z",
      "lastCheckedAt": "2025-12-19T10:35:00Z"
    },
    {
      "id": "990e8400-e29b-41d4-a716-446655440005",
      "service": "youtube",
      "url": "https://www.youtube.com/watch?v=different-id",
      "title": "Queen - Bohemian Rhapsody (Lyrics)",
      "description": "Lyrics video",
      "thumbnail": "https://i.ytimg.com/vi/different-id/default.jpg",
      "duration": 355,
      "verified": false,
      "available": true,
      "createdAt": "2025-12-19T10:35:00Z",
      "lastCheckedAt": "2025-12-19T10:35:00Z"
    }
  ]
}
```

**Link Status:**
- `pending` - Background job not yet completed
- `processing` - Currently searching for links
- `completed` - Search completed
- `failed` - Search failed (external service error)

---

#### POST /songs/{songId}/links/refresh

Trigger a refresh of playable links for a song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID

**Success Response (202 Accepted):**
```json
{
  "songId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "message": "Link refresh job has been queued"
}
```

**Notes:**
- This operation is asynchronous
- Rate limited to prevent abuse (max 1 refresh per song per hour)

---

#### POST /songs/{songId}/links

Manually add a playable link to a song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID

**Request Body:**
```json
{
  "service": "youtube",
  "url": "https://www.youtube.com/watch?v=fJ9rUzIMcZQ"
}
```

**Success Response (201 Created):**
```json
{
  "id": "990e8400-e29b-41d4-a716-446655440006",
  "service": "youtube",
  "url": "https://www.youtube.com/watch?v=fJ9rUzIMcZQ",
  "title": "Queen – Bohemian Rhapsody (Official Video)",
  "thumbnail": "https://i.ytimg.com/vi/fJ9rUzIMcZQ/default.jpg",
  "duration": 354,
  "verified": false,
  "available": true,
  "createdAt": "2025-12-19T16:00:00Z"
}
```

**Error Responses:**

409 Conflict - Link already exists:
```json
{
  "error": {
    "code": "RESOURCE_ALREADY_EXISTS",
    "message": "This link is already associated with the song"
  }
}
```

---

#### DELETE /songs/{songId}/links/{linkId}

Remove a playable link from a song.

**Authentication:** Required

**Path Parameters:**
- `songId` (UUID) - Song ID
- `linkId` (UUID) - Link ID

**Success Response (204 No Content)**

---

## Data Models

### User

```json
{
  "id": "uuid",
  "username": "string",
  "email": "string",
  "displayName": "string",
  "role": "USER | ADMIN",
  "createdAt": "datetime",
  "updatedAt": "datetime",
  "lastLoginAt": "datetime"
}
```

### Song

```json
{
  "id": "uuid",
  "musicbrainzRecordingId": "string",
  "title": "string",
  "duration": "integer (seconds)",
  "releaseDate": "date",
  "artist": "Artist",
  "album": "Album",
  "genres": ["string"],
  "tags": ["Tag"],
  "playableLinks": ["PlayableLink"],
  "addedBy": "uuid",
  "createdAt": "datetime",
  "updatedAt": "datetime"
}
```

### Artist

```json
{
  "id": "uuid",
  "musicbrainzId": "string",
  "name": "string",
  "sortName": "string",
  "country": "string (ISO 3166-1 alpha-2)",
  "type": "Person | Group | Orchestra | Choir | Character | Other",
  "disambiguation": "string",
  "lifeSpan": {
    "begin": "string (year or date)",
    "end": "string (year or date) | null"
  }
}
```

### Album

```json
{
  "id": "uuid",
  "musicbrainzId": "string",
  "title": "string",
  "releaseDate": "date",
  "trackCount": "integer",
  "coverArtUrl": "string | null",
  "label": "string | null"
}
```

### Playlist

```json
{
  "id": "uuid",
  "name": "string",
  "description": "string | null",
  "type": "manual | smart",
  "public": "boolean",
  "owner": "User",
  "songCount": "integer",
  "totalDuration": "integer (seconds)",
  "coverImages": ["string (urls)"],
  "criteria": "object | null (for smart playlists)",
  "createdAt": "datetime",
  "updatedAt": "datetime"
}
```

### Tag

```json
{
  "id": "uuid",
  "name": "string",
  "color": "string (hex color)",
  "songCount": "integer",
  "createdAt": "datetime",
  "updatedAt": "datetime"
}
```

### PlayableLink

```json
{
  "id": "uuid",
  "service": "youtube | soundcloud | spotify",
  "url": "string",
  "title": "string",
  "description": "string | null",
  "thumbnail": "string | null",
  "duration": "integer (seconds) | null",
  "verified": "boolean",
  "available": "boolean",
  "createdAt": "datetime",
  "lastCheckedAt": "datetime"
}
```

---

## Additional Considerations

### Security

1. **Authentication:**
   - Store passwords using bcrypt with salt rounds ≥ 12
   - Implement JWT token rotation
   - Support token revocation

2. **Authorization:**
   - Role-based access control (USER, ADMIN)
   - Resource ownership validation
   - Public playlist access control

3. **Input Validation:**
   - Sanitize all user inputs
   - Validate UUIDs format
   - Enforce field length limits
   - Prevent SQL injection

4. **Rate Limiting:**
   - Per-user rate limits
   - Per-endpoint rate limits for expensive operations
   - Progressive backoff for repeated violations

### Performance

1. **Caching:**
   - Cache MusicBrainz API responses
   - Cache album cover art URLs
   - Cache smart playlist results with TTL

2. **Database:**
   - Index frequently queried fields (user_id, artist_id, album_id)
   - Use full-text search for song/artist/album searches
   - Optimize smart playlist queries

3. **Background Jobs:**
   - Asynchronous link discovery
   - Periodic link availability checks
   - Smart playlist updates

### Future Enhancements

1. **API Key Management:**
   - POST /users/me/api-keys - Manage API keys for external services
   - GET /users/me/api-keys/{service} - Get configured services

2. **Social Features (Future):**
   - POST /users/{userId}/follow - Follow users
   - GET /playlists/public - Browse public playlists
   - POST /playlists/{playlistId}/rating - Rate playlists

3. **Import/Export:**
   - POST /playlists/{playlistId}/export - Export playlist
   - POST /playlists/import - Import playlist

4. **Recommendations (AI Feature):**
   - GET /recommendations/songs - Get song recommendations
   - GET /recommendations/playlists - Get playlist recommendations

---

## Versioning and Deprecation

When breaking changes are introduced:
1. New version will be released (e.g., `/v2`)
2. Old version will be supported for minimum 6 months
3. Deprecation warnings will be sent via response headers
4. Migration guide will be provided

**Deprecation Header:**
```
X-API-Deprecation: version=v1; sunset=2026-06-01; link="https://api.playlists.example.com/docs/migration"
```