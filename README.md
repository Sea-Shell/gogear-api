# GoGear API

A Go HTTP API for gear catalogs, personal inventory, containers and loadouts, backed by one SQLite file.

## Releases

Merging to `main` releases and deploys automatically; nobody runs anything after the merge. The release version comes from all commits since the last tag, and the highest type among them wins: a `fix` gives a patch release, a `feat` a minor release, and a breaking change a major release. Pull requests are squash-merged, so each merge adds one commit, its title, to that set (a rebase or merge adds every commit of the branch). `chore` and `ci` commits alone release nothing: no tag, image or deploy, and the run still succeeds. This holds only while no `fix` or `feat` commit is waiting unreleased; one such commit makes any later merge release. `perf` and `revert` commits do not release.

## Operating notes

Read these before deploying a new version.

### Configuration the service now refuses to start without

- `auth.jwt-audience` and `auth.jwt-admin-audience` must both be set, non-blank and different from each other. Tokens for ordinary users and for admins are told apart by this value.
- `auth.google-client-id` is required. The service refuses to start without it, because an empty audience would make Google token validation skip the audience check.
- `database.file` must be set. An empty value is refused.

### Automatic backup before schema migrations, and restore

The service backs up its own database. Each time it starts and finds a pending migration (one in `migrations/` that the database has not applied), it makes a backup before the first migration changes anything. No step by you is needed, and the release workflow does it too. It makes no backup when nothing is pending, and none on the first start on an empty volume.

- **Where.** In the directory `backups` next to the database file (for example `/config/backups`, on the same `hostPath` volume). Change it with `database.backup-dir`; a relative value is resolved against the directory of `database.file`. It must not be the database's own directory. The directory is created with mode `0700`; backup files get the database file's permissions.
- **How.** `VACUUM INTO` writes a standalone copy of the committed content, write-ahead log included. The service opens the copy read-only and checks it (`PRAGMA integrity_check` is `ok`, schema version as expected, nothing was committed meanwhile) before it keeps it and before it migrates.
- **Names.** `gogear-<UTC time>-<kind>-schema<N>-app<version>.db`, for example `gogear-20261122T181500Z-premigration-schema10-appv4.11.0.db`. `kind` is `premigration` or `prerestore`, `N` is the schema version the backup holds, and `version` is the release tag of the image that made it. Only files with this exact name pattern are counted or deleted; the service never touches other files in the directory.
- **Retention.** The three newest backups of each kind. An older one is removed only after the new one passed its check.
- **If the backup fails** (full disk, unwritable directory), no migration runs, the database stays as it was, and the service exits. The release run then fails and shows the reason. Fix the cause and the next start backs up and migrates. This is the current default (`Refuse` in `pkg/dbupgrade/policy.go`).
- **Limit.** Backups are on the same disk as the database. They do not protect against losing that disk. Copy them elsewhere yourself if that matters.

Find the backups in the pod log:

```
kubectl -n seashell logs deployment/gogear-api | grep 'gogear-api backup:'
```

Every start lists the backups it keeps (`kept file=...`); a new backup, a removed one, a skipped one and each failure have their own line.

**Restore** (the database is replaced as a whole, and the replaced database is kept first as a `prerestore` backup, so a restore can be undone):

```
kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP=<backup file name>
```

The new pod makes the `prerestore` backup, swaps the named backup in, and prints `restored file=...`. While the variable is set the service **never migrates**. If the restored schema is older than the image expects, it prints `restore mode ...` and waits without serving. Then do one of:

- deploy the older release that matches the backup (the `app` part of its name), or
- remove the variable, so the next start backs up and migrates again: `kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP-`

The next release deploy applies the manifest without the variable and so clears it too. Each value is restored once; a pod restart does not repeat it. To restore the same backup again, add a tag: `GOGEAR_RESTORE_BACKUP=<file name>#2`. A restore to the schema the image expects serves normally.

**A failed migration.** If a migration fails after the backup was made, the backup stays and the service reports it (`migration failed from=<N> target=<M>; the state before the attempt is in file=...`) and writes `backups/migration-failed.json`. The database is then left as the failed migration left it, and the service does not restore it by itself (`Keep`, the current default). Every later start prints `dirty schema=<N>` with the restore command for the newest `premigration` backup, makes no backup and does not start. Restore that backup with the command above, then deploy a fixed release.

The manual methods below still work as an extra, for example for a copy on another disk.

### Notes on individual migrations, and manual backups

Migration `000005` changes data: it clamps loadout item quantities of 0 or below to 1, turns blank loadout slugs into NULL, and deletes identical duplicate container links. The down migration removes the constraints but does not restore those changes. The original quantities stay in the table `audit_000005_clamped_loadout_item_quantity`. Every run of the migration adds its own rows (with a `clampedAt` timestamp), so if the migration is rolled back and applied again and clamps the same item a second time, both original values are kept.

Migration `000007` designates the permanent admin (the admin with the lowest user ID when it runs, or the first account that becomes admin if there is none). That account can never lose admin status or be deleted through the API. Back up first if the lowest-ID admin is not the account you want: the designation lives in the table `permanent_admin`, and changing it is a manual database edit. Rolling back `000007` and applying it again designates the permanent admin anew: it picks the admin with the lowest user ID at that moment, which may be a different account from before.

Migration `000008` deletes container links that break the nesting rules: self-links, links with a missing registration, links between different users' registrations, all but the oldest container of a registration, and one link of each cycle. The down migration restores the indexes but not the deleted links.

Migration `000009` adds the container settings of the catalog categories (`top_category_is_container` on a top category, `category_container` on a sub-category, set through the admin API). They start as "not a container"; migration `000010` then seeds them (below), so no manual step is needed after a deploy. The old per-gear flag is kept as `gear.gearIsContainerLegacy` and is no longer read. It does not change any link. It cannot be applied twice by hand: a second run fails on its first statement and changes nothing. Its down migration discards the category settings and keeps links and the old flag values.

Migration `000010` seeds the container settings from the old per-gear flags: a sub-category with at least one flagged gear item becomes an explicit container category, unless it already has an explicit setting. Top categories are not changed. It runs once per database, whether the database comes from version 8 or 9, and seeds nothing on a fresh database. No admin action is needed after deploying it. Its down migration changes no data (a seeded setting cannot be told apart from one an admin set), so down to 9 keeps the seeded settings. Admins can still change any setting through the admin API afterwards.

After running its migrations the service prints `gogear-api migrations: version=<N> dirty=<true|false>` to stdout, whatever the log level (also when nothing changed). The release workflow checks this line after a deploy. The `gogear-api backup:` lines (above) go to stdout the same way, and the workflow includes them in its failure diagnostics.

The migrations in `migrations/` are the only source of the schema. The old seed scripts `sql/database.sql` and `sql/patch.sql` were removed because they cannot create a current schema.

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
