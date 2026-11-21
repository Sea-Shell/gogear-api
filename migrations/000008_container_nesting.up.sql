-- BR-020: a registration has at most one direct container and is never inside
-- itself, at any depth. The API checks both before it inserts (inside an
-- immediate transaction, so concurrent requests queue); the rules below are the
-- database's own guard, so a handler that forgets a check, or a second
-- connection, still cannot store a violation.
--
-- The data is generated test data (anchor, context note), so links that
-- already violate the rules are removed (BR-020 AC-8). Every statement is
-- idempotent: a replay on a migrated database changes nothing.

-- 1. Remove stored links that break the rules.
--    a. self-links and links whose container or content registration is gone
DELETE FROM user_container_registration WHERE userContainerId = userGearRegistrationId;
DELETE FROM user_container_registration
    WHERE NOT EXISTS (SELECT 1 FROM user_gear_registrations r WHERE r.userGearRegistrationId = user_container_registration.userContainerId)
       OR NOT EXISTS (SELECT 1 FROM user_gear_registrations r WHERE r.userGearRegistrationId = user_container_registration.userGearRegistrationId);

--    b. links between registrations of different owners
DELETE FROM user_container_registration
    WHERE (SELECT r.userId FROM user_gear_registrations r WHERE r.userGearRegistrationId = user_container_registration.userContainerId)
       <> (SELECT r.userId FROM user_gear_registrations r WHERE r.userGearRegistrationId = user_container_registration.userGearRegistrationId);

--    c. several direct containers: keep the oldest link of each registration
DELETE FROM user_container_registration
    WHERE containerRegistrationId NOT IN (
        SELECT MIN(containerRegistrationId) FROM user_container_registration GROUP BY userGearRegistrationId);

--    d. cycles. After (c) every registration has at most one container, so a
--    cycle is a ring in which every member has exactly its ring neighbour as
--    container. reach(node, anc) lists every container above node; a node on a
--    ring reaches itself, and then reaches exactly the ring. One link per ring
--    is removed: the one that puts the lowest-numbered member into its container.
DELETE FROM user_container_registration
    WHERE userGearRegistrationId IN (
        WITH RECURSIVE reach(node, anc) AS (
            SELECT userGearRegistrationId, userContainerId FROM user_container_registration
            UNION
            SELECT r.node, l.userContainerId FROM reach r
                JOIN user_container_registration l ON l.userGearRegistrationId = r.anc
        )
        SELECT node FROM reach r
        WHERE node = anc AND node = (SELECT MIN(anc) FROM reach r2 WHERE r2.node = r.node));

-- 2. One direct container per registration. This unique index on the content
--    column supersedes the (container, content) unique index of 000005: a pair
--    can no longer repeat when its content appears only once. The old content
--    index is redundant now, and lookups by container get their own index.
DROP INDEX IF EXISTS idx_container_links_unique;
DROP INDEX IF EXISTS idx_container_links_content;
CREATE UNIQUE INDEX IF NOT EXISTS idx_container_links_content_unique ON user_container_registration(userGearRegistrationId);
CREATE INDEX IF NOT EXISTS idx_container_links_container ON user_container_registration(userContainerId);

-- 3. No cycles. The new link closes a cycle when its container is already
--    inside its content: walking up from the container reaches the content.
--    (UNION, not UNION ALL, so the walk ends even over a ring.) The no-self
--    triggers of 000005 stay and catch the one-step case first.
CREATE TRIGGER IF NOT EXISTS trg_container_links_no_cycle_insert
BEFORE INSERT ON user_container_registration
WHEN EXISTS (
    WITH RECURSIVE up(id) AS (
        SELECT NEW.userContainerId
        UNION
        SELECT l.userContainerId FROM user_container_registration l JOIN up ON l.userGearRegistrationId = up.id
    )
    SELECT 1 FROM up WHERE id = NEW.userGearRegistrationId)
BEGIN
    SELECT RAISE(ABORT, 'a registration cannot be inside one of its own contents');
END;

-- An update re-points an existing link, so that link is not part of the walk.
CREATE TRIGGER IF NOT EXISTS trg_container_links_no_cycle_update
BEFORE UPDATE OF userContainerId, userGearRegistrationId ON user_container_registration
WHEN EXISTS (
    WITH RECURSIVE up(id) AS (
        SELECT NEW.userContainerId
        UNION
        SELECT l.userContainerId FROM user_container_registration l JOIN up ON l.userGearRegistrationId = up.id
        WHERE l.containerRegistrationId <> OLD.containerRegistrationId
    )
    SELECT 1 FROM up WHERE id = NEW.userGearRegistrationId)
BEGIN
    SELECT RAISE(ABORT, 'a registration cannot be inside one of its own contents');
END;
