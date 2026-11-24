-- ARS-001 section 2.2 / 2.6 (milestone B): measured weight with its source and
-- closet count on registrations (BR-026, BR-036), the consumable role of a
-- loadout item (BR-027), and the stage, dates and goal of a loadout (BR-028).
--
-- ADD COLUMN only: SQLite checks each new CHECK and NOT NULL constraint against
-- the existing rows, so every existing registration gets closetCount 1, every
-- loadout stage 'planning' and every item consumable 0 (BR-036 AC-1).
-- Not idempotent: a replay on a migrated database fails on the first statement
-- and changes nothing.

-- Registrations: the owner's own weight and how many the owner has.
ALTER TABLE user_gear_registrations ADD COLUMN measuredWeight INTEGER
    CHECK (measuredWeight IS NULL OR (typeof(measuredWeight) = 'integer' AND measuredWeight >= 1));

-- Both or neither. Written with explicit IS NOT NULL tests: the form in
-- ARS-001 2.2 (`src IN (...) AND weight IS NOT NULL OR src IS NULL AND weight
-- IS NULL`) evaluates to NULL, which a CHECK accepts, for a weight whose source
-- is NULL, and BR-026 AC-1 refuses a value without a source.
ALTER TABLE user_gear_registrations ADD COLUMN measuredWeightSource TEXT
    CHECK ((measuredWeight IS NULL AND measuredWeightSource IS NULL)
        OR (measuredWeight IS NOT NULL AND measuredWeightSource IS NOT NULL
            AND measuredWeightSource IN ('weighed','guess')));

ALTER TABLE user_gear_registrations ADD COLUMN closetCount INTEGER NOT NULL DEFAULT 1
    CHECK (typeof(closetCount) = 'integer' AND closetCount >= 1);

-- Loadout items: the consumable role (written by the item handlers of B2).
ALTER TABLE loadout_items ADD COLUMN consumable INTEGER NOT NULL DEFAULT 0
    CHECK (consumable IN (0,1));

-- Loadouts: stage, dates and goal (written by the loadout handlers of B2).
-- startDate is added before endDate, whose CHECK reads it.
ALTER TABLE loadouts ADD COLUMN stage TEXT NOT NULL DEFAULT 'planning'
    CHECK (stage IN ('planning','packing','done'));
ALTER TABLE loadouts ADD COLUMN doneAt TEXT;
ALTER TABLE loadouts ADD COLUMN startDate TEXT
    CHECK (startDate IS NULL OR date(startDate) IS startDate);
ALTER TABLE loadouts ADD COLUMN endDate TEXT
    CHECK (endDate IS NULL OR (date(endDate) IS endDate AND (startDate IS NULL OR endDate >= startDate)));
ALTER TABLE loadouts ADD COLUMN weightGoal INTEGER
    CHECK (weightGoal IS NULL OR (typeof(weightGoal) = 'integer' AND weightGoal >= 1));

-- The effective weight of a registration is its measured weight when it has
-- one, else the catalog weight (BR-026). loadout_item_weight and
-- trg_registration_delete_detach (000011) name this view and are not
-- recreated: SQLite resolves view names when a statement runs.
DROP VIEW registration_effective_weight;
CREATE VIEW registration_effective_weight AS
SELECT r.userGearRegistrationId AS registrationId,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END                    AS catalogWeight,
       COALESCE(r.measuredWeight, CASE WHEN g.gearWeight > 0 THEN g.gearWeight END) AS unitWeight,
       COALESCE(r.measuredWeightSource,
                CASE WHEN g.gearWeight > 0 THEN 'catalog' ELSE 'none' END) AS weightSource
FROM user_gear_registrations r LEFT JOIN gear g ON g.gearId = r.gearId;

-- BR-029 AC-3, exact. The on-your-back weight of a loadout is the sum of
-- quantity x unit weight over the terms of the two views below. SQLite cannot
-- add them exactly: a product past 2^63-1 turns into a REAL, SUM past it raises
-- an error and TOTAL is a float. So the views only list the terms, each an exact
-- integer, and the API adds them with math/big (loadoutColumns, sumOnBackTerms).
-- The 000011 view loadout_on_back_weight, which summed them, is replaced.
--
-- Two views, not one UNION ALL: SQLite does not push the loadout filter into a
-- compound view, and would scan every item of every loadout for each list
-- (EXPLAIN QUERY PLAN); each view alone uses idx_loadout_items_loadout and
-- idx_loadout_places_bag as the 000011 view did.
--
-- An item term is an item that is not in Worn (an item without a place counts)
-- with its effective unit weight. A bag term is a bag's own weight, once, with
-- quantity 1. Worn items, and items and bags with no weight, are not terms.
DROP VIEW loadout_on_back_weight;
CREATE VIEW loadout_on_back_item_terms AS
SELECT li.loadoutId AS loadoutId, li.quantity AS quantity, iw.unitWeight AS unitWeight
FROM loadout_items li
JOIN loadout_item_weight iw ON iw.loadoutItemId = li.loadoutItemId
LEFT JOIN loadout_places p  ON p.placeId = li.placeId
WHERE iw.unitWeight IS NOT NULL AND (p.placeKind IS NULL OR p.placeKind <> 'worn');

CREATE VIEW loadout_on_back_bag_terms AS
SELECT p.loadoutId AS loadoutId, 1 AS quantity, w.unitWeight AS unitWeight
FROM loadout_places p
JOIN registration_effective_weight w ON w.registrationId = p.registrationId
WHERE p.placeKind = 'bag' AND w.unitWeight IS NOT NULL;
