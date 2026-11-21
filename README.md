# GoGear API

A Go HTTP API for gear catalogs, personal inventory, containers and loadouts, backed by one SQLite file.

## Operating notes

Read these before deploying a new version.

### Configuration the service now refuses to start without

- `auth.jwt-audience` and `auth.jwt-admin-audience` must both be set, non-blank and different from each other. Tokens for ordinary users and for admins are told apart by this value.
- `auth.google-client-id` is required. The service refuses to start without it, because an empty audience would make Google token validation skip the audience check.
- `database.file` must be set. An empty value is refused.

### Back up the SQLite file first

Migration `000005` changes data: it clamps loadout item quantities of 0 or below to 1, turns blank loadout slugs into NULL, and deletes identical duplicate container links. The down migration removes the constraints but does not restore those changes. The original quantities stay in the table `audit_000005_clamped_loadout_item_quantity`.

The database runs in WAL mode. Next to the database file there are `<file>-wal` and `<file>-shm` files. A backup must include them, or the database must be checkpointed first:

```sh
sqlite3 /path/to/gogear.db "PRAGMA wal_checkpoint(TRUNCATE);"   # then copy the .db file
# or take a consistent copy while the service runs:
sqlite3 /path/to/gogear.db ".backup '/path/to/backup.db'"
```

WAL needs a local disk (a Kubernetes `hostPath` is fine, a network file system is not). Run one instance against the file; a rolling update that starts a second pod while the first is running should use the `Recreate` strategy instead.

### Foreign-key warning at startup

Foreign keys are enforced on every connection. Rows written before that may point at parents that no longer exist. At startup the service runs `PRAGMA foreign_key_check` and logs one warning per table and parent:

```
Database contains rows that reference a missing parent row; they are left unchanged  table=loadouts parent_table=users orphan_rows=1
```

The service starts anyway, and nothing is deleted or repaired. The warning only means that those rows exist. New writes that would create such rows are refused. To list them, run `PRAGMA foreign_key_check;` in `sqlite3`; decide per row whether to delete or fix it. Requests that name catalog gear that does not exist now get 404, and deleting a catalog row that other rows still use gets 409.

### First Google sign-in after the upgrade

Accounts are bound to the stable Google subject. Existing accounts have none yet, so each one is bound at its first Google sign-in, by matching the stored email with the verified Google email. The match is exact and case-sensitive, after trimming whitespace of any kind (spaces, tabs, newlines and other Unicode whitespace). Emails are stored trimmed, and migration 000006 trims the ones already stored.

- Before deploying, check that the stored `users.userEmail` of each existing account is exactly the email of its Google account. If it is not, the first sign-in does not find the account and creates a new, empty one (unless another rule in the requirements refuses the sign-in).
- An account whose owner changed its email through the profile update is never bound by email. It has to be fixed by the operator.
- After the first sign-in, the account is found by subject only; later email changes at Google have no effect on which account the user reaches.
