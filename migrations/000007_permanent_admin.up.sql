-- BR-019: the first admin is permanent.
--
-- permanent_admin holds at most one row (singleton = 1): the account that is
-- the permanent admin. The designation is stored durably and nothing in the API
-- moves it.
--
-- There is deliberately no foreign key to users. If the account is removed
-- outside the API (for example by an operator), the row stays, so no other
-- account takes its place (BR-019 AC-8). User IDs are AUTOINCREMENT and are
-- never reused.

CREATE TABLE IF NOT EXISTS permanent_admin (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    userId INTEGER NOT NULL UNIQUE
);

-- When this migration runs, the admin with the lowest user ID is designated
-- (P-8). With no admin yet, the table stays empty and the triggers below
-- designate the first account that becomes admin afterwards, by any means.
INSERT OR IGNORE INTO permanent_admin (singleton, userId)
    SELECT 1, userId FROM users WHERE userIsAdmin <> 0 ORDER BY userId LIMIT 1;

-- INSERT OR IGNORE does nothing once the singleton row exists, so these
-- triggers can never move the designation.
CREATE TRIGGER IF NOT EXISTS trg_permanent_admin_designate_insert
AFTER INSERT ON users
WHEN NEW.userIsAdmin <> 0
BEGIN
    INSERT OR IGNORE INTO permanent_admin (singleton, userId) VALUES (1, NEW.userId);
END;

CREATE TRIGGER IF NOT EXISTS trg_permanent_admin_designate_update
AFTER UPDATE OF userIsAdmin ON users
WHEN NEW.userIsAdmin <> 0
BEGIN
    INSERT OR IGNORE INTO permanent_admin (singleton, userId) VALUES (1, NEW.userId);
END;
