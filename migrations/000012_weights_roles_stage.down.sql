-- Reverses 000012. Lossy: measured weights, closet counts, consumable roles,
-- stages, dates and goals are discarded. Not on the release path.
--
-- Order matters: the view that reads measuredWeight goes first (DROP COLUMN
-- refuses a column a view still names), and a column that another column's
-- CHECK reads goes after that column (endDate before startDate,
-- measuredWeightSource before measuredWeight).

DROP VIEW registration_effective_weight;
CREATE VIEW registration_effective_weight AS
SELECT r.userGearRegistrationId AS registrationId,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END            AS catalogWeight,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END            AS unitWeight,
       CASE WHEN g.gearWeight > 0 THEN 'catalog' ELSE 'none' END   AS weightSource
FROM user_gear_registrations r LEFT JOIN gear g ON g.gearId = r.gearId;

ALTER TABLE loadouts DROP COLUMN weightGoal;
ALTER TABLE loadouts DROP COLUMN endDate;
ALTER TABLE loadouts DROP COLUMN startDate;
ALTER TABLE loadouts DROP COLUMN doneAt;
ALTER TABLE loadouts DROP COLUMN stage;

ALTER TABLE loadout_items DROP COLUMN consumable;

ALTER TABLE user_gear_registrations DROP COLUMN closetCount;
ALTER TABLE user_gear_registrations DROP COLUMN measuredWeightSource;
ALTER TABLE user_gear_registrations DROP COLUMN measuredWeight;
