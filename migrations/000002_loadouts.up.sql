-- Create loadouts table for user gear groupings

CREATE TABLE IF NOT EXISTS loadouts (
    loadoutId INTEGER PRIMARY KEY AUTOINCREMENT,
    userId INTEGER NOT NULL,
    loadoutName TEXT NOT NULL,
    loadoutDescription TEXT NOT NULL DEFAULT '',
    loadoutIsPublic INTEGER NOT NULL DEFAULT 0,
    loadoutSlug TEXT,
    totalWeight INTEGER NOT NULL DEFAULT 0,
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updatedAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (userId) REFERENCES users(userId)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_loadouts_slug ON loadouts(loadoutSlug);
CREATE INDEX IF NOT EXISTS idx_loadouts_user ON loadouts(userId);
CREATE INDEX IF NOT EXISTS idx_loadouts_public ON loadouts(loadoutIsPublic);
