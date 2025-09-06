CREATE TABLE IF NOT EXISTS users (
    user_id INT PRIMARY KEY,
    username TEXT
);

CREATE TABLE IF NOT EXISTS songs (
    id TEXT PRIMARY KEY,
    title TEXT,
    description TEXT,
    author TEXT,
    views INT,
    duration INTERVAL,
    publishdate TIMESTAMP,
    url TEXT
);

CREATE TABLE IF NOT EXISTS playlists (
    user_id INT REFERENCES users(user_id) ON DELETE CASCADE,
    song_id TEXT REFERENCES songs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS playlists_by_user ON playlists(user_id);
CREATE INDEX IF NOT EXISTS playlists_by_song ON playlists(song_id);