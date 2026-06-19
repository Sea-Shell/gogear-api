-- Drop loadouts table

DROP INDEX IF EXISTS idx_loadouts_public;
DROP INDEX IF EXISTS idx_loadouts_user;
DROP INDEX IF EXISTS idx_loadouts_slug;
DROP TABLE IF EXISTS loadouts;
