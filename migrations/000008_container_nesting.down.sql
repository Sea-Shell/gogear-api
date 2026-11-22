-- Removes the nesting rules of 000008 and restores the indexes 000005 created.
-- Links removed by the up migration are not restored.
DROP TRIGGER IF EXISTS trg_container_links_no_cycle_insert;
DROP TRIGGER IF EXISTS trg_container_links_no_cycle_update;
DROP INDEX IF EXISTS idx_container_links_content_unique;
DROP INDEX IF EXISTS idx_container_links_container;
CREATE UNIQUE INDEX IF NOT EXISTS idx_container_links_unique ON user_container_registration(userContainerId, userGearRegistrationId);
CREATE INDEX IF NOT EXISTS idx_container_links_content ON user_container_registration(userGearRegistrationId);
