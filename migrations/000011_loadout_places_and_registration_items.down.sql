-- Reverses 000011 (ARS-001 section 2.1, down migration). Lossy: places, ticks
-- and detached snapshots are discarded; legacy and registration-deleted items
-- keep their gear only when it still exists. Not on the release path.
--
-- Order matters: first drop every trigger and view that names loadout_items or
-- loadout_places, then rebuild loadout_items, then drop loadout_places.
-- Otherwise the schema check during RENAME can fail on objects that refer to
-- the old table.

DROP TRIGGER IF EXISTS trg_registration_delete_detach;
DROP TRIGGER IF EXISTS trg_registration_owner_fixed;
DROP TRIGGER IF EXISTS trg_loadouts_create_worn;
DROP TRIGGER IF EXISTS trg_loadout_items_owner_insert;
DROP TRIGGER IF EXISTS trg_loadout_items_owner_update;
DROP TRIGGER IF EXISTS trg_loadout_items_place_insert;
DROP TRIGGER IF EXISTS trg_loadout_items_place_update;
DROP TRIGGER IF EXISTS trg_loadout_places_owner_insert;
DROP TRIGGER IF EXISTS trg_loadout_places_owner_update;
DROP TRIGGER IF EXISTS trg_loadout_places_parent_insert;
DROP TRIGGER IF EXISTS trg_loadout_places_parent_update;
DROP TRIGGER IF EXISTS trg_loadout_places_no_cycle;
DROP VIEW IF EXISTS loadout_on_back_weight;
DROP VIEW IF EXISTS loadout_item_weight;
DROP VIEW IF EXISTS registration_effective_weight;

-- The stored total returns, recomputed from catalog weights.
ALTER TABLE loadouts ADD COLUMN totalWeight INTEGER NOT NULL DEFAULT 0;
UPDATE loadouts SET totalWeight = COALESCE((
    SELECT SUM(g.gearWeight * li.quantity)
    FROM loadout_items li
    LEFT JOIN user_gear_registrations r ON r.userGearRegistrationId = li.registrationId
    JOIN gear g ON g.gearId = COALESCE(r.gearId, li.detachedGearId)
    WHERE li.loadoutId = loadouts.loadoutId), 0);

-- Rebuild loadout_items as 000003 + 000005 left it, carrying the sequence.
CREATE TABLE loadout_items_v10 (
    loadoutItemId INTEGER PRIMARY KEY AUTOINCREMENT,
    loadoutId INTEGER NOT NULL,
    gearId INTEGER NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    notes TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (loadoutId) REFERENCES loadouts(loadoutId) ON DELETE CASCADE,
    FOREIGN KEY (gearId) REFERENCES gear(gearId)
);

-- Rows with neither a registration nor a detached gear cannot name a gear and
-- are dropped.
INSERT INTO loadout_items_v10 (loadoutItemId, loadoutId, gearId, quantity, notes)
SELECT li.loadoutItemId, li.loadoutId, COALESCE(r.gearId, li.detachedGearId), li.quantity, li.notes
FROM loadout_items li
LEFT JOIN user_gear_registrations r ON r.userGearRegistrationId = li.registrationId
WHERE COALESCE(r.gearId, li.detachedGearId) IS NOT NULL
  AND EXISTS (SELECT 1 FROM gear g WHERE g.gearId = COALESCE(r.gearId, li.detachedGearId));

CREATE TEMP TABLE m11_seq AS
SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'loadout_items'), 0) AS seq;

DROP TABLE loadout_items;
ALTER TABLE loadout_items_v10 RENAME TO loadout_items;

DELETE FROM sqlite_sequence WHERE name = 'loadout_items';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'loadout_items', MAX((SELECT seq FROM m11_seq), COALESCE((SELECT MAX(loadoutItemId) FROM loadout_items), 0));
DROP TABLE m11_seq;

CREATE INDEX IF NOT EXISTS idx_loadout_items_loadout ON loadout_items(loadoutId);
CREATE INDEX IF NOT EXISTS idx_loadout_items_gear ON loadout_items(gearId);

CREATE TRIGGER IF NOT EXISTS trg_loadout_items_quantity_insert
BEFORE INSERT ON loadout_items
WHEN typeof(NEW.quantity) <> 'integer' OR NEW.quantity <= 0
BEGIN
    SELECT RAISE(ABORT, 'loadout_items.quantity must be an integer greater than 0');
END;

CREATE TRIGGER IF NOT EXISTS trg_loadout_items_quantity_update
BEFORE UPDATE OF quantity ON loadout_items
WHEN typeof(NEW.quantity) <> 'integer' OR NEW.quantity <= 0
BEGIN
    SELECT RAISE(ABORT, 'loadout_items.quantity must be an integer greater than 0');
END;

DROP TABLE loadout_places;
