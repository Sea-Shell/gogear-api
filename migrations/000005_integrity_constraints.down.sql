-- Removes the constraints and indexes of 000005. Data changes are not undone:
-- blank slugs stay NULL, clamped quantities stay 1 (their original values stay
-- in audit_000005_clamped_loadout_item_quantity, which is kept on purpose) and
-- removed duplicate links are gone (they were identical to the one kept).

DROP TRIGGER IF EXISTS trg_loadout_items_quantity_insert;
DROP TRIGGER IF EXISTS trg_loadout_items_quantity_update;
DROP TRIGGER IF EXISTS trg_container_links_no_self_insert;
DROP TRIGGER IF EXISTS trg_container_links_no_self_update;

DROP INDEX IF EXISTS idx_container_links_unique;
DROP INDEX IF EXISTS idx_container_links_content;
DROP INDEX IF EXISTS idx_user_gear_registrations_user;
DROP INDEX IF EXISTS idx_user_gear_registrations_gear;
