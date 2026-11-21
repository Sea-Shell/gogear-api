-- Removes the permanent-admin designation. Admin status of users is unchanged.
DROP TRIGGER IF EXISTS trg_permanent_admin_designate_insert;
DROP TRIGGER IF EXISTS trg_permanent_admin_designate_update;
DROP TABLE IF EXISTS permanent_admin;
