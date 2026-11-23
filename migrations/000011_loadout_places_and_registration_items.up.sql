-- ARS-001 section 2.1 / ADR-003 / ADR-004 (milestone A): loadout places,
-- loadout items that are the owner's registrations, packing progress and
-- computed weights. BR-023, BR-024, BR-025, BR-029, BR-030.
--
-- Runs in golang-migrate's transaction with foreign keys on. loadout_items is
-- referenced by no table, so it can be rebuilt (create, copy, drop, rename).
-- Not idempotent: a replay on a migrated database fails on the first statement
-- and changes nothing.

-- 1. Places: Worn and Last minute (one each per loadout) and bags (a
--    registration used as a bag, optionally inside another bag of the loadout).
CREATE TABLE loadout_places (
    placeId        INTEGER PRIMARY KEY AUTOINCREMENT,
    loadoutId      INTEGER NOT NULL REFERENCES loadouts(loadoutId) ON DELETE CASCADE,
    placeKind      TEXT    NOT NULL CHECK (placeKind IN ('worn','last_minute','bag')),
    registrationId INTEGER REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE CASCADE,
    parentPlaceId  INTEGER REFERENCES loadout_places(placeId) ON DELETE SET NULL,
    position       INTEGER,
    closed         INTEGER NOT NULL DEFAULT 0 CHECK (closed IN (0,1)),
    CHECK ((placeKind = 'bag') = (registrationId IS NOT NULL)),
    CHECK (placeKind = 'bag' OR (parentPlaceId IS NULL AND position IS NULL)),
    CHECK (placeKind <> 'bag' OR position IS NOT NULL)
);
CREATE UNIQUE INDEX idx_loadout_places_worn        ON loadout_places(loadoutId) WHERE placeKind = 'worn';
CREATE UNIQUE INDEX idx_loadout_places_last_minute ON loadout_places(loadoutId) WHERE placeKind = 'last_minute';
CREATE UNIQUE INDEX idx_loadout_places_bag         ON loadout_places(loadoutId, registrationId) WHERE placeKind = 'bag';
CREATE INDEX idx_loadout_places_registration ON loadout_places(registrationId);
CREATE INDEX idx_loadout_places_parent       ON loadout_places(parentPlaceId);

-- 2. Worn for every loadout, existing and future (insert and copy paths).
INSERT INTO loadout_places (loadoutId, placeKind) SELECT loadoutId, 'worn' FROM loadouts;

CREATE TRIGGER trg_loadouts_create_worn
AFTER INSERT ON loadouts
BEGIN
    INSERT INTO loadout_places (loadoutId, placeKind) VALUES (NEW.loadoutId, 'worn');
END;

-- 3. Place triggers. The API checks every rule first; a message reaching a
--    handler means a missed check (ARS-001 section 1).
CREATE TRIGGER trg_loadout_places_owner_insert
BEFORE INSERT ON loadout_places
WHEN NEW.registrationId IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM user_gear_registrations r JOIN loadouts l ON l.userId = r.userId
                 WHERE r.userGearRegistrationId = NEW.registrationId AND l.loadoutId = NEW.loadoutId)
BEGIN
    SELECT RAISE(ABORT, 'bag registration must belong to the loadout owner');
END;

CREATE TRIGGER trg_loadout_places_owner_update
BEFORE UPDATE OF registrationId, loadoutId ON loadout_places
WHEN NEW.registrationId IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM user_gear_registrations r JOIN loadouts l ON l.userId = r.userId
                 WHERE r.userGearRegistrationId = NEW.registrationId AND l.loadoutId = NEW.loadoutId)
BEGIN
    SELECT RAISE(ABORT, 'bag registration must belong to the loadout owner');
END;

CREATE TRIGGER trg_loadout_places_parent_insert
BEFORE INSERT ON loadout_places
WHEN NEW.parentPlaceId IS NOT NULL
 AND (NEW.parentPlaceId IS NEW.placeId
      OR NOT EXISTS (SELECT 1 FROM loadout_places p
                     WHERE p.placeId = NEW.parentPlaceId AND p.placeKind = 'bag' AND p.loadoutId = NEW.loadoutId))
BEGIN
    SELECT RAISE(ABORT, 'parent must be another bag of the same loadout');
END;

CREATE TRIGGER trg_loadout_places_parent_update
BEFORE UPDATE OF parentPlaceId, loadoutId ON loadout_places
WHEN NEW.parentPlaceId IS NOT NULL
 AND (NEW.parentPlaceId = NEW.placeId
      OR NOT EXISTS (SELECT 1 FROM loadout_places p
                     WHERE p.placeId = NEW.parentPlaceId AND p.placeKind = 'bag' AND p.loadoutId = NEW.loadoutId))
BEGIN
    SELECT RAISE(ABORT, 'parent must be another bag of the same loadout');
END;

-- On insert a new row cannot be anyone's parent, so only an update can close
-- a cycle. UNION, not UNION ALL, so the walk ends over a ring.
CREATE TRIGGER trg_loadout_places_no_cycle
BEFORE UPDATE OF parentPlaceId ON loadout_places
WHEN NEW.parentPlaceId IS NOT NULL
 AND EXISTS (
    WITH RECURSIVE up(id) AS (
        SELECT NEW.parentPlaceId
        UNION
        SELECT p.parentPlaceId FROM loadout_places p JOIN up ON p.placeId = up.id
        WHERE p.parentPlaceId IS NOT NULL
    )
    SELECT 1 FROM up WHERE id = NEW.placeId)
BEGIN
    SELECT RAISE(ABORT, 'a bag cannot be inside one of its own contents');
END;

-- A registration never changes owner. No API path does it, but without this
-- guard a direct update would break the owner rules of existing bags and
-- items, which are checked only when places and items are written.
CREATE TRIGGER trg_registration_owner_fixed
BEFORE UPDATE OF userId ON user_gear_registrations
WHEN NEW.userId IS NOT OLD.userId
BEGIN
    SELECT RAISE(ABORT, 'a registration cannot change owner');
END;

-- 4. Rebuild loadout_items.
CREATE TABLE loadout_items_v11 (
    loadoutItemId        INTEGER PRIMARY KEY AUTOINCREMENT,
    loadoutId            INTEGER NOT NULL REFERENCES loadouts(loadoutId) ON DELETE CASCADE,
    registrationId       INTEGER REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE SET NULL,
    placeId              INTEGER REFERENCES loadout_places(placeId) ON DELETE SET NULL,
    quantity             INTEGER NOT NULL DEFAULT 1 CHECK (typeof(quantity) = 'integer' AND quantity > 0),
    notes                TEXT    NOT NULL DEFAULT '',
    ticked               INTEGER NOT NULL DEFAULT 0 CHECK (ticked IN (0,1)),
    detachedReason       TEXT    CHECK (detachedReason IN ('legacy','registration_deleted')),
    detachedGearId       INTEGER REFERENCES gear(gearId),   -- no ON DELETE action: deleting gear a detached item uses is refused (409)
    detachedGearName     TEXT,
    detachedUnitWeight   INTEGER,
    detachedWeightSource TEXT    CHECK (detachedWeightSource IN ('weighed','guess','catalog')),
    -- One-directional on purpose: trg_registration_delete_detach sets
    -- detachedReason while registrationId is still set; the foreign-key
    -- action clears registrationId afterwards.
    CHECK (registrationId IS NOT NULL OR detachedReason IS NOT NULL)
);

-- Binding rule: the lowest registration of the loadout owner with the same
-- gear. No registration but the gear exists: a legacy item that reads its gear
-- live. Neither: an orphan test row, not copied (BR-023 AC-7). Rows of missing
-- loadouts are not copied.
INSERT INTO loadout_items_v11 (loadoutItemId, loadoutId, registrationId, quantity, notes, detachedReason, detachedGearId)
SELECT b.loadoutItemId, b.loadoutId, b.regId, b.quantity, b.notes,
       CASE WHEN b.regId IS NULL THEN 'legacy' END,
       CASE WHEN b.regId IS NULL THEN b.gearId END
FROM (
    SELECT li.loadoutItemId, li.loadoutId, li.gearId, li.quantity, li.notes,
           (SELECT MIN(r.userGearRegistrationId) FROM user_gear_registrations r
             WHERE r.gearId = li.gearId AND r.userId = l.userId) AS regId
    FROM loadout_items li JOIN loadouts l ON l.loadoutId = li.loadoutId
) b
WHERE b.regId IS NOT NULL OR EXISTS (SELECT 1 FROM gear g WHERE g.gearId = b.gearId);

-- Keep the ID sequence (BR-025 AC-7): a rebuild resets AUTOINCREMENT, so IDs
-- of items deleted before the migration would otherwise be issued again.
CREATE TEMP TABLE m11_seq AS
SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'loadout_items'), 0) AS seq;

DROP TABLE loadout_items;
ALTER TABLE loadout_items_v11 RENAME TO loadout_items;

DELETE FROM sqlite_sequence WHERE name = 'loadout_items';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'loadout_items', MAX((SELECT seq FROM m11_seq), COALESCE((SELECT MAX(loadoutItemId) FROM loadout_items), 0));
DROP TABLE m11_seq;

CREATE INDEX idx_loadout_items_loadout       ON loadout_items(loadoutId);
CREATE INDEX idx_loadout_items_registration  ON loadout_items(registrationId);
CREATE INDEX idx_loadout_items_place         ON loadout_items(placeId);
CREATE INDEX idx_loadout_items_detached_gear ON loadout_items(detachedGearId);

-- 5. Item triggers.
CREATE TRIGGER trg_loadout_items_owner_insert
BEFORE INSERT ON loadout_items
WHEN NEW.registrationId IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM user_gear_registrations r JOIN loadouts l ON l.userId = r.userId
                 WHERE r.userGearRegistrationId = NEW.registrationId AND l.loadoutId = NEW.loadoutId)
BEGIN
    SELECT RAISE(ABORT, 'item registration must belong to the loadout owner');
END;

CREATE TRIGGER trg_loadout_items_owner_update
BEFORE UPDATE OF registrationId, loadoutId ON loadout_items
WHEN NEW.registrationId IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM user_gear_registrations r JOIN loadouts l ON l.userId = r.userId
                 WHERE r.userGearRegistrationId = NEW.registrationId AND l.loadoutId = NEW.loadoutId)
BEGIN
    SELECT RAISE(ABORT, 'item registration must belong to the loadout owner');
END;

CREATE TRIGGER trg_loadout_items_place_insert
BEFORE INSERT ON loadout_items
WHEN NEW.placeId IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM loadout_places p WHERE p.placeId = NEW.placeId AND p.loadoutId = NEW.loadoutId)
BEGIN
    SELECT RAISE(ABORT, 'item place must belong to the same loadout');
END;

CREATE TRIGGER trg_loadout_items_place_update
BEFORE UPDATE OF placeId, loadoutId ON loadout_items
WHEN NEW.placeId IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM loadout_places p WHERE p.placeId = NEW.placeId AND p.loadoutId = NEW.loadoutId)
BEGIN
    SELECT RAISE(ABORT, 'item place must belong to the same loadout');
END;

-- 6. Deleting a registration keeps its items as snapshots (BR-030). The
--    foreign-key actions then clear the items' registrationId, delete the
--    registration's bag rows and, through them, clear contained items' placeId
--    and child bags' parentPlaceId.
CREATE TRIGGER trg_registration_delete_detach
BEFORE DELETE ON user_gear_registrations
BEGIN
    UPDATE loadout_items
    SET detachedReason       = 'registration_deleted',
        detachedGearId       = (SELECT g.gearId FROM gear g WHERE g.gearId = OLD.gearId),
        detachedGearName     = COALESCE((SELECT g.gearName FROM gear g WHERE g.gearId = OLD.gearId), 'Unknown gear'),
        detachedUnitWeight   = (SELECT w.unitWeight FROM registration_effective_weight w WHERE w.registrationId = OLD.userGearRegistrationId),
        detachedWeightSource = (SELECT CASE WHEN w.weightSource = 'none' THEN NULL ELSE w.weightSource END
                                FROM registration_effective_weight w WHERE w.registrationId = OLD.userGearRegistrationId)
    WHERE registrationId = OLD.userGearRegistrationId;
END;

-- 7. Views (the only definitions of these values), then the stored total goes.
CREATE VIEW registration_effective_weight AS
SELECT r.userGearRegistrationId AS registrationId,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END            AS catalogWeight,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END            AS unitWeight,
       CASE WHEN g.gearWeight > 0 THEN 'catalog' ELSE 'none' END   AS weightSource
FROM user_gear_registrations r LEFT JOIN gear g ON g.gearId = r.gearId;

CREATE VIEW loadout_item_weight AS
SELECT li.loadoutItemId, li.loadoutId,
       CASE WHEN li.registrationId IS NOT NULL       THEN w.unitWeight
            WHEN li.detachedReason = 'legacy'        THEN (SELECT CASE WHEN g.gearWeight > 0 THEN g.gearWeight END FROM gear g WHERE g.gearId = li.detachedGearId)
            ELSE li.detachedUnitWeight END                               AS unitWeight,
       CASE WHEN li.registrationId IS NOT NULL       THEN w.weightSource
            WHEN li.detachedReason = 'legacy'        THEN CASE WHEN (SELECT g.gearWeight FROM gear g WHERE g.gearId = li.detachedGearId) > 0 THEN 'catalog' ELSE 'none' END
            ELSE COALESCE(li.detachedWeightSource, 'none') END           AS weightSource
FROM loadout_items li
LEFT JOIN registration_effective_weight w ON w.registrationId = li.registrationId;

CREATE VIEW loadout_on_back_weight AS          -- BR-029 AC-3
SELECT l.loadoutId,
       COALESCE((SELECT SUM(li.quantity * COALESCE(iw.unitWeight, 0))
                 FROM loadout_items li
                 JOIN loadout_item_weight iw ON iw.loadoutItemId = li.loadoutItemId
                 LEFT JOIN loadout_places p  ON p.placeId = li.placeId
                 WHERE li.loadoutId = l.loadoutId AND (p.placeKind IS NULL OR p.placeKind <> 'worn')), 0)
     + COALESCE((SELECT SUM(COALESCE(w.unitWeight, 0))
                 FROM loadout_places p
                 JOIN registration_effective_weight w ON w.registrationId = p.registrationId
                 WHERE p.loadoutId = l.loadoutId AND p.placeKind = 'bag'), 0) AS totalWeight
FROM loadouts l;

ALTER TABLE loadouts DROP COLUMN totalWeight;
