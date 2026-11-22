---
type: Architecture Decision
title: ADR-001 Realizing category-driven container capability
description: How the API stores, exposes and enforces the container setting of top categories and sub-categories (BR-021), derives every gear item's container status from one SQL view, refuses links into non-capable containers in the API and the database, and migrates with a new migration 000009 that keeps the old per-gear flags recoverable.
tags: [architecture, containers, catalog, sqlite, migration, api-contract]
timestamp: 2026-11-22T00:00:00Z
---

# ADR-001 — Realizing category-driven container capability

**Status:** Accepted (architect, 2026-11-22). Implemented on 2026-11-22: migration 000009 and the derived status and link guard in the first change, the admin API for the settings in the second. The decisions are unchanged. An independent review of the implementation is pending. **Amended 2026-11-22:** P-14 is superseded by P-18 (BR-021 AC-5). The seeding from D-5 is realized in migration `000010` by [ADR-002](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md) D-7, which closes OQ-1.

## In brief

**What changes:** Each top category gets a container setting (yes or no). Each sub-category gets an optional explicit setting (yes, no or none). A gear item's container status is no longer stored on the gear. It is computed by one SQL view from the gear's sub-category. Every gear and registration response reports the computed value in the existing `gear_is_container` field, which becomes read-only. Linking into a registration whose gear is not container-capable is refused with 409, both by the API and by a database trigger. Migration `000009` adds the columns, the views and the triggers. It renames the old per-gear column to `gearIsContainerLegacy`. Nothing reads that column, but its values stay in place.

**Why this design:** One SQL view is the only definition of the rule. Go queries and the database trigger both use it, so no two code paths can disagree. A three-valued text field for the sub-category setting keeps the existing rule that JSON `null` means "not sent", and still lets an admin remove the explicit setting. The existing immediate write transactions already serialize the setting change against link requests (BR-021 AC-8). The new code only has to read the capability inside the link transaction.

**Normal example:** An admin sends `POST /api/v1/topCategory/3/update {"top_category_is_container": true}` for "Bags". The sub-category "Backpacks" has `category_container: "inherit"`, so `GET /api/v1/category/7/get` reports `category_is_container: true`. Every gear item in "Backpacks" reports `gear_is_container: true`, and `GET /api/v1/gear/list?container=true` returns them. A user's backpack registration can now receive contents. The admin then sends `{"category_container": "not_container"}` for "Backpacks". Existing links stay, and a new link into the backpack is 409 `this registration cannot hold items: its gear is not in a container category`.

**Most important limits:** Down-migrating `000009` discards all category settings; links and the old per-gear flags survive. `000009` cannot be replayed on a database that already has it: the replay fails, rolls back and changes nothing. The gear's own top-category reference never affects the status (BR-021 AC-3).

## Derived from

- [BR-021 The catalog category decides whether gear can be a container](/docs/requirements/requirements/br-021-container-capability-from-category.md)
- [BR-020 Containers nest to any depth, with one direct container per registration and no cycles](/docs/requirements/requirements/br-020-container-nesting-consistency.md) (as amended by D11)
- [BR-001 Catalog changes are admin-only](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) AC-6
- [BR-013 Container links involve only the caller's own registrations](/docs/requirements/requirements/br-013-container-links-owner-scoped.md)
- [BR-018 Refused requests change nothing and use a defined status](/docs/requirements/requirements/br-018-denial-outcomes.md)
- [Access control and identity decisions](/docs/anchors/access-control-and-identity.md), D11

## Satisfies

The provisional defaults P-13 to P-17 (P-11 is superseded) are listed in the requirements index (`docs/requirements/index.md`, section "Provisional defaults").

| Requirement | Satisfied by | Evidence that would prove it |
|---|---|---|
| BR-021 AC-1 (settings and inheritance) | D-1 columns, D-3 view `category_container_setting` | AC-1 three examples through the API (E2E-001) |
| BR-021 AC-2, BR-001 AC-6 (admin-only writes, open reads) | D-2 fields on the existing catalog routes behind `RequireAdmin` | non-admin create/update of each setting is 403 and nothing changes; reads work for any authenticated user |
| BR-021 AC-3 (status only from the effective setting, at read time) | D-3 view `gear_container_capability`, D-4 read-only `gear_is_container`, D-5 rename of the old column | every gear/registration/container-contents response equals the view; a gear whose top-category reference disagrees; a write that sends `gear_is_container` has no effect |
| BR-021 AC-4 / P-13 (missing sub-category is not capable) | D-3 `COALESCE(…, 0)` over a `LEFT JOIN` | gear row with a dangling `gearCategoryId` (seeded with foreign keys off) reports `false` |
| BR-021 AC-5 / P-14 (start values, no carry-over; superseded by P-18, seeding in ADR-002 D-7) | D-6 column defaults; no seeding statement in `000009` | migration test on a v8 database with old flags set: every setting is the default, and every gear reports `false` |
| BR-021 AC-6 / P-15 (existing links stay) | D-7 trigger fires only on link insert/update; no migration step touches links | AC-6 example; migration test keeps all links |
| BR-021 AC-7 / P-16 (`container=true\|false`) | D-3 view, joined by the filters with the same expression as the reported value | filter results match the reported value after each catalog change |
| BR-021 AC-8 (setting change vs link) | D-8 capability read inside the immediate link transaction; D-7 trigger | repeated race test: no link into a not-capable container is stored after the change was answered |
| BR-020 AC-1, AC-5 / P-17 (409, order) | D-9 order in `InsertContainer` and message mapping | precedence tests (404, then 400 self-link, then not-capable 409, then the AC-3/AC-4 409s) |
| BR-020 AC-2 to AC-4, AC-6 to AC-8 | unchanged wave 7 mechanisms; fixtures must use container-capable gear | wave 7 tests still pass with capable fixtures |

Nothing applicable is deferred. BR-002 (user contributions) needs nothing here: AC-3 holds for any gear row, whoever inserted it.

## Decision question

How should BR-021 and amended BR-020 AC-1 be realized in this single-instance SQLite service so that the API contract, the migration and the enforcement are fixed before coding? The ADR covers seven linked choices: the API representation of the settings, the fate of `gear_is_container`, where the rule is computed, database enforcement, the AC-8 race, the migration shape, and status mapping.

## Drivers and constraints

- One source of truth for the effective setting (AC-3). A second computation anywhere (Go, SQL text pasted in several queries, or the client) is a falsification risk.
- Catalog updates treat JSON `null` as "not sent" (`catalogUpdate`, `pkg/api/catalog.go`; recorded in BR-001's implementation note and the wave 4 index entry). That rule stays for every field.
- P-14 must stay cheap to reverse: the old per-gear flags must survive the migration.
- Wave 7 precedent: rules the API checks are repeated by the database (unique index and cycle trigger, `migrations/000008_container_nesting.up.sql`).
- Every connection uses `_txlock=immediate`, `_busy_timeout=5000`, enforced foreign keys and WAL (`utils.SQLiteDSN`, `pkg/utils/main.go`). One instance only (README, `strategy: Recreate`).
- `000008` is not on `main` (`git log main -- migrations/000008*` is empty), but it is already applied to test databases.
- The query layer builds SELECT and INSERT column lists from `db:` struct tags (`utils.GetDBFieldNames`, `GenericInsert`, `GenericGet`). Gear list, search and get use `models.GearListItem` and `models.FullGear`. Registration list, get and container contents use `userGearColumns` and `userGearCatalogJoins` in `pkg/api/usergear_queries.go`.
- Out of scope: physical fit and weight limits, top-category/sub-category consistency of gear, and BR-002.

## Options considered and decisions

### D-1 Storage of the settings

**Decision:** two new columns.

- `gear_top_category.topCategoryIsContainer INTEGER NOT NULL DEFAULT 0 CHECK (topCategoryIsContainer IN (0, 1))`
- `gear_category.categoryIsContainer INTEGER DEFAULT NULL CHECK (categoryIsContainer IS NULL OR categoryIsContainer IN (0, 1))`. `NULL` means "no explicit setting".

`ALTER TABLE … ADD COLUMN` accepts a column CHECK without a table rebuild, which 000005 had to avoid. Rejected alternative: a separate settings table keyed by category. It adds a join and an orphan case and buys nothing: a setting has no history and no other attributes.

### D-2 API representation (set, change, remove)

**Decision:**

| Resource | Field | Read | Create | Update |
|---|---|---|---|---|
| Top category | `top_category_is_container` (bool) | always present | absent or `null` → `false` | absent or `null` → unchanged; `true`/`false` sets it |
| Sub-category | `category_container` (string: `"container"`, `"not_container"`, `"inherit"`) | always present; `"inherit"` when there is no explicit setting | absent or `null` → `"inherit"` | absent or `null` → unchanged; `"inherit"` **removes** the explicit setting; `"container"`/`"not_container"` sets it |
| Sub-category | `category_is_container` (bool, effective setting) | always present | ignored (read-only) | ignored (read-only) |
| Category list item | `top_category_is_container` (bool) | present with the other embedded `top_category_*` fields | — | — |

Rules:

- Any other value of `category_container` (another string, a number, a bool) is 400 with `category_container must be "container", "not_container" or "inherit"`. It is a malformed body, so it is decided where invalid JSON is decided today (before the 404 check). Non-admins never get this far (403 from `RequireAdmin`).
- A read-only field counts as "not an updatable field". A body that holds only read-only fields is 400 `no updatable fields in request body`, as for an empty body today.
- A `category_container` update is one partial `UPDATE` column like the others (`categoryIsContainer = NULL | 0 | 1`), so it goes through `catalogUpdate.apply` with its 404 → body-ID 400 → empty-body 400 order unchanged. `catalog.go` itself needs no change.

Alternatives:

- *Nullable bool where `null` means remove.* This would break the BR-001 "`null` is not sent" rule for one field. A client that echoes a read response back (all fields, `null` for none) would then silently remove settings. Rejected.
- *Bool field plus a separate remove endpoint or flag* (`DELETE /category/{id}/container`, or `"category_container_inherit": true`). This works, but gives two ways to express one value and a conflict case (`true` plus `inherit`) that needs its own rule. Rejected as more surface for the same meaning.
- *Same three-valued field on top categories.* A top category always has a value (BR-021 AC-1), so "inherit" would be meaningless there. Rejected.

### D-3 The single place the rule is computed

**Decision:** two SQL views created by `000009`. No Go code and no other SQL text computes the rule.

```sql
CREATE VIEW category_container_setting AS
SELECT c.categoryId                                              AS categoryId,
       c.categoryIsContainer                                     AS explicitIsContainer,
       COALESCE(c.categoryIsContainer, t.topCategoryIsContainer, 0) AS effectiveIsContainer
FROM gear_category c
LEFT JOIN gear_top_category t ON t.topCategoryId = c.categoryTopCategoryId;

CREATE VIEW gear_container_capability AS
SELECT g.gearId                                AS gearId,
       COALESCE(s.effectiveIsContainer, 0)     AS isContainer
FROM gear g
LEFT JOIN category_container_setting s ON s.categoryId = g.gearCategoryId;
```

- The view joins on `gear.gearCategoryId` only, so the gear's `gearTopCategoryId` has no effect (AC-3). A missing sub-category, or a sub-category whose top category is missing, gives 0 (P-13).
- Consumers (all must use the view, nothing else):
  - Gear list, search and get (`models.GearListItem`, `models.FullGear`): join `LEFT JOIN gear_container_capability ON gear_container_capability.gearId = gear.gearId` and map `gear_is_container` to `gear_container_capability.isContainer`. Every gear row has a view row, so no extra COALESCE is needed.
  - Registration list and get, and container contents (`userGearColumns`, `userGearCatalogJoins`): `LEFT JOIN gear_container_capability cap ON cap.gearId = ugr.gearId` and report `COALESCE(cap.isContainer, 0)`. A registration whose gear row is missing gets no view row, so the COALESCE is needed there.
  - `container=true|false` on the gear list and the registration list: filter on exactly the reported expression (`… = 1` / `… = 0`). The **count query must use the same join and filter** as the item query (today both count queries join only `gear`).
  - Category get and list: join `category_container_setting` for `category_container` (mapped from `explicitIsContainer`: `NULL` → `"inherit"`, `1` → `"container"`, `0` → `"not_container"`) and `category_is_container` (from `effectiveIsContainer`).
  - The link check (D-8) and the database triggers (D-7).
- The gear insert response (`InsertGear` returns `models.Gear` through `GenericInsert` → `GenericGet` on table `gear`) must also report the view value. The INSERT column list must not contain the status column. How the model or handler does this is an implementation choice within those two rules.

Alternatives:

- *A Go constant holding the SQL expression*, pasted into each query. The trigger cannot use it, so the database guard would need its own copy: two places. Rejected.
- *A Go helper that computes status after the query.* The filters and counts still need SQL, which means a second implementation. Rejected.
- *A stored, maintained column on `gear`* (kept up to date by triggers on category, top-category and gear changes). This brings back the second source of truth BR-021 removes, adds three or four maintenance triggers, and goes stale on any missed path. Rejected.

SQLite flattens these simple views into the outer query. For a catalog of this size the cost is negligible. Revisit only if a measured gear-list latency problem points at the view.

### D-4 `gear_is_container` in the API

**Decision:** keep the JSON field `gear_is_container` in every gear and registration response as a **read-only derived** value (D-3). On gear create and update it is **ignored**, not 400.

- Reason for keeping it: clients and the `container` filter already use it, and AC-3 allows a read-only field. Removing it breaks clients for no gain.
- Reason for ignoring rather than 400: clients that edit by echoing a read response send the field back, so a 400 would break ordinary edits. Ignoring it already meets AC-3 ("no request … can give a gear item a different status"). BR-017's 400 rule (P-2) covers identity and privilege fields, and this field is neither.
- Consequence: an update whose only field is `gear_is_container` is now 400 `no updatable fields in request body` (today it changed the flag). Swagger marks the field read-only and says it is ignored on write.

Rejected: dropping the field (breaks clients; AC-3 does not require it). Rejected: 400 on send (breaks round-trip edits; no requirement asks for it).

### D-5 Fate of the old per-gear column (P-14 reversibility)

**Decision:** `000009` renames `gear.gearIsContainer` to `gear.gearIsContainerLegacy` (`ALTER TABLE gear RENAME COLUMN …`). The values stay. No API code reads or writes it, and new gear rows get its default 0.

- Renaming rather than leaving the name: any query left over that still names `gearIsContainer` fails with "no such column" in the tests, instead of quietly reporting the old flag (an AC-3 violation that would otherwise be easy to miss in string SQL).
- Rejected: dropping the column, which loses the data P-14's alternative needs. Rejected: copying it to an audit table (as 000005 did). A copy is a second artefact for the same data, and renaming keeps it in place at no cost.

**Where the P-14 switch lives:** if the owner chooses "seed from the old flags", the change is one statement. Before `000009` is released (merged to `main`), it is added at the end of `000009_category_container.up.sql`. After that, it goes into a new migration `000010` alone. The statement depends on a rule for sub-categories whose gear has mixed flags, which the requirements do not define (see open questions). The candidate is "explicit container where any of its gear was flagged":

```sql
UPDATE gear_category SET categoryIsContainer = 1
WHERE categoryId IN (SELECT gearCategoryId FROM gear WHERE gearIsContainerLegacy = 1);
```

`000009` records this statement as a comment, marked "not applied (P-14)". `sql/database.sql` and `sql/patch.sql` were not changed by `000009` and have since been deleted (review follow-up). They were pre-migration seed and patch scripts and still create `gearIsContainer`, which `000009` renames on the way up. A database loaded from `database.sql` therefore keeps its seeded flags as recoverable legacy values.

### D-6 Migration shape: new `000009`, not an edit of `000008`

**Decision:** a new migration `000009_category_container` (up and down). `000008` stays byte-identical.

- Reason: golang-migrate runs a version once. Databases already at version 8 (the test databases, and any developer database) would never see an edited `000008`. They would silently lack the new columns, views and triggers, while a fresh database would have them. That is schema drift with no error. That `000008` is unreleased does not help, because it has already been applied.
- `000008` contains no "any registration may contain" rule (it enforces only one direct container and no cycles), so no text in it needs to change. Nothing in `000009` relies on wave 7's "any registration may contain". The README note on `000008` stays as written. The code comment at the top of `pkg/api/userContainers.go` ("any registration may hold any other, whatever the catalog's gearIsContainer says") and the `InsertContainer` Swagger description must change, because they describe superseded behavior (P-11).

**Up, in order (one transaction, as golang-migrate's sqlite3 driver runs it):**

1. Add the two columns (D-1). Existing rows get `0` and `NULL` (P-14).
2. Rename `gear.gearIsContainer` to `gearIsContainerLegacy` (D-5).
3. Create the two views (D-3).
4. Create the two link triggers (D-7).
5. The P-14 alternative as a comment only.

Links, registrations and gear data are not changed (P-15).

**Replay:** SQLite has no `ADD COLUMN IF NOT EXISTS`, so running the up file again on a migrated database fails on the first statement. The transaction rolls back and nothing changes. This departs from the idempotent-replay style of 000005 and 000008, and the README note for `000009` must say so.

**Down:** drop both triggers, drop both views, `ALTER TABLE … DROP COLUMN` for both new columns (a CHECK on the column itself does not block DROP COLUMN), and rename `gearIsContainerLegacy` back to `gearIsContainer`. Settings are lost. Links and legacy flag values are kept. Up again gives P-14 start values again.

**README note:** `000009` adds category settings that all start as "not a container" (no gear is container-capable until an admin marks categories; *amended 2026-11-22: migration `000010` now seeds them from the old flags, so no admin step is needed*). It keeps the old per-gear flags as `gear.gearIsContainerLegacy`. It does not change links. It cannot be replayed. Its down migration discards the settings.

### D-7 Database guard for links into non-capable containers

**Decision:** yes. Two triggers repeat the rule, as 000008 does for cycles.

```sql
CREATE TRIGGER trg_container_links_capable_insert
BEFORE INSERT ON user_container_registration
WHEN NOT EXISTS (
    SELECT 1 FROM user_gear_registrations r
    JOIN gear_container_capability cap ON cap.gearId = r.gearId
    WHERE r.userGearRegistrationId = NEW.userContainerId AND cap.isContainer = 1)
BEGIN
    SELECT RAISE(ABORT, 'the container registration cannot hold items');
END;

CREATE TRIGGER trg_container_links_capable_update
BEFORE UPDATE OF userContainerId, userGearRegistrationId ON user_container_registration
WHEN NOT EXISTS (
    SELECT 1 FROM user_gear_registrations r
    JOIN gear_container_capability cap ON cap.gearId = r.gearId
    WHERE r.userGearRegistrationId = NEW.userContainerId AND cap.isContainer = 1)
BEGIN
    SELECT RAISE(ABORT, 'the container registration cannot hold items');
END;
```

- **Interaction with P-15:** the triggers fire only when a link row is inserted or re-pointed. Catalog changes, gear moves and registration deletes never fire them, so a link that already exists stays when its container loses capability, and it can still be listed and deleted. The update trigger treats re-pointing a link as new content, which is "cannot receive new contents" (BR-021 AC-6). The API never updates links.
- **Why add it:** it keeps BR-020 AC-1 true for a second connection or a future handler, at the cost of about 15 lines. It also follows the wave 7 rule that the database repeats what the API checks.
- The trigger message is a fixed string that the handler matches (D-9). SQLite does not fix the order in which several BEFORE triggers fire, so the database guards carry no precedence. Precedence is the API's job (D-9).
- The migration test must show that the triggers work through the view: a direct `INSERT` into a not-capable container fails, and into a capable one succeeds.

Rejected: an API check only. It is enough for the API path under immediate transactions, but it leaves the database as the only layer in the container rules without a repeat, and the cost of the repeat is small. Rejected: a CHECK or foreign key. Neither can express a lookup through two joins.

### D-8 Setting change vs concurrent link request (BR-021 AC-8)

**Decision:** no new mechanism. Use the existing serialization and make one placement rule binding.

- `InsertContainer` runs in `inTx` with `_txlock=immediate`, so it holds SQLite's single write lock from `BEGIN`. Every catalog write that can change capability (top-category setting, sub-category setting or top category, gear sub-category) is a single autocommit `UPDATE` (`catalogUpdate.apply`) and needs the same lock. Writers are therefore strictly ordered:
  - If the link transaction gets the lock first, it decides and commits under the old setting. The catalog `UPDATE` waits (busy timeout) and commits afterwards, and the link stays (P-15).
  - If the catalog `UPDATE` commits first, it was answered after its commit. The link transaction starts later, and its reads see the new setting, because no other writer can commit while it holds the lock.
- **Binding rule:** the capability check is read **inside** the link transaction, through `gear_container_capability`, after the ownership check. No capability value may be read before `BEGIN` or cached.
- **Binding rule:** a catalog write that changes a setting, a category's top category or a gear's sub-category must stay one statement, or run in one immediate transaction. It must not commit in steps that leave a mixed state.
- The trigger (D-7) covers any other connection. In WAL mode a write from a transaction whose read snapshot is stale fails with `SQLITE_BUSY_SNAPSHOT`, so the trigger always checks against the latest committed settings.

Rejected: application locks or version counters. They duplicate what SQLite's write lock already guarantees in a single-instance deployment.

### D-9 Status mapping and messages

**Decision:** `InsertContainer` decides in this order (BR-018 precedence, BR-020 AC-5, P-17):

| Order | Condition | Status | Body `error` |
|---|---|---|---|
| — | missing or invalid token | 401 | unchanged |
| — | missing `user_container_id` or `user_gear_registration_id`, invalid JSON | 400 | unchanged |
| 1 | container or content not owned by the caller, or unknown | 404 | `Registration not found` (unchanged) |
| 2 | container = content (after the ownership check of that one ID) | 400 | `a registration cannot be contained in itself` (unchanged) |
| 3 | container's gear is not container-capable (view value 0 or no row) | 409 | `this registration cannot hold items: its gear is not in a container category` |
| 4 | content already has a container (same or other) | 409 | unchanged wave 7 messages |
| 5 | container is inside the content | 409 | unchanged wave 7 message |
| — | database guards | 409 | a trigger abort whose message contains `the container registration cannot hold items` maps to the step 3 message; any other trigger abort maps to the cycle message; a unique violation maps to `registration is already in a container` (unchanged) |

- Only the container's capability is checked. Content may be any registration (BR-020 AC-1).
- The step 3 message names only the caller's own registration, so it reveals nothing about another user (BR-020 AC-5).
- Catalog statuses are unchanged: 403 for non-admin writes, 404 for an unknown ID, 400 for a body-ID conflict, an empty update or an invalid `category_container` (D-2).
- The test `AnyRegistrationCanContainAnyOther` is inverted to the D11 rule (as the `impl` task requires), and the wave 7 nesting fixtures use container-capable gear.

## Consequences

**Positive:** one definition of the rule, used by reads, filters, the link check and the database guard. The API keeps its existing field and filter. The setting fields fit the existing partial-update machinery with no special `null` handling. No new concurrency mechanism. P-14 stays a one-statement switch.

**Negative:** `000009` cannot be replayed and its down migration loses settings (acceptable: test data, recorded in the README). Every gear, registration and category query gains one view join, and both count queries must change with their item queries. Forgetting a count query is the most likely defect, and the filter tests catch it. A changed behavior: a gear update that sends only `gear_is_container` is now 400 instead of a change.

**Neutral:** the gear's `gearTopCategoryId` stays stored and reported, and it has no role in container status. Whether it must match the sub-category's top category stays out of scope (BR-021).

**Reversibility:** a down migration plus reverting the code restores wave 7 behavior. Legacy flags survive in both directions.

## Confirmation and reconsideration

Already checked (SQLite 3.46.1 CLI, scratch database): the column CHECKs accept `ALTER TABLE … ADD COLUMN`, a trigger can query the views, the views apply inheritance and explicit overrides, and the down steps (DROP VIEW, DROP COLUMN with a self-CHECK, RENAME COLUMN back) succeed. The driver's bundled SQLite (`mattn/go-sqlite3 v1.14.47`) must be confirmed by the migration test.

Confirmed when:

- The migration test on a v8 database with old flags set shows: the new columns hold their defaults, every gear reports `false`, `gearIsContainerLegacy` holds the old values, all links are kept, the triggers refuse a direct `INSERT` into a not-capable container, replay fails without change, and down then up restores the start values.
- `rg -n "gearIsContainer\b"` finds no reference in Go source outside comments, and `gearIsContainerLegacy` is used only in migrations.
- The `impl` and `impl-catalog` criteria pass, including the AC-8 race run many times and E2E-001.

Reconsider when:

- the service runs more than one writer instance or leaves SQLite (D-8 rests on SQLite's single write lock);
- the owner flips P-14 (apply D-5's statement after the mixed-flag rule is decided);
- a measured performance problem points at the views;
- BR-002 introduces unapproved gear that must not be container-capable (the view would then need the approval state).

## Open questions

- **OQ-1 (closed 2026-11-22 by P-18 and ADR-002 D-7: any flagged gear → explicit container; top categories stay not container):** when seeding from the old flags, what does a sub-category get when only some of its gear was flagged? The candidate in D-5 is "any flagged → explicit container". It also leaves top categories at "not container". Nothing needs this answer while P-14 stands.
- No contradiction between the requirements was found. BR-020 AC-8 ("a stored link whose container is no longer capable is not a violation") and BR-021 AC-6 agree with D-7's insert/update-only triggers.
