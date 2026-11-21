-- Remove the Google subject binding columns.

DROP INDEX IF EXISTS idx_users_google_subject;
ALTER TABLE users DROP COLUMN userEmailEdited;
ALTER TABLE users DROP COLUMN googleSubject;
