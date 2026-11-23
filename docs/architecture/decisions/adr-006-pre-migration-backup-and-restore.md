---
type: Architecture Decision
title: ADR-006 Pre-migration backup and one-action restore inside the application
description: How gogear-api realizes BR-038 - it reads the schema version itself before golang-migrate touches the database, takes a VACUUM INTO snapshot while no listener is open, verifies it (integrity, version, no concurrent commit), keeps three, reports every step on stdout, restores a named backup through one Kubernetes env setting, and handles golang-migrate's dirty state, with the owner questions Q-8 and Q-9 as two policy switches.
tags: [architecture, backup, restore, migrations, sqlite, release, deployment, data-safety]
timestamp: 2026-11-22T00:00:00Z
---

# ADR-006 — Pre-migration backup and one-action restore inside the application

**Status:** Accepted (architect, 2026-11-22). Implemented 2026-11-22 (task A6b, package `pkg/dbupgrade`). Ships in the milestone A merge together with migration 000011 (task A6b, [ARD-001](/docs/architecture/design/ard-001-loadout-planning-and-packing.md)). Exact behavior, names and messages: [ARS-002](/docs/architecture/spec/ars-002-pre-migration-backup-and-restore.md). Owner questions Q-8 and Q-9 are open; the design builds the requirement's current defaults and makes each answer a one-constant switch (D-7).

## In brief

**What the system will do:** At startup, before golang-migrate is even constructed, the application reads the database's schema version with plain SQL and compares it with the highest migration in the image. If something is pending and the database already existed, it writes a `VACUUM INTO` snapshot to a temporary file in `backups/` next to the database, checks it (opens, `integrity_check` is `ok`, same schema version, not dirty, nothing committed meanwhile), renames it to its final name, prints one stdout line, trims old backups to three, and only then migrates. To restore, the owner runs one command, `kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP=<backup file name>`; the new pod first backs up the current database, then swaps in the named backup before opening it.

**Why this design:** The application is already the only process that opens the database. Recreate with one replica stops the old pod first, and the HTTP listener opens only after migrations. So a snapshot taken at that point cannot miss a commit, and the application needs no external tooling (the image has no `sqlite3` binary). `VACUUM INTO` is one SQL statement through the existing driver. It reads committed content including the WAL and writes a standalone file. A Kubernetes env setting is a means the deployment already has and needs no node shell. The next release deploy resets it to "no request", because `kubernetes-manifests/deployment.yaml` declares the variable with an empty value and the deploy applies that manifest. That this reset happens is to be confirmed on the first production deploy that carries the declaration.

**Normal example:** Release `v4.11.0` brings migration 000011. The pod starts, reads `version=10 dirty=false`, finds 11 in `migrations/`, and writes `/config/backups/gogear-20261122T181500Z-premigration-schema10-appv4.11.0.db`. It prints `gogear-api backup: created file=… schema=10 app=v4.11.0 kind=premigration integrity=ok` and removes the oldest of four pre-migration backups. Then it migrates and prints the usual `gogear-api migrations: version=11 dirty=false`. The release run's pod check passes as before.

**Most important limits:** Backups live on the same disk as the database (BR-038 out of scope: losing the disk loses both). The guarantee that nothing is committed between snapshot and migration rests on the deployment model (one process, listener not yet open) plus a detection check. The application cannot hold one SQLite lock across golang-migrate's own transactions. Restoring to go back to an older release is two actions: set the env, then deploy the older image (or unset it to migrate again).

## Derived from

- [BR-038](/docs/requirements/requirements/br-038-backup-before-schema-migration.md); provisional defaults P-37 to P-40; open questions Q-8 and Q-9 ([index](/docs/requirements/index.md))
- [Release and deployment decisions — RD1, RD2, RD3](/docs/anchors/release-and-deployment.md)
- [BR-022](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md), [ADR-002](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md) D-8 (in-run pod check: one pod, `restartCount 0`, `gogear-api migrations: version=<N> dirty=false`)
- Code and manifests read this session (branch head `7467b25`): `main.go` (`main`: config → `sql.Open` → `warnIfForeignKeysOff` → `runMigrations` → listener; `runMigrations` prints the version line); golang-migrate v4.19.1 `database/sqlite3/sqlite3.go` (`WithInstance` creates `schema_migrations`; `Run` and `SetVersion` use separate transactions); `kubernetes-manifests/deployment.yaml` (one replica, `Recreate`, `hostPath /data/disk2/opt/gogear-api` mounted at `/config`, config `/config/config.yaml`, no probes, uid `abc` 1001, `fsGroup 1000`); `Dockerfile` (`go build -ldflags="-s -w"`, no version embedded, no `sqlite3` binary); `.github/workflows/release.yaml` (build with `docker/build-push-action` v7.2.0, no build args; pod-log diagnostics grep `panic:|fatal error:|Failed to|gogear-api migrations:`); README "Back up the SQLite file first"
- `[unverified]` the production `database.file` value: `config.yaml` lives on the node, not in the repository. Tests use `/data/gogear.db`. The default backup directory is derived from `database.file`, so it works for any value

## Satisfies

| BR-038 | Satisfied by |
|---|---|
| AC-1 (backup before any pending migration, every release path) | D-1 detection before golang-migrate is constructed; D-2 snapshot before `m.Up()` |
| AC-2 (consistent, WAL included, no commit in between, verified, records old version) | D-2 `VACUUM INTO` while no listener is open, plus a `data_version` check; D-3 verification |
| AC-3 (where, identity, stdout, protection) | D-4 directory, name, file mode, stdout lines |
| AC-4, AC-5 (nothing pending, first start) | D-1 |
| AC-6 (keep 3, delete after verify, only own files) | D-5 |
| AC-7 (no user step) | everything runs in startup |
| AC-8 (backup fails) | D-7 policy `OnBackupFailure = refuse` (P-39) |
| AC-9 (restore: one action, whole replace, pre-restore backup, report, older release) | D-6 |
| AC-10 (migration fails after a good backup: keep, report, no auto-restore until Q-9) | D-7 policy `OnMigrationFailure = keep`; D-8 dirty state |
| AC-11 (SQLite only) | scope of the design |

## Decision question

How does the application detect pending migrations, take and verify a consistent backup with no commit in between, store, label, announce and retain it, let the owner restore with one action, and stay compatible with golang-migrate's dirty state, the Recreate deploy and the release run's pod check?

## Options considered

**Snapshot mechanism.**
- **S-A `VACUUM INTO` (chosen):** one SQL statement through `database/sql`. It reads one consistent read transaction, WAL content included, and writes a standalone, compacted database with no `-wal` file. Costs a full rewrite of the database, which is small here, and the target file must not exist (a fresh temporary name always).
- **S-B SQLite online backup API** (`sqlite3.SQLiteConn.Backup` through `conn.Raw`): also consistent, and copies pages without a rewrite. Costs driver-specific code. It restarts the copy if another connection writes, which adds nothing here. Rejected as more code for no gain.
- **S-C file copy after a checkpoint:** a plain copy can tear, and a later write can land in between; the README already warns against it. Rejected.

**Closing the gap between snapshot and migration.**
- **G-A Process model plus detection (chosen):** the snapshot runs after the old pod has stopped (Recreate, one replica) and before the listener opens, so no request can write. The snapshot connection records `PRAGMA data_version` before `VACUUM INTO` and again right before `m.Up()`. A change means another connection committed (for example, a person running `sqlite3` on the node), and the backup is discarded and counted as failed.
- **G-B One lock across backup and migration:** take `BEGIN IMMEDIATE` and run the migrations on the same connection. golang-migrate opens its own transactions per migration, so this needs a hand-written migration runner. Rejected: it replaces a working, tested component (and its dirty-state semantics) to close a window that has no writer under the deployment model.

**Restore trigger.**
- **R-A Env setting on the Deployment (chosen):** `kubectl set env` is one command. Kubernetes rolls the pod (Recreate), the new pod restores before opening the database. Each request value is performed once (D-6), so restarts do not repeat it. The manifest declares `GOGEAR_RESTORE_BACKUP` with an empty value, so the next release deploy sets it back to empty, which the application treats as no request. *Corrected 2026-11-22 (review, Loom qa `bcfb1772`; fix `d34c18d`):* the earlier text said the deploy would drop a variable the manifest does not name. It does not: a client-side `kubectl apply` keeps a variable added by `kubectl set env`, because the variable was never in the applied manifest. The reset works only because the manifest names the variable. It must be confirmed on the first deploy that carries the declaration: after the rollout, `kubectl -n seashell get deployment gogear-api -o jsonpath='{.spec.template.spec.containers[0].env}'` shows `GOGEAR_RESTORE_BACKUP` with value `""`.
- **R-B Config file key:** the config lives on the node's `hostPath`; editing it needs node access, which AC-9 rules out.
- **R-C A dispatched "restore" workflow** that scales down, runs a Job with `--restore`, and scales up: one click, but a new workflow with cluster permissions and three steps that can fail between them. Rejected for now; a revisit trigger is to use it if `kubectl` access is not available to the owner.

**Where policy for Q-8 and Q-9 lives.**
- **P-A Two constants in one policy struct passed into the startup routine (chosen):** an owner answer flips one constant, tests exercise both values, and there is no setting anyone can misconfigure.
- **P-B Config keys:** more surface and validation, and a setting that can silently disable protection. Rejected.

## Decision

- **D-1 — Detect pending migrations without touching the database.** Before `sql.Open` for the service pool, record whether the database file exists with size > 0 (*existed*). After opening, read `SELECT version, dirty FROM schema_migrations LIMIT 1` with plain SQL. A missing table or no row means *no version*. Do not construct golang-migrate yet: `WithInstance` creates `schema_migrations`, which is a write. The highest available version comes from golang-migrate's file source driver over `migrations/` (`First` and `Next` to the end). A migration is *pending* iff the database has no version or `version < latest`. Backup iff pending and existed (AC-4, AC-5).
- **D-2 — Snapshot.** On one dedicated `*sql.Conn` of the service pool: read `PRAGMA data_version`, then `VACUUM INTO ?` with a temporary path `<final name>.partial` in the backup directory. Verify it (D-3), read `data_version` again (still equal), `fsync` the file, rename it to the final name, and `fsync` the directory. Only then construct golang-migrate and call `m.Up()`. The listener is opened only after `runMigrations` returns, as today; this ordering is now an invariant with a test.
- **D-3 — Verification.** Open the temporary file read-only (`file:<path>?mode=ro`, without the service DSN options, which would switch it to WAL). Check: `PRAGMA integrity_check` returns exactly one row `ok`; `schema_migrations` holds the same version and `dirty = 0` (or no version, for a legacy database without the table); and `data_version` is unchanged on the source connection. Any failure deletes the `.partial` file and is a backup failure.
- **D-4 — Location, identity, announcement.** Directory: `database.backup-dir` in the config (new optional key), default `<directory of database.file>/backups`, which is `/config/backups` on the existing `hostPath` volume if the database lives in `/config`. It is created with mode `0700`; backups get the database file's own permission bits. Name: `gogear-<UTC yyyymmddThhmmssZ>-<kind>-schema<N>-app<version>.db`, with kind `premigration` or `prerestore` and characters outside `[A-Za-z0-9._-]` in the version replaced by `_`. The application version is embedded at build time (`-X main.appVersion=<tag>`; Docker build argument `VERSION` from the release tag; `dev` otherwise). Every step prints one stdout line beginning with `gogear-api backup:` (ARS-002), and at every start the kept backups are listed, so the owner finds names in the pod log without the node.
- **D-5 — Retention.** After a new backup is verified and renamed, keep the three newest of its kind, by the timestamp in the name, and delete older ones of that kind. Only files in the backup directory whose names match the exact pattern are ever deleted or counted. Leftover `*.partial` files that match the pattern are deleted at startup and never counted. Pre-restore backups have their own pool of three, so a restore never pushes out a pre-migration backup.
- **D-6 — Restore.** Trigger: env `GOGEAR_RESTORE_BACKUP=<file name>[#<tag>]`, read at startup before the database is opened. An empty or absent value is no request. `kubernetes-manifests/deployment.yaml` declares the variable with value `""`, so every release deploy resets an owner's setting to no request (to be confirmed on the first deploy, see R-A). The file name must match the pattern and exist in the backup directory, and the name may contain no path. A request already performed (recorded in `backups/restore-done.json` by its full value) is skipped. `#<tag>` makes a repeat of the same backup a new request. Steps:
  1. verify the named backup (D-3, without the `data_version` part); if that fails, change nothing and exit 1;
  2. if the database existed, make a verified `prerestore` backup of it (D-2, D-3);
  3. `PRAGMA wal_checkpoint(TRUNCATE)` and close every connection;
  4. copy the backup to `<database>.restore-partial` and `fsync` it; remove `-wal` and `-shm`; rename onto the database file; `fsync` the directory;
  5. record the request and print `restored`.

  Then normal startup continues, with one difference: **while the variable is set, the application never migrates.** If the restored schema equals the image's latest, it serves normally. If it is older, it prints the restore-mode line and blocks without opening the listener, until the owner either deploys the older release (the image that matches the backup's `app` label) or removes the variable (`kubectl set env … GOGEAR_RESTORE_BACKUP-`), after which the next start backs up and migrates as usual (AC-9).
- **D-7 — Policy switches for Q-8 and Q-9.** `Policy{OnBackupFailure, OnMigrationFailure}`, set from two constants in one file.
  - `OnBackupFailure`: `Refuse` (default, P-39): print the failure line, apply no migration, exit 1. `MigrateWithoutBackup` (Q-8 option b): print a `WARNING … migrating without a backup` line and continue.
  - `OnMigrationFailure`: `Keep` (default; BR-038 AC-10 says no automatic restore until Q-9 is answered): record `backups/migration-failed.json` (app version, from and target version, backup name), print the line naming the backup, exit 1. `Restore` (Q-9 recommended): additionally restore that backup by D-6 steps 3 to 5 (no pre-restore backup: the failed attempt changed only golang-migrate's dirty flag, because each SQLite migration runs in one transaction), then exit 1.
- **D-8 — Dirty state.** If the plain read of D-1 finds `dirty = 1`, a previous migration failed. The application makes no backup (the database is flagged inconsistent), does not migrate (golang-migrate would refuse with `ErrDirty`), prints the dirty line naming the newest pre-migration backup from `migration-failed.json`, and exits 1. Under `OnMigrationFailure = Restore` this only occurs if the automatic restore itself failed. Under `Restore`, when the previous attempt was by the same app version and to the same target, the new backup *replaces* the backup named in `migration-failed.json` instead of rotating the oldest one out. It is a copy of the same restored state, so retries after Kubernetes restarts never push older backups out. A successful migration deletes `migration-failed.json`.
- **D-9 — Release workflow.** No new gate. The deploy applies `kubernetes-manifests/deployment.yaml`, which declares `GOGEAR_RESTORE_BACKUP` empty, so a release rollout also clears any restore request (D-6); the first deploy with that declaration must confirm it (R-A). The backup adds seconds before the migration line; the 180 s wait of ADR-002 D-8 is unaffected. A backup or migration failure exits the container, so `restartCount` rises and the existing check fails the run visibly (AC-8). One change: the log filter `safe_logs` in `.github/scripts/verify-deployed-pod.sh` (the deploy check of [ADR-002](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md) D-11) adds `gogear-api backup:`, so the reason appears in the Actions log. Its stub test (T-21) covers the line. The lines contain only paths, times and versions, no user data. The build step passes `build-args: VERSION=${{ needs.release.outputs.tag }}`.

## Cost of the owner choices

| Choice | Code cost | Operational consequence |
|---|---|---|
| Q-8 (a) refuse (default) | none beyond D-7 | a full disk stops the service until fixed; the release run fails visibly; every upgrade is protected |
| Q-8 (b) migrate and warn | one branch and one line | the service stays up, but that upgrade has no way back; the release run passes, so the warning is seen only in the pod log, unless the workflow also greps for it (one more line in `release.yaml`) |
| Q-9 keep and report (current default) | dirty line and `migration-failed.json` | the service stays down at the dirty version (crash loop, visible); the owner runs one restore command, then deploys a fixed release |
| Q-9 restore and stop (recommended) | restore-on-failure path reusing D-6, plus the replace-not-rotate retention rule of D-8 and its tests (about one more test file) | the database is automatically back at the old version with a clean flag; Kubernetes restarts retry the same migration, and a transient cause (a full disk that was cleared) heals with no action; a persistent cause still crash-loops visibly until a fixed release |

## Consequences

- `main.go` gains a startup routine (in a new package) before the existing `runMigrations`. The README's manual-backup section is replaced by the automatic backup and the restore steps.
- `Dockerfile` and `release.yaml` change in one place (version build argument), and the deploy check script changes in one place (diagnostics grep, with its stub test).
- `kubernetes-manifests/deployment.yaml` declares `GOGEAR_RESTORE_BACKUP` with an empty value. Removing that declaration would bring back the bug found in review: a restore request would survive release deploys, and the pod would stay in restore mode after every later release.
- Open confirmation: the reset is reasoned from how client-side `kubectl apply` merges a list entry the manifest names, and it was found in review, not observed on the cluster. The first production deploy with the declaration must show the value `""` (R-A).
- Disk use: up to six database-sized files in `backups/`.

**Revisit triggers:** the owner has no `kubectl` access (then R-C); a database grows large enough that `VACUUM INTO` delays startup past the release wait (then S-B and a longer wait); a second process or replica ever opens the database (then G-A no longer holds, and G-B or an external lock is needed).

## Confirmation

The tests listed in ARS-002 section 6, all against real files and the real startup routine. The decisive ones: WAL-only rows are present in the backup; a commit from a second connection between snapshot and migration is detected; a full or unwritable backup directory under `Refuse` leaves the database at the old version; four upgrades leave three backups; restore through the env variable, including restore mode for an older schema; dirty state on restart; and both Q-9 policies.
