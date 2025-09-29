CREATE TABLE IF NOT EXISTS users (
    user_id BIGINT PRIMARY KEY,
    username TEXT
);

CREATE TABLE IF NOT EXISTS songs (
    id TEXT PRIMARY KEY,
    title TEXT,
    author TEXT,
    views INT,
    description TEXT,
    duration INT,
    publish_date TIMESTAMP,
    url TEXT
);

CREATE TABLE IF NOT EXISTS playlists (
    user_id BIGINT REFERENCES users(user_id) ON DELETE CASCADE,
    song_id TEXT REFERENCES songs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS playlists_by_user ON playlists(user_id);
CREATE INDEX IF NOT EXISTS playlists_by_song ON playlists(song_id);