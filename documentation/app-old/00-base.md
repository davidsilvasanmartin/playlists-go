### Project Overview

* **Project Name:** "Playlists" (this will change later)
* **Purpose:** Saving playlists of music in a way that can be persisted forever (unlike with online services such as
  YouTube or Spotify, where they can delete songs without any notice)
* **Target Audience:** Music lovers
* **Overall architecture:** Traditional web application. Backend is a REST API written in Go. Frontend is an Angular
  application. Database is PostgreSQL, running in Docker for development.

### Core Features

* User registration, authentication and authorization. Roles USER and ADMIN.
* Users can save songs. When a user tries to save a song, Musicbrainz is checked for that song. If there are several
  alternatives found, the app will show the user the list of matched songs so he can choose one. Once the users selects
  a song, the app must then fetch all metadata possible from Musicbrainz regarding that song and save it into the database. The app will save song, artist, album, and metadata such as genre, data, etc.
* Users later organise those songs into playlists.
* Users can add tags to songs.
* Users can create "programmatically defined" playlists, e.g. by genre, by decade, by tag...
* After a song is added, there will be a background process to find links to websites (e.g. YouTube, Soundcloud etc) where that song can be played. All found links will be saved into the database. Some sites such as Spotify will only be enabled after the user introduces the API key to use that service. We will use the API of those services (as opposed to scraping their websites). To start with, we will only support YouTube.
* The front-end allows the user to play playlists.

### Future Features - To be implemented slowly after release

* User roles and full role-based access control.
* Support SoundCloud, Spotify, and other services to play songs.
* Support Navidrome (locally hosted music server) to play songs.
* The user can rate songs in the playlist. The user can rate its own playlists.
* The user can share playlists with other users.
* The user can search for playlists from all users by song title, artist, or album.
* The user can follow other users and see their playlists.
* The user can create public or private playlists.
* The user can import/export playlists in a standard format (JSON, XML).
* The user can rate other users' playlists.
* [AI feature] The user can receive recommendations based on their playlists.

### Non-Functional Requirements

* **Deployment:** This project is a self-hosted app. It is meant to be deployed as a Docker compose project.
* **Scalability:** Must support up to 100 concurrent users.
