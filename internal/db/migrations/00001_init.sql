-- +goose Up

CREATE TYPE entity_type AS ENUM ('artist', 'album', 'song');
CREATE TYPE source_type AS ENUM ('ytmusic', 'spotify', 'bandcamp', 'musicbrainz', 'lastfm', 'deezer', 'subsonic', 'unknown');
CREATE TYPE correlation_method AS ENUM ('source_id', 'fuzzy_name', 'manual');
CREATE TYPE import_service AS ENUM ('spotify', 'ytmusic', 'lastfm', 'listenbrainz', 'maloja', 'canto_export');
CREATE TYPE import_status AS ENUM ('queued', 'running', 'completed', 'failed', 'cancelled');

CREATE TABLE users (
    id             BIGSERIAL PRIMARY KEY,
    username       TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,
    public_stats   BOOLEAN NOT NULL DEFAULT FALSE,
    is_admin       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_settings (
    user_id   BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    settings  JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE api_keys (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    key_hash      TEXT NOT NULL,
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX api_keys_key_hash_idx ON api_keys (key_hash);
CREATE INDEX api_keys_user_id_idx ON api_keys (user_id);

CREATE TABLE sessions (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sessions_token_hash_idx ON sessions (token_hash);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE TABLE invites (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    max_uses    INTEGER,
    uses_count  INTEGER NOT NULL DEFAULT 0,
    created_by  BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE artists (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    name_normalized TEXT NOT NULL,
    description     TEXT,
    image_id        UUID,
    pinned          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX artists_name_normalized_idx ON artists (name_normalized);
CREATE INDEX artists_updated_at_idx ON artists (updated_at);

CREATE TABLE albums (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    name_normalized TEXT NOT NULL,
    release_date    DATE,
    description     TEXT,
    image_id        UUID,
    pinned          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX albums_name_normalized_idx ON albums (name_normalized);
CREATE INDEX albums_updated_at_idx ON albums (updated_at);

CREATE TABLE songs (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    name_normalized TEXT NOT NULL,
    duration_ms     INTEGER,
    image_id        UUID,
    pinned          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX songs_name_normalized_idx ON songs (name_normalized);
CREATE INDEX songs_updated_at_idx ON songs (updated_at);

CREATE TABLE album_artists (
    album_id  BIGINT NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    artist_id BIGINT NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
    position  SMALLINT NOT NULL DEFAULT 0,
    PRIMARY KEY (album_id, artist_id)
);

CREATE INDEX album_artists_artist_id_idx ON album_artists (artist_id);

CREATE TABLE song_artists (
    song_id   BIGINT NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
    artist_id BIGINT NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
    position  SMALLINT NOT NULL DEFAULT 0,
    PRIMARY KEY (song_id, artist_id)
);

CREATE INDEX song_artists_artist_id_idx ON song_artists (artist_id);

CREATE TABLE song_albums (
    song_id      BIGINT NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
    album_id     BIGINT NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    track_number INTEGER,
    PRIMARY KEY (song_id, album_id)
);

CREATE INDEX song_albums_album_id_idx ON song_albums (album_id);

CREATE TABLE sources (
    id                  BIGSERIAL PRIMARY KEY,
    entity_type         entity_type NOT NULL,
    entity_id           BIGINT NOT NULL,
    source_type         source_type NOT NULL,
    raw_url             TEXT,
    extracted_id        TEXT,
    correlation_method  correlation_method NOT NULL,
    confidence          REAL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX sources_source_type_extracted_id_idx ON sources (source_type, extracted_id) WHERE extracted_id IS NOT NULL;
CREATE INDEX sources_entity_idx ON sources (entity_type, entity_id);

CREATE TABLE entity_aliases (
    id          BIGSERIAL PRIMARY KEY,
    entity_type entity_type NOT NULL,
    entity_id   BIGINT NOT NULL,
    alias       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_type, entity_id, alias)
);

CREATE INDEX entity_aliases_entity_idx ON entity_aliases (entity_type, entity_id);

CREATE TABLE listens (
    id                   BIGSERIAL PRIMARY KEY,
    user_id              BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    song_id              BIGINT NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
    listened_at          TIMESTAMPTZ NOT NULL,
    client               TEXT,
    duration_played_ms   INTEGER,
    extra                JSONB NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX listens_user_id_listened_at_idx ON listens (user_id, listened_at DESC);
CREATE INDEX listens_song_id_listened_at_idx ON listens (song_id, listened_at DESC);
CREATE INDEX listens_listened_at_idx ON listens (listened_at DESC);

CREATE TABLE now_playing (
    user_id      BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    song_id      BIGINT NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms  INTEGER
);

CREATE TABLE artist_blacklist (
    user_id     BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    artist_id   BIGINT NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, artist_id)
);

CREATE TABLE import_jobs (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    batch_id         UUID NOT NULL,
    filename         TEXT NOT NULL,
    service          import_service NOT NULL,
    status           import_status NOT NULL DEFAULT 'queued',
    total_items      INTEGER NOT NULL DEFAULT 0,
    processed_items  INTEGER NOT NULL DEFAULT 0,
    imported_items   INTEGER NOT NULL DEFAULT 0,
    skipped_items    INTEGER NOT NULL DEFAULT 0,
    failed_items     INTEGER NOT NULL DEFAULT 0,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);

CREATE INDEX import_jobs_user_id_idx ON import_jobs (user_id);
CREATE INDEX import_jobs_batch_id_idx ON import_jobs (batch_id);
CREATE INDEX import_jobs_status_idx ON import_jobs (status);

-- +goose Down

DROP TABLE import_jobs;
DROP TABLE artist_blacklist;
DROP TABLE now_playing;
DROP TABLE listens;
DROP TABLE entity_aliases;
DROP TABLE sources;
DROP TABLE song_albums;
DROP TABLE song_artists;
DROP TABLE album_artists;
DROP TABLE songs;
DROP TABLE albums;
DROP TABLE artists;
DROP TABLE invites;
DROP TABLE sessions;
DROP TABLE api_keys;
DROP TABLE user_settings;
DROP TABLE users;
DROP TYPE import_status;
DROP TYPE import_service;
DROP TYPE correlation_method;
DROP TYPE source_type;
DROP TYPE entity_type;
