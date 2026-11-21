# GoGear API

A Go HTTP API for gear catalogs, personal inventory, containers and loadouts, backed by one SQLite file.

## Operating notes

Read these before deploying a new version.

### Configuration the service now refuses to start without

- `auth.jwt-audience` and `auth.jwt-admin-audience` must both be set, non-blank and different from each other. Tokens for ordinary users and for admins are told apart by this value.
- `auth.google-client-id` is required. The service refuses to start without it, because an empty audience would make Google token validation skip the audience check.
- `database.file` must be set. An empty value is refused.

### Back up the SQLite file first

Migration `000005` changes data: it clamps loadout item quantities of 0 or below to 1, turns blank loadout slugs into NULL, and deletes identical duplicate container links. The down migration removes the constraints but does not restore those changes. The original quantities stay in the table `audit_000005_clamped_loadout_item_quantity`. Every run of the migration adds its own rows (with a `clampedAt` timestamp), so if the migration is rolled back and applied again and clamps the same item a second time, both original values are kept.

Migration `000007` designates the permanent admin (the admin with the lowest user ID when it runs, or the first account that becomes admin if there is none). That account can never lose admin status or be deleted through the API. Back up first if the lowest-ID admin is not the account you want: the designation lives in the table `permanent_admin`, and changing it is a manual database edit. Rolling back `000007` and applying it again designates the permanent admin anew: it picks the admin with the lowest user ID at that moment, which may be a different account from before.

Migration `000008` deletes container links that break the nesting rules: self-links, links with a missing registration, links between different users' registrations, all but the oldest container of a registration, and one link of each cycle. The down migration restores the indexes but not the deleted links.

Migration `000009` adds the container settings of the catalog categories. Every setting starts as "not a container", so no gear is container-capable until an admin marks categories: **after deploying, an admin must mark the container categories** (`top_category_is_container` on a top category, or `category_container` on a sub-category, through the admin API) before any registration can hold items; the old per-gear flag is kept as `gear.gearIsContainerLegacy` and is no longer read. It does not change any link. It cannot be applied twice by hand: a second run fails on its first statement and changes nothing. Its down migration discards the category settings and keeps links and the old flag values.

The database runs in WAL mode. Next to the database file there are `<file>-wal` and `<file>-shm` files. A plain copy of the `.db` file while the service runs can be torn or miss recent writes, and checkpointing first does not prevent that, because the service can write again before the copy finishes. Use one of these:

- **Stop the service, then copy.** Scale the Deployment to 0 and wait for the pod to end. Copy the `.db` file together with `-wal` and `-shm` if they exist. Run this on the node that holds the `hostPath` directory (`/data/disk2/opt/gogear-api` in `kubernetes-manifests/deployment.yaml`; the database path is `database.file` in the config).
- **Live backup with SQLite itself.** Run `sqlite3 /path/to/gogear.db ".backup '/path/to/backup.db'"` (or `sqlite3 ... "VACUUM INTO '/path/to/backup.db'"`) on the node, or in a debug container that has the `sqlite3` binary and mounts the same `hostPath` directory. The service image is Alpine-based and does **not** contain `sqlite3`, so `kubectl exec` into the service pod will not work. The backup file must not exist yet for `VACUUM INTO`.

Keep the backup outside the directory the service writes to, and check it with `sqlite3 /path/to/backup.db "PRAGMA integrity_check;"` (expected output: `ok`).

WAL needs a local disk (a Kubernetes `hostPath` is fine, a network file system is not). Run one instance against the file. The Deployment in `kubernetes-manifests/deployment.yaml` uses `strategy: Recreate`, so the old pod stops before the new one starts and runs its migrations. Keep that strategy: with the default `RollingUpdate` the new pod would migrate the database while the old pod is still serving it.

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
