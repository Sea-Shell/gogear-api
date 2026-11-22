-- Removes the category-driven container capability of 000009. All category
-- settings are lost. Links and the old per-gear flag values are kept; an up
-- migration afterwards gives the P-14 start values again.
DROP TRIGGER IF EXISTS trg_container_links_capable_insert;
DROP TRIGGER IF EXISTS trg_container_links_capable_update;
DROP VIEW IF EXISTS gear_container_capability;
DROP VIEW IF EXISTS category_container_setting;
ALTER TABLE gear_category DROP COLUMN categoryIsContainer;
ALTER TABLE gear_top_category DROP COLUMN topCategoryIsContainer;
ALTER TABLE gear RENAME COLUMN gearIsContainerLegacy TO gearIsContainer;
