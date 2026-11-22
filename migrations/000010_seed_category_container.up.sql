-- BR-021 AC-5 (amended) / P-18, ADR-002 D-7: seed the category container
-- settings from the old per-gear flags, once, as part of the release.
-- A sub-category with at least one flagged gear item and no explicit setting
-- becomes explicitly a container. Explicit settings (0 or 1), top categories and
-- every other sub-category are not touched. golang-migrate runs this version
-- once per database.
UPDATE gear_category
SET categoryIsContainer = 1
WHERE categoryIsContainer IS NULL
  AND categoryId IN (SELECT gearCategoryId FROM gear
                     WHERE gearIsContainerLegacy = 1
                       AND gearCategoryId IS NOT NULL);
