-- Rollback baseline: drop all legacy tables
-- Order respects foreign-key dependencies

DROP TABLE IF EXISTS user_container_registration;
DROP TABLE IF EXISTS user_gear_registrations;
DROP INDEX IF EXISTS manufacture_index;
DROP INDEX IF EXISTS category_index;
DROP INDEX IF EXISTS topCategory_index;
DROP TABLE IF EXISTS gear;
DROP TABLE IF EXISTS manufacture;
DROP TABLE IF EXISTS gear_category;
DROP TABLE IF EXISTS gear_top_category;
DROP TABLE IF EXISTS users;
