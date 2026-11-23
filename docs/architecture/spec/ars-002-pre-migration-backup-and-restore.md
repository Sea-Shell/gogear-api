---
type: Architecture Specification
title: ARS-002 Pre-migration backup and restore - startup sequence, names, messages and tests
description: Exact startup order, configuration key, environment variable, file names, state files, stdout lines, exit behavior and required tests for the automatic pre-migration backup and the one-action restore of BR-038 (ADR-006), plus the build and workflow changes.
tags: [architecture, backup, restore, migrations, sqlite, startup, specification]
timestamp: 2026-11-22T00:00:00Z
---

# ARS-002 — Pre-migration backup and restore: startup sequence, names, messages and tests

**Status:** Accepted (architect, 2026-11-22). Implemented 2026-11-22 (task A6b). Realizes [ADR-006](/docs/architecture/decisions/adr-006-pre-migration-backup-and-restore.md) for [BR-038](/docs/requirements/requirements/br-038-backup-before-schema-migration.md). Implemented as task A6b of milestone A and merged in the same PR as migration 000011 ([ARD-001](/docs/architecture/design/ard-001-loadout-planning-and-packing.md) merge rule).

## In brief

One startup routine, called by `main` after the config is loaded and before the listener opens, owns everything here. It returns either "continue" (the database is at the image's latest version) or "hold" (restore mode with an older schema). On any error it returns that error, and `main` exits 1. Everything it does is announced on stdout with the prefix `gogear-api backup:`.

## Satisfies

BR-038 AC-1 to AC-11, as mapped in ADR-006.

## 1. Surface

| Item | Value |
|---|---|
| Package | new `pkg/dbupgrade` (startup routine, backup, verify, retention, restore, policy); `main.go` calls it |
| Config key | `database.backup-dir` (optional string). Absent or blank: `filepath.Join(filepath.Dir(database.file), "backups")`. A relative value is resolved against the directory of `database.file`. It must not equal `database.file`'s directory itself. That is refused at startup as invalid configuration, so backups never mix with the database and its `-wal`/`-shm` files |
| Env (restore) | `GOGEAR_RESTORE_BACKUP=<file name>[#<tag>]`; empty or absent = no request. Declared in `kubernetes-manifests/deployment.yaml` with `value: ""`, so each release deploy resets it (ADR-006 R-A; confirm on the first deploy) |
| Build | `var appVersion = "dev"` in `main`; `Dockerfile`: `ARG VERSION=dev` and `-ldflags="-s -w -X main.appVersion=${VERSION}"`; `release.yaml` build step: `build-args: VERSION=${{ needs.release.outputs.tag }}` |
| Workflow diagnostics | the pod-log filter `safe_logs` in `.github/scripts/verify-deployed-pod.sh` (ADR-002 D-11; test T-21 in `verify-deployed-pod_test.sh`) becomes `panic:|fatal error:|Failed to|gogear-api migrations:|gogear-api backup:` |
| Policy | `type Policy struct{ OnBackupFailure BackupFailure; OnMigrationFailure MigrationFailure }`; defaults `Refuse` and `Keep` as two constants in `pkg/dbupgrade/policy.go`. Changing an owner answer edits only those constants |

## 2. Files in the backup directory

- Directory mode `0700`, created if missing (`MkdirAll`). Backups use the database file's permission bits (`0600` if the database does not exist).
- **Backup name:** `gogear-<yyyymmddThhmmssZ>-<kind>-schema<N>-app<version>.db`.
  - Time in UTC; kind is `premigration` or `prerestore`.
  - `<N>` is the decimal schema version without padding, or `none` for a database with no version.
  - `<version>` is `appVersion` with every character outside `[A-Za-z0-9._-]` replaced by `_`.
  - Example: `gogear-20261122T181500Z-premigration-schema10-appv4.11.0.db`.
  - If the name already exists (two backups in one second), add `-2`, `-3` and so on before `.db`.
- **Pattern** (the only files the routine counts or deletes): `^gogear-\d{8}T\d{6}Z-(premigration|prerestore)-schema(\d+|none)-app[A-Za-z0-9._-]+(-\d+)?\.db$`. Temporary files are the same name plus `.partial`.
- **State files** (JSON, mode `0600`, written to a temporary file and renamed):
  - `migration-failed.json`: `{"app_version", "from_version", "target_version", "backup", "at"}`;
  - `restore-done.json`: `{"requests": [{"value", "backup", "at"}]}`, keeping the last 20 entries.

## 3. Startup sequence

1. **Clean up:** delete `*.partial` files in the backup directory that match the pattern.
2. **List:** print one `kept` line per kept backup, newest first.
3. ***existed*** := the database file exists and its size is > 0.
4. **Restore request:** if `GOGEAR_RESTORE_BACKUP` is set and its full value is not in `restore-done.json`, run the restore (section 4). If the value is already recorded, print `restore already done` and continue.
5. Open the service pool (`sql.Open` with `utils.SQLiteDSN`), as today. `warnIfForeignKeysOff` stays.
6. **Read the version with plain SQL:** `SELECT version, dirty FROM schema_migrations LIMIT 1`. A missing table or no row means `none`. **latest** := the highest version from golang-migrate's `source/file` driver over `migrations/`.
7. **Dirty** (`dirty = 1`): print the `dirty` line and return an error (exit 1). No backup, no migration.
8. **Restore mode:** if `GOGEAR_RESTORE_BACKUP` is set and `version < latest` (or `none`), print the `restore mode` line and return "hold". `main` then blocks until SIGTERM without opening the listener.
9. **Pending:** if `version ≥ latest`, print `skipped reason=no-pending` and go to step 12. If the database did not exist (step 3), print `skipped reason=first-start` and go to step 11.
10. **Backup:** on one `*sql.Conn` from the pool:
    - read `PRAGMA data_version` → *d1*;
    - run `VACUUM INTO ?` with the `.partial` path;
    - verify the file (section 5);
    - read `PRAGMA data_version` → *d2* and require *d1* = *d2*;
    - `fsync` the file, rename it to the final name, `fsync` the directory;
    - print `created`;
    - apply retention (keep 3 per kind) and print `removed` per deleted file;
    - release the connection.

    On any failure: delete `.partial`, print `FAILED`, and then follow `Policy.OnBackupFailure`. `Refuse` returns an error. `MigrateWithoutBackup` prints `WARNING` and continues.
    Under `OnMigrationFailure = Restore` only: if `migration-failed.json` has the same `app_version`, `from_version` = *version* and `target_version` = *latest*, then after the new backup is verified, delete the backup named there (if it matches the pattern) instead of applying the normal retention.
11. **Migrate:** construct golang-migrate (`sqlite3.WithInstance`, `NewWithDatabaseInstance`) and call `m.Up()`, as in today's `runMigrations`.
    - Success: delete `migration-failed.json` if present.
    - Failure: write `migration-failed.json` and print `migration failed`. `Keep` returns the error. `Restore` restores the backup of step 10 by section 4 steps 3 to 5, prints `restored after failed migration`, and returns the error. If there was no backup (`MigrateWithoutBackup`), it only reports.
12. Print the existing `gogear-api migrations: version=<N> dirty=<bool>` line (unchanged, ADR-002 D-8) and return "continue". `main` then opens the listener, as today.

**Invariant:** the listener (`router.Run`) is reached only after the routine returns "continue". A test must fail if any request can be answered before that.

## 4. Restore

1. **Parse the value:** the file name is everything before the first `#`. It must match the pattern and contain no path separator; otherwise print `restore FAILED reason=invalid-name` and return an error. The file must exist in the backup directory; otherwise `reason=not-found`.
2. **Verify** the named backup (section 5, without `data_version`). On failure: `reason=verify`, return an error, nothing changed.
3. If the database existed: make a `prerestore` backup of it by step 10, using a temporary pool opened just for this, and apply retention to the `prerestore` kind. Failure: `restore FAILED reason=prerestore-backup`, return an error, nothing changed.
4. Open the database once more, run `PRAGMA wal_checkpoint(TRUNCATE)`, and close every connection to it. The service pool is not open yet during a requested restore. For the restore after a failed migration, the routine closes its pool first and reopens it afterwards.
5. Copy the backup to `<database file>.restore-partial` and `fsync` it. Delete `<database file>-wal` and `-shm` if present. Rename onto the database file and `fsync` the directory.
6. Record the request value in `restore-done.json` (requested restores only). Print `restored`.

A crash at any point leaves either the old database (checkpointed, with its pre-restore backup made) or the restored one. Never a mix.

## 5. Verification of a backup file

Open `file:<path>?mode=ro` with the sqlite3 driver and no other options (not `utils.SQLiteDSN`, whose `_journal_mode=WAL` would write). Then:

1. `PRAGMA integrity_check` returns exactly one row, `ok`.
2. `SELECT version, dirty FROM schema_migrations LIMIT 1` gives the expected version with `dirty = 0`. For a pre-migration or pre-restore backup the expected version is the source's version read in step 6 (or `none`); for a restore it is the version in the file name.
3. Close the connection.

## 6. Stdout lines

Exact prefixes and fields. Values never contain user data.

```text
gogear-api backup: kept file=<path> kind=<kind> schema=<N> app=<version>
gogear-api backup: skipped reason=no-pending|first-start
gogear-api backup: created file=<path> kind=premigration schema=<N> app=<version> integrity=ok
gogear-api backup: removed file=<path> reason=retention|replaced-retry
gogear-api backup: FAILED reason=<short reason>; no migration applied, database unchanged at schema=<N>
gogear-api backup: WARNING backup failed (reason=<short reason>); migrating without a backup
gogear-api backup: migration failed from=<N> target=<M>; the state before the attempt is in file=<path>
gogear-api backup: restored after failed migration file=<path> schema=<N>
gogear-api backup: dirty schema=<N>; a previous migration failed. Restore with: kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP=<name of newest premigration backup>
gogear-api backup: restored file=<path> schema=<N> app=<version>; replaced database kept as file=<path>|none
gogear-api backup: restore already done value=<value>; remove GOGEAR_RESTORE_BACKUP or add #<tag> to restore again
gogear-api backup: restore FAILED reason=invalid-name|not-found|verify|prerestore-backup|swap; database unchanged
gogear-api backup: restore mode schema=<N> latest=<M>; not migrating and not serving. Deploy the release that matches schema <N> (app=<version> of the backup), or remove GOGEAR_RESTORE_BACKUP to migrate again
```

The same reasons are also logged through the logger at error level. Only `skipped`, `kept`, `created` and `removed` are info lines.

## 7. README (replaces "Back up the SQLite file first")

Document:
- the automatic backup (when it happens, the directory, names, retention of three per kind, the stdout lines);
- how to find the backup names (`kubectl -n seashell logs deployment/gogear-api | grep 'gogear-api backup:'`);
- the restore command and its two follow-ups (deploy the older release, or clear the request with `kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP=`);
- that the next release deploy resets the variable to empty, because the manifest declares it empty. A client-side `kubectl apply` would keep a variable the manifest does not name. Removing it with the trailing-`-` form also works, and the next deploy adds it back as empty;
- how to check the reset after a deploy (`kubectl -n seashell get deployment gogear-api -o jsonpath='{.spec.template.spec.containers[0].env}'` shows `GOGEAR_RESTORE_BACKUP` with `""`);
- `#<tag>` to repeat a restore;
- what a dirty start means and the current `OnMigrationFailure` behavior;
- that backups are on the same disk as the database.

Keep the WAL and Recreate notes. The manual `sqlite3` method may stay as an extra option.

## 8. Required tests

All tests use real files in `t.TempDir()` and call the startup routine, not internals.

1. **Consistent backup with WAL:** seed a version-10 database; keep a second connection open with `wal_autocheckpoint=0` and commit rows that are only in `-wal`; run with a migration 11 → the backup exists, passes `integrity_check`, reports schema 10, and contains the WAL-only rows; the database is at 11.
2. **Concurrent commit:** inject a commit from another connection between `VACUUM INTO` and the second `data_version` read (test hook) → the backup counts as failed; under `Refuse` nothing migrates.
3. **Nothing pending** → no new file, `skipped reason=no-pending`. **Empty volume** → no backup, `skipped reason=first-start`, database migrated.
4. **Backup failure:** make the backup directory unwritable, and separately make the directory path a regular file → under `Refuse`: error, version still 10, no `.partial` or counted file, a `FAILED` line; after fixing the cause, the next run backs up and migrates. Under `MigrateWithoutBackup`: migrated, `WARNING` line.
5. **Retention:** four upgrades (synthetic migrations 11 to 14) → three `premigration` backups remain, the oldest went only after the fourth was verified, and a foreign file in the directory stays untouched.
6. **Restore:** with a version-11 database and kept backups → set the variable to the schema-10 backup → a `prerestore` backup of the version-11 database exists, the database equals the schema-10 backup (row comparison), routine returns "hold" with the restore-mode line, and no migration ran. Second run with the same value → `restore already done`, still "hold". Value with `#2` → restored again. Invalid name (`../x.db`) and an unknown name → error, database unchanged.
7. **Restore to the same schema** → "continue", serves.
8. **Failed migration, `Keep`:** a synthetic failing migration 12 after a good backup → error, `migration-failed.json` written, backup kept; next run → `dirty` line, error, no new backup.
9. **Failed migration, `Restore`:** same → database restored, `dirty = 0` at the old version; next run (same app version) → a new backup replaces the previous one (`removed … reason=replaced-retry`), older backups are untouched, and the migration is tried again.
10. **Listener order:** a test hook proves the routine has returned before `router.Run` is reached.
11. **Config validation:** `database.backup-dir` equal to the database directory is refused.

## 9. Task A6b (one coder session, inside the milestone A merge)

- **Goal:** BR-038 in effect in the milestone A merge, so the first production start that applies 000011 makes a verified backup.
- **Authority:** BR-038, ADR-006, this document. Q-8 and Q-9 stay at the defaults `Refuse` and `Keep`. Changing them later edits only `pkg/dbupgrade/policy.go` (tests 4 and 8 or 9 already cover both values).
- **Change surface:**
  - new `pkg/dbupgrade/` (routine, backup, verify, retention, restore, policy, tests);
  - `main.go` (call the routine in place of the direct `runMigrations` call, keep the version line, hold mode, `appVersion`);
  - `pkg/models/config.go` (`Database.BackupDir`) and `validateConfig`;
  - `kubernetes-manifests/deployment.yaml` (declare `GOGEAR_RESTORE_BACKUP` with `value: ""` and a comment saying why; done in `d34c18d`);
  - `Dockerfile` (ARG and ldflags);
  - `.github/workflows/release.yaml` (build-args);
  - `.github/scripts/verify-deployed-pod.sh` and `verify-deployed-pod_test.sh` (diagnostics grep, test T-21);
  - `README.md` (section 7);
  - the requirements index status of BR-038 and the ADR-006 and ARS-002 status lines;
  - existing migration tests only if the extracted `runMigrations` signature changes.
- **Not in scope:** off-disk copies, scheduled backups, encryption, a restore workflow.
- **Confirmation after merge (not a merge gate):** the first production deploy that carries the manifest declaration shows `GOGEAR_RESTORE_BACKUP` with value `""` in the Deployment spec (section 7 command). This proves the reset of ADR-006 R-A.
- **Done when:** the tests of section 8 pass with `go test -race -count=1 ./...`, plus `go vet`, `gofmt` and the Swagger drift check (no API change expected). The replay suite and the milestone A tests stay green. A local `docker build --build-arg VERSION=v0.0.0-test` succeeds and the binary prints that version in a backup name.
- **Dependencies:** none on A1 to A6, other than being in the same merge. It must land before the milestone A PR is merged.
