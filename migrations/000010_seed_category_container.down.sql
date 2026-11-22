-- ADR-002 D-7: no data change. A seeded 1 cannot be told apart from an admin's 1,
-- so undoing the seeding could remove admin settings. Down of 000009 removes
-- all settings.
SELECT 1;
