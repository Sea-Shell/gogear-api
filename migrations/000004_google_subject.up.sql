-- Bind accounts to the stable Google subject (sub) instead of email.
-- googleSubject is NULL until the account's first Google sign-in after this
-- migration; the sign-in then fills it in once (BR-004).
-- userEmailEdited is set when the account's owner changes the profile email.
-- Such an account is never eligible for the email-based backfill, so a user
-- cannot plant another person's email to capture their first sign-in.

ALTER TABLE users ADD COLUMN googleSubject TEXT;
ALTER TABLE users ADD COLUMN userEmailEdited INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_google_subject ON users(googleSubject) WHERE googleSubject IS NOT NULL;
