-- Integrity constraints (review COR-05, PERF-01). Every statement is
-- idempotent, so replaying the file on a migrated database changes nothing.
--
-- Constraints are triggers and indexes, not CHECK constraints: SQLite adds a
-- CHECK only by rebuilding the table (DROP TABLE + rename), and the migration
-- runs inside a transaction where "PRAGMA foreign_keys = OFF" has no effect.
-- With foreign keys enforced, dropping a parent table deletes through its
-- ON DELETE CASCADE keys. Nothing here rebuilds a table.
--
-- Existing data is never deleted, apart from exact duplicate container links
-- (identical rows). Orphans (rows whose parent is missing) are left alone;
-- the service reports them at startup.

-- 1. loadout_items.quantity must be a positive integer.
--    Rows that already violate it are clamped to 1. Their original values are
--    kept in the audit table so they can be reviewed or restored by hand.
CREATE TABLE IF NOT EXISTS audit_000005_clamped_loadout_item_quantity (
    loadoutItemId INTEGER PRIMARY KEY,
    loadoutId INTEGER NOT NULL,
    quantity INTEGER NOT NULL
);

INSERT OR IGNORE INTO audit_000005_clamped_loadout_item_quantity (loadoutItemId, loadoutId, quantity)
    SELECT loadoutItemId, loadoutId, quantity FROM loadout_items
    WHERE typeof(quantity) <> 'integer' OR quantity <= 0;

UPDATE loadout_items SET quantity = 1
    WHERE typeof(quantity) <> 'integer' OR quantity <= 0;

-- The stored total weight of the affected loadouts follows the new quantities.
UPDATE loadouts SET totalWeight = IFNULL((
        SELECT SUM(g.gearWeight * li.quantity)
        FROM loadout_items li JOIN gear g ON g.gearId = li.gearId
        WHERE li.loadoutId = loadouts.loadoutId), 0)
    WHERE loadoutId IN (SELECT loadoutId FROM audit_000005_clamped_loadout_item_quantity);

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

-- 2. A blank loadout slug means "no slug": NULL, as the service stores it.
UPDATE loadouts SET loadoutSlug = NULL WHERE loadoutSlug IS NOT NULL AND TRIM(loadoutSlug) = '';

-- 3. Container links: no exact duplicates, no self-links.
--    Identical duplicate links (same container, same content) are reduced to
--    the oldest one; they carry no other data.
DELETE FROM user_container_registration
    WHERE containerRegistrationId NOT IN (
        SELECT MIN(containerRegistrationId) FROM user_container_registration
        GROUP BY userContainerId, userGearRegistrationId);

-- Also serves lookups by container (leftmost column).
CREATE UNIQUE INDEX IF NOT EXISTS idx_container_links_unique
    ON user_container_registration(userContainerId, userGearRegistrationId);

-- Existing self-links (container = content) are left in place for review;
-- new ones are refused.
CREATE TRIGGER IF NOT EXISTS trg_container_links_no_self_insert
BEFORE INSERT ON user_container_registration
WHEN NEW.userContainerId = NEW.userGearRegistrationId
BEGIN
    SELECT RAISE(ABORT, 'a registration cannot be contained in itself');
END;

CREATE TRIGGER IF NOT EXISTS trg_container_links_no_self_update
BEFORE UPDATE OF userContainerId, userGearRegistrationId ON user_container_registration
WHEN NEW.userContainerId = NEW.userGearRegistrationId
BEGIN
    SELECT RAISE(ABORT, 'a registration cannot be contained in itself');
END;

-- 4. Lookup indexes for owner, gear and content lookups. The loadout,
--    loadout item and gear-category indexes already exist.
CREATE INDEX IF NOT EXISTS idx_user_gear_registrations_user ON user_gear_registrations(userId);
CREATE INDEX IF NOT EXISTS idx_user_gear_registrations_gear ON user_gear_registrations(gearId);
CREATE INDEX IF NOT EXISTS idx_container_links_content ON user_container_registration(userGearRegistrationId);
