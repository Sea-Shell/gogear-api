-- BR-021 / ADR-001: the catalog category decides whether gear can be a
-- container. Each top category carries a yes/no setting and each sub-category
-- an optional explicit setting that overrides it. A gear item's container
-- status is computed from its sub-category (view gear_container_capability);
-- it is no longer stored on the gear.
--
-- Not idempotent: SQLite has no ADD COLUMN IF NOT EXISTS, so a replay on a
-- migrated database fails on the first statement and changes nothing (the
-- migration runs in one transaction). Links, registrations and gear data are
-- not changed (BR-021 AC-6, P-15).

-- 1. The settings. Every existing row starts as "not a container" (top
--    category 0, sub-category NULL = no explicit setting): no old flag is
--    carried over (P-14).
ALTER TABLE gear_top_category ADD COLUMN topCategoryIsContainer INTEGER NOT NULL DEFAULT 0
    CHECK (topCategoryIsContainer IN (0, 1));
ALTER TABLE gear_category ADD COLUMN categoryIsContainer INTEGER DEFAULT NULL
    CHECK (categoryIsContainer IS NULL OR categoryIsContainer IN (0, 1));

-- 2. The per-gear flag stops meaning anything. It is renamed, not dropped, so
--    its values stay recoverable and a query that still names the old column
--    fails loudly instead of reporting a stale flag.
ALTER TABLE gear RENAME COLUMN gearIsContainer TO gearIsContainerLegacy;

-- 3. The only definition of the effective setting (P-13): the sub-category's
--    explicit setting, else its top category's, else not a container. The gear
--    view joins on the gear's sub-category only; the gear's own top-category
--    reference has no effect (BR-021 AC-3).
CREATE VIEW category_container_setting AS
SELECT c.categoryId                                                   AS categoryId,
       c.categoryIsContainer                                          AS explicitIsContainer,
       COALESCE(c.categoryIsContainer, t.topCategoryIsContainer, 0)   AS effectiveIsContainer
FROM gear_category c
LEFT JOIN gear_top_category t ON t.topCategoryId = c.categoryTopCategoryId;

CREATE VIEW gear_container_capability AS
SELECT g.gearId                            AS gearId,
       COALESCE(s.effectiveIsContainer, 0) AS isContainer
FROM gear g
LEFT JOIN category_container_setting s ON s.categoryId = g.gearCategoryId;

-- 4. The database repeats the API's rule (BR-020 AC-1): a link can only be
--    stored or re-pointed into a registration whose gear is container-capable.
--    The triggers fire only on link insert and update, so an existing link
--    stays when its container loses capability (BR-021 AC-6, BR-020 AC-8).
--    The message is matched by the API (ADR-001 D-9).
CREATE TRIGGER trg_container_links_capable_insert
BEFORE INSERT ON user_container_registration
WHEN NOT EXISTS (
    SELECT 1 FROM user_gear_registrations r
    JOIN gear_container_capability cap ON cap.gearId = r.gearId
    WHERE r.userGearRegistrationId = NEW.userContainerId AND cap.isContainer = 1)
BEGIN
    SELECT RAISE(ABORT, 'the container registration cannot hold items');
END;

CREATE TRIGGER trg_container_links_capable_update
BEFORE UPDATE OF userContainerId, userGearRegistrationId ON user_container_registration
WHEN NOT EXISTS (
    SELECT 1 FROM user_gear_registrations r
    JOIN gear_container_capability cap ON cap.gearId = r.gearId
    WHERE r.userGearRegistrationId = NEW.userContainerId AND cap.isContainer = 1)
BEGIN
    SELECT RAISE(ABORT, 'the container registration cannot hold items');
END;

-- 5. P-14 alternative (seed the settings from the old per-gear flags): NOT
--    APPLIED. If the owner chooses it, add this statement here before the
--    migration is released, or in a new migration afterwards. What a
--    sub-category gets when only some of its gear was flagged is open (ADR-001
--    OQ-1); the candidate is "explicit container where any gear was flagged":
--
--      UPDATE gear_category SET categoryIsContainer = 1
--      WHERE categoryId IN (SELECT gearCategoryId FROM gear WHERE gearIsContainerLegacy = 1);
