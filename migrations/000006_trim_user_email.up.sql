-- BR-004 AC-4: stored emails are trimmed, the same way the Go side trims them
-- (strings.TrimSpace strips all Unicode whitespace, SQLite's trim() only
-- spaces unless told otherwise). New writes are trimmed by the API; this
-- normalises rows written before that. Case is left alone (P-7).
UPDATE users SET userEmail = trim(userEmail, char(9,10,11,12,13,32,133,160,5760,8192,8193,8194,8195,8196,8197,8198,8199,8200,8201,8202,8232,8233,8239,8287,12288))
WHERE userEmail <> trim(userEmail, char(9,10,11,12,13,32,133,160,5760,8192,8193,8194,8195,8196,8197,8198,8199,8200,8201,8202,8232,8233,8239,8287,12288));
