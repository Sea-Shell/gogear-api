---
type: Architecture Specification
title: ARS-001 Loadout loop schema, migrations and API contract
description: Exact database changes (migrations 000011 to 000014 and the cutover), the weight views, response shapes and every new or changed endpoint for the loadout planning and packing work (BR-023 to BR-037, OC-002), grouped by milestone A to E.
tags: [architecture, api, schema, migration, sqlite, loadouts, catalog, loadout-planning]
timestamp: 2026-11-24T00:00:00Z
---

# ARS-001 — Loadout loop schema, migrations and API contract

**Status:** Accepted (architect, 2026-11-22). **Milestone A implemented** (2026-11-22, branch `loadout-loop-support` at `2d236d2`): migration 000011 and every milestone A endpoint and response field of section 4, including the legacy `gear_id` insert path; Swagger regenerated. **Not implemented:** milestones B to E (migrations 000012 to 000014 and their endpoints) and the cutover (section 2.5), so BR-023 AC-1 and AC-7 are still pending. Realizes [ADR-003](/docs/architecture/decisions/adr-003-loadout-places-registration-items-and-computed-weight.md), [ADR-004](/docs/architecture/decisions/adr-004-expand-contract-release-of-the-loadout-item-contract.md) and [ADR-005](/docs/architecture/decisions/adr-005-gear-catalog-states-contributions-and-private-items.md) within [ARD-001](/docs/architecture/design/ard-001-loadout-planning-and-packing.md). **Amended 2026-11-22:** reconciled with [BR-037](/docs/requirements/requirements/br-037-private-items.md) (private items), the amended OC-002, BR-033 AC-8 and AC-9, and the updated frontend design (F8 to F10, N9, N11, N12): no replace for private items, contribution list without private items, own-code conflict response names the other registration. No open markers remain. **Amended after the critic review (Loom qa `279ad7b0`):** 000011 keeps the `loadout_items` ID sequence (F2), `detachedGearId` has no `ON DELETE` action (F3), legacy items have no snapshot and read their gear live (F4), the closet default parent is skipped when it lost capability (F5), new trigger `trg_registration_owner_fixed`, down-migration drop order, and the public-items filter in milestone A. Each migration merges to `main` together with the code that depends on it (ARD-001 merge rule).

## In brief

This document is what a coder implements. Each section says which milestone it belongs to. Names of tables and columns follow the existing camelCase style; JSON fields follow the existing snake_case style. Migration numbers assume the milestones merge in order A, B, D, E; the cutover takes the next free number when it lands, and if milestones merge in another order, numbers follow merge order with the same content.

## Satisfies

See ARD-001 for the full requirement map. This document fixes the contract each row there relies on.

## 1. Conventions (all milestones)

- **Paths** are under `/api/v1`, behind the JWT middleware unless marked *public*. Path style follows today's routes (`/loadout/:loadout/...`, verbs in the path).
- **Refusal order** (BR-018, BR-024 AC-11): unknown, foreign or wrong-parent target → **404** (identical body for all three); then malformed or invalid request → **400**; then a state conflict → **409**. A refused request changes nothing.
- **Body IDs** that differ from the path or caller → 400 after the 404 check (P-2, as today).
- **Absent vs `null`:** for the new nullable fields listed as *clearable* below, an absent field keeps the stored value and an explicit `null` clears it (the `max_container_weight` precedent, wave 3). All other fields keep today's rule: `null` is treated as absent. The implementation detects presence with `json.RawMessage` or an equivalent.
- **Transactions:** every write runs in one transaction (`BEGIN IMMEDIATE`, already forced by `utils.SQLiteDSN`), with all checks inside it. Concurrent writers queue, which realizes BR-024 AC-10, BR-031 AC-7, BR-033 AC-2 and BR-034 AC-6. `GET /loadout/:loadout/full` reads its three parts inside one transaction so they describe one state.
- **Database messages:** triggers raise fixed messages (listed per trigger). The API checks every rule before writing, so a trigger message reaching a handler means a missed check; the handler maps it to the status of the rule and logs it, as ADR-001 D-9 does.
- **Swagger** is regenerated in every milestone (BR-023 AC-8).

## 2. Schema

### 2.1 Migration `000011_loadout_places_and_registration_items` (milestone A)

Runs in golang-migrate's transaction with foreign keys on. Statements in this order:

1. **`loadout_places`** (new):

   ```sql
   CREATE TABLE loadout_places (
     placeId        INTEGER PRIMARY KEY AUTOINCREMENT,
     loadoutId      INTEGER NOT NULL REFERENCES loadouts(loadoutId) ON DELETE CASCADE,
     placeKind      TEXT    NOT NULL CHECK (placeKind IN ('worn','last_minute','bag')),
     registrationId INTEGER REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE CASCADE,
     parentPlaceId  INTEGER REFERENCES loadout_places(placeId) ON DELETE SET NULL,
     position       INTEGER,
     closed         INTEGER NOT NULL DEFAULT 0 CHECK (closed IN (0,1)),
     CHECK ((placeKind = 'bag') = (registrationId IS NOT NULL)),
     CHECK (placeKind = 'bag' OR (parentPlaceId IS NULL AND position IS NULL)),
     CHECK (placeKind <> 'bag' OR position IS NOT NULL)
   );
   CREATE UNIQUE INDEX idx_loadout_places_worn        ON loadout_places(loadoutId) WHERE placeKind = 'worn';
   CREATE UNIQUE INDEX idx_loadout_places_last_minute ON loadout_places(loadoutId) WHERE placeKind = 'last_minute';
   CREATE UNIQUE INDEX idx_loadout_places_bag         ON loadout_places(loadoutId, registrationId) WHERE placeKind = 'bag';
   CREATE INDEX idx_loadout_places_registration ON loadout_places(registrationId);
   CREATE INDEX idx_loadout_places_parent       ON loadout_places(parentPlaceId);
   ```

2. **Worn for every loadout:** `INSERT INTO loadout_places(loadoutId, placeKind) SELECT loadoutId, 'worn' FROM loadouts;` and trigger `trg_loadouts_create_worn AFTER INSERT ON loadouts` inserting the Worn row for `NEW.loadoutId`. Every create path (insert, copy) gets Worn from it.

3. **Place triggers** (`BEFORE INSERT` and `BEFORE UPDATE OF` the named columns):
   - `trg_loadout_places_owner` (`registrationId`, `loadoutId`): when `registrationId` is not null, the registration's `userId` must equal the loadout's `userId`. Message: `bag registration must belong to the loadout owner`.
   - `trg_loadout_places_parent` (`parentPlaceId`, `loadoutId`): when not null, the parent must be a row with `placeKind = 'bag'` and the same `loadoutId`, and not the row itself. Message: `parent must be another bag of the same loadout`.
   - `trg_loadout_places_no_cycle` (`parentPlaceId`, update only; on insert a new row cannot be anyone's parent): walking `parentPlaceId` up from `NEW.parentPlaceId` (recursive CTE with `UNION`, as in 000008) must not reach `NEW.placeId`. Message: `a bag cannot be inside one of its own contents`.
   - `trg_registration_owner_fixed` (`BEFORE UPDATE OF userId ON user_gear_registrations`, when `NEW.userId IS NOT OLD.userId`): refuses the update. Message: `a registration cannot change owner`. No API path changes `userId` (the registration update writes only its own fields), but without this guard a direct update would leave the owner triggers' rule (I-4) broken for existing bags and items, because those triggers fire only on writes to places and items. It costs one trigger.

4. **Rebuild `loadout_items`.** `loadout_items` is referenced by no table, so it is rebuilt safely with foreign keys on:

   ```sql
   CREATE TABLE loadout_items_v11 (
     loadoutItemId        INTEGER PRIMARY KEY AUTOINCREMENT,
     loadoutId            INTEGER NOT NULL REFERENCES loadouts(loadoutId) ON DELETE CASCADE,
     registrationId       INTEGER REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE SET NULL,
     placeId              INTEGER REFERENCES loadout_places(placeId) ON DELETE SET NULL,
     quantity             INTEGER NOT NULL DEFAULT 1 CHECK (typeof(quantity) = 'integer' AND quantity > 0),
     notes                TEXT    NOT NULL DEFAULT '',
     ticked               INTEGER NOT NULL DEFAULT 0 CHECK (ticked IN (0,1)),
     detachedReason       TEXT    CHECK (detachedReason IN ('legacy','registration_deleted')),
     detachedGearId       INTEGER REFERENCES gear(gearId),   -- no ON DELETE action: see "Gear referenced by detached items" below
     detachedGearName     TEXT,
     detachedUnitWeight   INTEGER,
     detachedWeightSource TEXT    CHECK (detachedWeightSource IN ('weighed','guess','catalog')),
     CHECK (registrationId IS NOT NULL OR detachedReason IS NOT NULL)
   );
   ```

   The last CHECK is one-directional on purpose: the delete trigger of step 6 sets `detachedReason` while `registrationId` is still set, and the foreign-key action clears `registrationId` afterwards. A row with a live `registrationId` is a normal item whatever its detached columns say.

   Copy every old row whose loadout exists, keeping `loadoutItemId`, `loadoutId`, `quantity`, `notes`:
   - `registrationId` = the lowest `userGearRegistrationId` with the same `gearId` and the loadout's `userId` (*binding rule*, used again by the legacy insert and the cutover);
   - when no registration matches and the gear exists: `detachedReason = 'legacy'`, `detachedGearId = gearId`; `detachedGearName`, `detachedUnitWeight` and `detachedWeightSource` stay `NULL`. A legacy item's name, maker, category and weight are always read **live** from `detachedGearId` (ADR-004 D-2), like today's items;
   - when no registration matches and the gear does not exist: the row is not copied (orphan test data, BR-023 AC-7 allows removal).

   **Keep the ID sequence (BR-025 AC-7).** Rebuilding resets `AUTOINCREMENT`, so IDs of items deleted before the migration would be issued again, and a stale or offline request for an old ID could hit a new item. Before the drop, capture the old value: `CREATE TEMP TABLE m11_seq AS SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'loadout_items'), 0) AS seq;`. After the rename, set the new table's sequence to the larger of that value and the highest copied ID: delete any `sqlite_sequence` row named `loadout_items` and insert `('loadout_items', MAX(m11_seq.seq, COALESCE(MAX(loadoutItemId), 0)))`; then `DROP TABLE m11_seq`. **Required test:** seed items with IDs up to 50, delete the last 43, migrate; the next inserted item gets an ID above 50, and a tick or update for a deleted old ID is 404.

   Then `DROP TABLE loadout_items; ALTER TABLE loadout_items_v11 RENAME TO loadout_items;` (the sequence step above runs around these two statements) and indexes `idx_loadout_items_loadout(loadoutId)`, `idx_loadout_items_registration(registrationId)`, `idx_loadout_items_place(placeId)`, `idx_loadout_items_detached_gear(detachedGearId)`. The 000005 quantity triggers are dropped with the old table; the CHECK replaces them.

   **Gear referenced by detached items.** `detachedGearId` has no `ON DELETE` action, so deleting gear that a legacy or registration-deleted item still references is refused by the foreign key. The admin catalog delete already maps that refusal to **409** (wave 6 policy, `catalog.go`), as it does today for gear used by loadout items. This keeps a legacy item's live name and weight, and its `gear_id` for the deployed frontend. Snapshot-on-delete was rejected: it needs a second trigger and would hide the fact that the gear is still in use. Merge and replace repoint `detachedGearId` before deleting the old gear (ADR-005 D-4), and account deletion removes the user's loadouts before their own gear, so neither hits the refusal.

5. **Item triggers** (`BEFORE INSERT` and `BEFORE UPDATE OF` the named columns):
   - `trg_loadout_items_owner` (`registrationId`, `loadoutId`): a non-null registration belongs to the loadout's owner. Message: `item registration must belong to the loadout owner`.
   - `trg_loadout_items_place` (`placeId`, `loadoutId`): a non-null place has the same `loadoutId`. Message: `item place must belong to the same loadout`.

6. **Registration delete snapshot:** `trg_registration_delete_detach BEFORE DELETE ON user_gear_registrations` updates every `loadout_items` row with `registrationId = OLD.userGearRegistrationId`: `detachedReason = 'registration_deleted'`, `detachedGearId = OLD.gearId` if that gear row exists, else `NULL` (registrations stored before foreign keys were enforced may point at missing gear), `detachedGearName` from `gear` (`'Unknown gear'` when it is missing), `detachedUnitWeight` and `detachedWeightSource` from `registration_effective_weight` (`NULL` source when it is `none`). The foreign-key actions then set the items' `registrationId` to `NULL`, delete the registration's bag rows, and through them set contained items' `placeId` and child bags' `parentPlaceId` to `NULL` (BR-030 AC-1, AC-3, AC-4).

7. **Views** (section 2.6), then `ALTER TABLE loadouts DROP COLUMN totalWeight;`.

**Down migration:** recreates `loadouts.totalWeight` (recomputed from catalog weights), rebuilds `loadout_items` with `gearId NOT NULL` (from the registration's gear or `detachedGearId`; rows with neither are dropped) and the 000005 quantity triggers, and drops the places table, triggers and views. **Order:** first drop every trigger and view that names `loadout_items` or `loadout_places` (`trg_registration_delete_detach`, `trg_registration_owner_fixed`, `trg_loadouts_create_worn`, the item and place triggers, the three views), then rebuild `loadout_items` (carrying `sqlite_sequence` as in step 4), then drop `loadout_places`; otherwise the schema check during `RENAME` can fail on objects that refer to the old table. Lossy: places, ticks and detached snapshots are discarded. Not on the release path.

### 2.2 Migration `000012_weights_roles_stage` (milestone B)

`ALTER TABLE … ADD COLUMN` only (SQLite 3.53.2 checks new CHECK and NOT NULL constraints against existing rows):

| Table | Column | Definition |
|---|---|---|
| `user_gear_registrations` | `measuredWeight` | `INTEGER CHECK (measuredWeight IS NULL OR (typeof(measuredWeight) = 'integer' AND measuredWeight >= 1))` |
| `user_gear_registrations` | `measuredWeightSource` | `TEXT CHECK ((measuredWeight IS NULL AND measuredWeightSource IS NULL) OR (measuredWeight IS NOT NULL AND measuredWeightSource IS NOT NULL AND measuredWeightSource IN ('weighed','guess')))` |
| `user_gear_registrations` | `closetCount` | `INTEGER NOT NULL DEFAULT 1 CHECK (typeof(closetCount) = 'integer' AND closetCount >= 1)` (BR-036 AC-1: existing rows get 1) |
| `loadout_items` | `consumable` | `INTEGER NOT NULL DEFAULT 0 CHECK (consumable IN (0,1))` |
| `loadouts` | `stage` | `TEXT NOT NULL DEFAULT 'planning' CHECK (stage IN ('planning','packing','done'))` |
| `loadouts` | `doneAt` | `TEXT` (UTC timestamp, same format as `updatedAt`) |
| `loadouts` | `startDate` | `TEXT CHECK (startDate IS NULL OR date(startDate) IS startDate)` |
| `loadouts` | `endDate` | `TEXT CHECK (endDate IS NULL OR (date(endDate) IS endDate AND (startDate IS NULL OR endDate >= startDate)))` |
| `loadouts` | `weightGoal` | `INTEGER CHECK (weightGoal IS NULL OR (typeof(weightGoal) = 'integer' AND weightGoal >= 1))` |

The first draft of the `measuredWeightSource` CHECK (`src IN (...) AND weight IS NOT NULL OR src IS NULL AND weight IS NULL`) evaluates to NULL, which a CHECK accepts, when a weight has a NULL source, so it admitted a weight without a source (BR-026 AC-1 refuses it). The table shows the corrected form, which tests `IS NOT NULL` explicitly; migration test `TestMigration12ColumnRules` fails on the first draft. *(Amended 2026-11-24, found by coder B1.)*

Then replace `registration_effective_weight` with the measured-weight version (section 2.6) by `DROP VIEW` and `CREATE VIEW` in the same migration. The migration test must confirm that `loadout_item_weight`, `loadout_on_back_weight` and the delete trigger still work after the replacement. They do, on a version 11 database with data: SQLite resolves view names when a statement runs, so the dependent views and the trigger are not recreated (`TestMigration12KeepsExistingDataAndDependentViews`). The down migration drops the new view and recreates the milestone A one before it drops the columns, because `DROP COLUMN` refuses a column a view still names, and drops `endDate` before `startDate` and `measuredWeightSource` before `measuredWeight` because their CHECKs read them.

### 2.3 Migration `000013_gear_catalog_states` (milestone D)

| Table | Column | Definition |
|---|---|---|
| `gear` | `gearCreatorUserId` | `INTEGER REFERENCES users(userId) ON DELETE SET NULL` |
| `gear` | `gearCatalogState` | `TEXT NOT NULL DEFAULT 'approved' CHECK (gearCatalogState IN ('approved','private','waiting','not_approved') AND (gearCatalogState = 'approved' OR gearCreatorUserId IS NOT NULL))` |
| `gear` | `gearStateReason` | `TEXT` (decline reason; cleared on resubmit) |
| `gear` | `gearStateChangedAt` | `TEXT` |
| `gear` | `gearCreatedAt` | `TEXT` |
| `gear` | `gearWeightSource` | `TEXT CHECK (gearWeightSource IN ('maker','weighed'))` (as given by the creator) |
| `manufacture` | `manufactureCreatorUserId` | `INTEGER REFERENCES users(userId) ON DELETE SET NULL` |
| `manufacture` | `manufactureState` | `TEXT NOT NULL DEFAULT 'approved' CHECK (manufactureState IN ('approved','pending') AND (manufactureState = 'approved' OR manufactureCreatorUserId IS NOT NULL))` |

New table `gear_contribution_merges(mergeId INTEGER PRIMARY KEY AUTOINCREMENT, contributorUserId INTEGER NOT NULL REFERENCES users(userId) ON DELETE CASCADE, contributedGearName TEXT NOT NULL, mergedIntoGearId INTEGER REFERENCES gear(gearId) ON DELETE SET NULL, mergedIntoGearName TEXT NOT NULL, mergedAt TEXT NOT NULL)`.

Indexes: `gear(gearCatalogState)`, `gear(gearCreatorUserId)`, `gear_contribution_merges(contributorUserId)`.

Trigger `trg_registration_gear_visible` (`BEFORE INSERT`, `BEFORE UPDATE OF gearId, userId` on `user_gear_registrations`): the gear must be `approved` or have `gearCreatorUserId = NEW.userId`. Message: `gear is not visible to the registration owner`.

### 2.4 Migration `000014_codes` (milestone E)

- `gear.gearProductCode TEXT CHECK (gearProductCode IS NULL OR gearProductCode <> '')`, index `idx_gear_product_code ON gear(gearProductCode) WHERE gearProductCode IS NOT NULL` (not unique, BR-035 AC-1).
- `user_gear_registrations.ownCode TEXT CHECK (ownCode IS NULL OR ownCode <> '')`, `CREATE UNIQUE INDEX idx_registration_own_code ON user_gear_registrations(userId, ownCode) WHERE ownCode IS NOT NULL` (BR-035 AC-2, also under concurrency; a unique violation is 409).
- The API trims leading and trailing whitespace (Go `strings.TrimSpace`) and stores a blank code as `NULL` (BR-035 AC-3). No other normalization.

### 2.5 Cutover migration `0000NN_bind_or_remove_legacy_items` (ADR-004 D-3)

For each item with `detachedReason = 'legacy'`: apply the binding rule with `detachedGearId`; when it binds, set `registrationId` and clear all `detached*` columns; delete the remaining legacy rows. No schema change (`'legacy'` stays allowed by the CHECK but is no longer produced). Down: no-op.

### 2.6 Views (the only definitions of these values)

```sql
-- milestone A version (catalog only); milestone B replaces it
CREATE VIEW registration_effective_weight AS
SELECT r.userGearRegistrationId AS registrationId,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END            AS catalogWeight,
       CASE WHEN g.gearWeight > 0 THEN g.gearWeight END            AS unitWeight,
       CASE WHEN g.gearWeight > 0 THEN 'catalog' ELSE 'none' END   AS weightSource
FROM user_gear_registrations r LEFT JOIN gear g ON g.gearId = r.gearId;

-- milestone B version
--   unitWeight   = COALESCE(r.measuredWeight, catalog weight > 0)
--   weightSource = r.measuredWeightSource if set, else 'catalog' if catalog weight > 0, else 'none'

CREATE VIEW loadout_item_weight AS
SELECT li.loadoutItemId, li.loadoutId,
       CASE WHEN li.registrationId IS NOT NULL       THEN w.unitWeight
            WHEN li.detachedReason = 'legacy'        THEN (SELECT CASE WHEN g.gearWeight > 0 THEN g.gearWeight END FROM gear g WHERE g.gearId = li.detachedGearId)
            ELSE li.detachedUnitWeight END                               AS unitWeight,
       CASE WHEN li.registrationId IS NOT NULL       THEN w.weightSource
            WHEN li.detachedReason = 'legacy'        THEN CASE WHEN (SELECT g.gearWeight FROM gear g WHERE g.gearId = li.detachedGearId) > 0 THEN 'catalog' ELSE 'none' END
            ELSE COALESCE(li.detachedWeightSource, 'none') END           AS weightSource
FROM loadout_items li
LEFT JOIN registration_effective_weight w ON w.registrationId = li.registrationId;

CREATE VIEW loadout_on_back_weight AS          -- BR-029 AC-3
SELECT l.loadoutId,
       COALESCE((SELECT SUM(li.quantity * COALESCE(iw.unitWeight, 0))
                 FROM loadout_items li
                 JOIN loadout_item_weight iw ON iw.loadoutItemId = li.loadoutItemId
                 LEFT JOIN loadout_places p  ON p.placeId = li.placeId
                 WHERE li.loadoutId = l.loadoutId AND (p.placeKind IS NULL OR p.placeKind <> 'worn')), 0)
     + COALESCE((SELECT SUM(COALESCE(w.unitWeight, 0))
                 FROM loadout_places p
                 JOIN registration_effective_weight w ON w.registrationId = p.registrationId
                 WHERE p.loadoutId = l.loadoutId AND p.placeKind = 'bag'), 0) AS totalWeight
FROM loadouts l;
```

## 3. Response shapes

Fields added by a later milestone are marked (B), (D), (E). Every field listed exists from its milestone on. `weight_source` is one of `"weighed"`, `"guess"`, `"catalog"`, `"none"`.

**`Loadout`** (list, get, insert and update responses, `full.loadout`): today's fields (`loadout_id`, `user_id`, `loadout_name`, `loadout_description`, `loadout_is_public`, `loadout_slug`, `total_weight`, `created_at`, `updated_at`) plus:
- `total_weight`: on-your-back grams from `loadout_on_back_weight` (A);
- `item_count`, `ticked_count`, `bag_count` (A). `item_count` and `ticked_count` count **item rows**, not quantities: a spork with quantity 3 is 1 item. This matches the frontend's Pack header "N of M items in", where each item row (card) has one tick, and BR-025, where a tick belongs to an item, not to each unit. Detached items (legacy or registration-deleted) are rows of the loadout and are counted. `bag_count` counts bag places only (not Worn or Last minute). *(Amended 2026-11-22, gap raised by coder A4, Loom `d6b0bf93`.)* `closed_places`: `[{place_id, place_kind, name}]` (A; `name` is the bag's gear name, `"Worn"` or `"Last minute"`);
- `stage`, `done_at` (`null` unless `stage = "done"`), `start_date`, `end_date` (`YYYY-MM-DD` or `null`), `weight_goal` (grams or `null`) (B).

This makes `/loadout/list` the home summary of BR-029 AC-4.

**`LoadoutItem`** (item list, `full.items`, item writes):
- today's `loadout_item_id`, `loadout_id`, `gear_id` (read-only: the registration's gear, else `detachedGearId`, may be `null`), `quantity`, `notes`;
- `usergear_registration_id` (`null` when detached), `registration_exists` (bool), `detached_reason` (`null`, `"legacy"`, `"registration_deleted"`) (A);
- `gear_name`, `manufacture_name`, `category_name`, `top_category_name`: for registration-backed and **legacy** items, live from the current gear (registration's gear, or `detachedGearId`); for registration-deleted items `gear_name` is the snapshot and the others are `null` (A);
- `place_id` (`null` = no place), `ticked` (A);
- `catalog_weight` (`null` = none), `unit_weight`, `weight_source` (A; from `loadout_item_weight`). `catalog_weight` follows the same source rule as the names: live from the current gear for registration-backed and legacy items, and **`null` for registration-deleted items**. A deleted-registration item reports only its snapshot (`unit_weight` and `weight_source` as they were at deletion, BR-030 AC-1); a live catalog value next to a frozen effective weight would invite clients to recompute a different weight. *(Amended 2026-11-22, spec gap raised by coder A2, Loom `3d2ff0d3`.)*
- `measured_weight`, `measured_weight_source`, `closet_count`, `consumable` (B). The first three are the registration's own values, read live; they are `null` for an item without a registration (legacy or registration-deleted), which reports only the effective `unit_weight` and `weight_source`, like `catalog_weight` (same reasoning: a live value next to a frozen one invites a recomputed weight). `closet_count` is therefore `null` for those items too. *(Clarified 2026-11-24, decision of coder B1; the spec was silent.)*
- `gear_catalog_state`, `gear_state_reason` (D; reason only when `not_approved`).

**`LoadoutPlace`** (`full.places`, place writes): `place_id`, `loadout_id`, `place_kind` (`"worn"`, `"last_minute"`, `"bag"`), `closed`; for bags also `usergear_registration_id`, `gear_id`, `gear_name`, `gear_is_container`, `max_container_weight`, `catalog_weight`, `unit_weight`, `weight_source`, `parent_place_id`, `position` (A), `measured_weight`, `measured_weight_source` (B), `gear_catalog_state` (D). Non-bag places have these as `null`.

**`LoadoutFull`**: `{ "loadout": Loadout, "places": [LoadoutPlace], "items": [LoadoutItem] }`, places ordered Worn, Last minute, then bags by `position, place_id`; items by `loadout_item_id`.

**`UserGear`** (registration list, get, writes): today's fields plus `gear_weight` reported as `null` when none (A), `measured_weight`, `measured_weight_source`, `unit_weight`, `weight_source`, `closet_count` (B), `gear_catalog_state`, `gear_state_reason` (D), `own_code`, `gear_product_code` (E).

**`GearListItem`** (catalog list and search): today's fields plus `gear_weight` (`null` when none) (A, BR-029 AC-5), `gear_catalog_state` (D; always `"approved"` for other users' gear), `gear_product_code` (E). `Gear` and `FullGear` single reads: `gear_weight` becomes `null` when none (A), plus the same D and E fields.

**Public items** (`GET /public/loadout/:slug/items`): today's `loadout_item_id`, `loadout_id`, `gear_id`, `quantity`, `notes`, plus `gear_name` (A). An item is listed only if it has a current gear row: the registration's gear for a registration-backed item, or `detachedGearId` for a detached item. A detached item with `detachedGearId = NULL` (only possible after deleting a registration whose gear was already missing) is not listed. In milestone A every gear row is shared catalog gear, so this is the whole filter. From D the filter also requires `gearCatalogState = 'approved'`, so private and unapproved gear is left out (BR-016 AC-6, OC-002 G-5, BR-037 AC-6). The public loadout's `total_weight` follows `loadout_on_back_weight` and so still counts hidden items.

## 4. Endpoints

Status codes not listed follow section 1. "Owner" means the caller owns the loadout or registration in the path; anything else is 404.

### Milestone A — items, places, progress, weights in reads

| Method and path | Change | Request | Success | Specific refusals |
|---|---|---|---|---|
| `GET /loadout/list` | changed | — | 200 `[Loadout]` | — |
| `GET /loadout/:loadout/get` | changed | — | 200 `Loadout` | 404 |
| `GET /loadout/:loadout/full` | **new** | — | 200 `LoadoutFull` | 404 |
| `PUT /loadout/insert` | changed (Worn created by trigger) | as today | 201 `Loadout` | as today |
| `POST /loadout/:loadout/import` | **removed** (ADR-004 D-1) | — | — | route no longer exists |
| `PUT /loadout/:loadout/item/insert` | changed | `usergear_registration_id` (required, unless legacy), `quantity` (≥1, default 1), `notes`, `place_id` (optional, default no place), `loadout_id` (optional, must match); legacy until cutover: `gear_id` instead of `usergear_registration_id` (ADR-004 D-2) | 201 `LoadoutItem` | 404 registration not owned or unknown; 404 `place_id` not a place of this loadout; 400 both `gear_id` and `usergear_registration_id`; 400 quantity; 409 `place_id` is a bag whose gear is not container-capable |
| `GET /loadout/:loadout/item/list` | changed | — | 200 `[LoadoutItem]` | 404 |
| `POST /loadout/:loadout/item/:item/update` | changed | `quantity`, `notes`, `place_id` (*clearable*: `null` = no place) | 200 `LoadoutItem` | 404 item or place; 400 quantity or no updatable field; 409 into a non-capable bag. Never changes `ticked` (BR-025 AC-4) |
| `DELETE /loadout/:loadout/item/:item/delete` | as today | — | 200 | 404 |
| `POST /loadout/:loadout/item/:item/tick` | **new** | `{"ticked": bool}` (required) | 200 `{loadout_item_id, ticked}` | 404 item or loadout gone (BR-025 AC-7); 400 missing or non-bool |
| `PUT /loadout/:loadout/place/insert` | **new** | `{"place_kind": "bag", "usergear_registration_id": n, "parent_place_id": n \| null (optional)}` or `{"place_kind": "last_minute"}` | 201 `LoadoutPlace` | 404 registration or parent; 400 `place_kind` not `bag`/`last_minute`, parent not a bag (Worn or Last minute: BR-024 AC-6); 409 already a bag of this loadout, gear not container-capable (message says the gear cannot hold items), second Last minute, parent bag not container-capable |
| `POST /loadout/:loadout/place/:place/update` | **new** (bags only) | `{"parent_place_id": n \| null}` (*clearable*: `null` = top level) | 200 `LoadoutPlace` | 404 place or parent; 400 not a bag, `parent_place_id` absent (it is the only updatable field, so a body without it has nothing to update, as for every other update with no fields), parent is the bag itself, parent not a bag; 409 parent is inside this bag (cycle), parent bag not container-capable. Setting the parent the bag already has is a 200 no-op, even if that parent has since lost capability: nothing new goes into it (BR-024 AC-6) |
| `POST /loadout/:loadout/place/order` | **new** | `{"place_ids": [n, …]}`: every bag of the loadout exactly once | 200 `[LoadoutPlace]` (bags in the new order) | 404 an ID that is not a place of this loadout; 400 a non-bag, a duplicate, or a bag missing |
| `POST /loadout/:loadout/place/:place/close` | **new** | `{"closed": bool}` (required) | 200 `{place_id, closed}` | 404 place; 400 missing or non-bool; `:place` = `0` or non-numeric is 400 ("no place" cannot be closed, BR-025 AC-3). Every place row can be closed and opened: bags, **Worn and Last minute** (BR-025 AC-3 "closable place", P-23) |
| `DELETE /loadout/:loadout/place/:place/delete` | **new** | — | 200 | 404; 400 the place is Worn |
| `GET /gear/list`, `GET /gear/search` | changed | — | `GearListItem` with `gear_weight` | — |
| `GET /usergear/:user/list`, `GET /usergear/registration/:usergear/get` | changed | — | `UserGear` | as today |
| `DELETE /usergear/registration/:usergear/delete` | behavior | — | 200 | as today; loadout effects by the delete trigger and FK actions (BR-030) |
| `GET /public/loadout/:slug`, `GET /public/loadout/:slug/items` | changed | — | `total_weight` per view; items with `gear_name` | as today |

Rules inside milestone A operations:

- **Adding a bag without `parent_place_id`** (field absent): if the registration's closet direct container (`user_container_registration` where it is the content) is a bag of this loadout **and that bag's gear is container-capable now**, that bag is the parent; otherwise the new bag is top level. A default parent that has lost capability is therefore skipped, never refused: BR-024 AC-6 forbids putting anything new into it, and refusing an add because of a parent the caller did not name would block for no reason the caller can see. An explicit `parent_place_id` naming such a bag is 409 (AC-6). An explicit `null` means top level. Nothing writes closet links (BR-024 AC-4).
- **New bag position:** `MAX(position) + 1` within the loadout (1 if none).
- **Container capability** (BR-024 AC-2, AC-6): read from `gear_container_capability` for the bag's registration inside the write transaction, on add bag, item insert and update into a bag, and bag parent change. Not checked by triggers and not checked by copy (ADR-003 limits).
- **Capacity and goal** are never checked (BR-024 AC-7).
- **Closing and ticking** never touch any other row; moving or editing an item never touches `ticked` or any `closed` (BR-025 AC-3, AC-4).

### Milestone B — measured weight, count, role, stage, dates, goal

| Method and path | Change | Request additions | Refusals |
|---|---|---|---|
| `PUT /usergear/insert` | changed | `closet_count` (optional, default 1); `measured_weight` and `measured_weight_source` (optional, both or neither) | 400 count not an integer ≥ 1; 400 measured rules (BR-026 AC-1) |
| `POST /usergear/registration/:usergear/update` | changed | `measured_weight` and `measured_weight_source` (*clearable* together: both `null` removes the measured weight; one without the other is 400); `closet_count` | 404; 400 as above. Use above the count is never refused (BR-036 AC-3) |
| `PUT /loadout/:loadout/item/insert`, `…/item/:item/update` | changed | `consumable` (bool, default false) | 400 non-bool |
| `PUT /loadout/insert` | changed | `start_date`, `end_date`, `weight_goal` (optional); `stage` is not accepted on insert (always `planning`) | 400 dates not `YYYY-MM-DD`, end before start, goal not an integer ≥ 1 |
| `POST /loadout/:loadout/update` | changed | `stage` (`planning`, `packing`, `done`); `start_date`, `end_date`, `weight_goal` (*clearable*) | 400 as above or unknown stage |

Stage rule: setting `stage` to `done` when it was not `done` sets `doneAt` to now; setting it to `done` when already `done` keeps `doneAt`; setting another stage clears `doneAt`. Nothing else ever changes `stage` (BR-028 AC-2). The end-before-start check uses the values after the update.

### Milestone C — copy

`POST /loadout/:loadout/copy` (**new**). Request: `{"loadout_name": string, "copy_places": bool, "copy_notes_and_goal": bool}`. All three are required; an absent option is 400 (BR-031 AC-8: the documented treatment of an unstated option). Name rules as for loadout insert.

Success: 201 `{"loadout": Loadout, "left_out": [{"loadout_item_id": n, "gear_name": string}]}`. Refusals: 404 source not owned (including another user's public loadout); 400 name or options.

One immediate transaction:
1. insert the new loadout (owner = caller, `stage = planning`, private, no slug, no dates; description and goal only with `copy_notes_and_goal`); the trigger creates its Worn;
2. with `copy_places`: create Last minute if the source has one; create every source bag with the same registration and `position`, then set parents through an old-to-new ID map; capability is not checked;
3. for each source item with a live `registrationId`: insert with the same registration, `quantity`, `consumable`; `notes` only with `copy_notes_and_goal`; `place_id` mapped (Worn → new Worn; Last minute and bags → new rows when `copy_places`, else `null`); `ticked = 0`;
4. every source item without a live registration (detached, including legacy) is left out and named by `gear_name`;
5. all `closed` are 0. The source is only read.

### Milestone D — new gear, private items, contributions, review

| Method and path | Who | Request | Success | Refusals |
|---|---|---|---|---|
| `PUT /usergear/new-gear` | any user | `gear_name` (required), `manufacture_id` or `new_manufacture_name` (exactly one), `gear_category_id` (required; top category taken from it), `gear_weight` (≥1, optional), `gear_weight_source` (`maker` or `weighed`, required with a weight), `gear_size_definition`, `gear_product_code` (E), `catalog_state` (`"catalog"` = contribution, or `"private"` = private item, BR-037 AC-1; required, no default, so the client's default (frontend DD-9) is always explicit), `closet_count`, `max_container_weight`, `own_code` (E) | 201 `{"gear": Gear, "registration": UserGear}` | 400 field rules (BR-032 AC-1, BR-037 AC-1); 404 unknown or invisible maker or category |
| `GET /contribution/list` | any user | — | 200 `[{gear_id \| null, gear_name, gear_catalog_state \| "merged", gear_state_reason, state_changed_at, merged_into_gear_id, merged_into_gear_name}]`: the caller's contributions only (`waiting`, `not_approved`, `approved`, merged); private items are not listed (BR-002 AC-6) | — |
| `POST /contribution/:gear/update` | creator | gear fields as in `new-gear` (partial, without `catalog_state`) | 200 `Gear` | 404 not the caller's own `private`, `waiting` or `not_approved` gear; 409 approved. Works for private items too (BR-037 AC-4) |
| `POST /contribution/:gear/submit` | creator | — | 200 `Gear` (`waiting`, reason cleared) | 404; 409 already `waiting` or `approved`. From `private` this is BR-037 AC-5 "send to the catalog": registrations and loadouts are not written, the private maker stays the same row |
| `POST /contribution/:gear/replace` | creator | `{"gear_id": G}` | 200 `{"replaced_by": Gear, "registrations": n}` | 404 source; 404 G unknown or not approved; 400 G is the source; 409 source `approved` or `private` (BR-037 has no replace) |
| `GET /contribution/review/list` | admin | — | 200 `[{gear_id, gear_name, manufacture_name, manufacture_is_new, category_name, gear_weight, gear_weight_source, gear_size_definition, gear_product_code, contributor_username, submitted_at}]` (only `waiting`) | 401; 403 |
| `POST /contribution/review/:gear/approve` | admin | — | 200 `Gear` | 404 unknown or not a contribution; 409 not `waiting` |
| `POST /contribution/review/:gear/merge` | admin | `{"gear_id": G}` | 200 `{"merged_into": Gear, "registrations": n}` | 404; 404 G unknown or not approved; 400 G is the source; 409 not `waiting` |
| `POST /contribution/review/:gear/decline` | admin | `{"reason": string}` (non-blank) | 200 `Gear` | 404; 400 blank reason; 409 not `waiting` |

Route groups: `/contribution/review/*` behind `RequireAdmin`; `/contribution/*` owner routes for every authenticated user, where a gear that is not the caller's own non-approved gear is 404 (also for admins, BR-034 AC-5, BR-037 AC-5). The review routes read `waiting` gear only: a private item, or any other non-waiting gear that is not approved, is 404 there exactly like an unknown ID (BR-033 AC-9, OC-002 G-1); approved or already-decided gear that was a contribution is 409 (BR-033 AC-2). Merge and replace follow ADR-005 D-4 exactly; BR-033 AC-8's 409 has no reachable case (ADR-005 D-4), so it needs no branch. There is no operation to make gear private or to delete a private item (BR-037 AC-4, AC-7; P-36). Every catalog and maker read applies the ADR-005 D-2 predicate; admin `POST /gear/:id/update` and `DELETE /gear/:id/delete` treat non-approved gear as 404. Account deletion adds the ADR-005 D-6 steps.

### Milestone E — codes

| Method and path | Change | Notes |
|---|---|---|
| `GET /code/lookup?code=…` | **new** | 200 `{"code": string, "own": UserGear \| null, "gear": [GearListItem]}`; empty result is 200 with `own = null` and `gear = []`; blank code is 400. `own` only from the caller's registrations; `gear` only visible gear (OC-002) |
| `PUT /usergear/insert`, `POST /usergear/registration/:usergear/update`, `PUT /usergear/new-gear` | changed | `own_code` (*clearable*); 409 when the caller already uses it on another registration, with body `{"error": …, "conflicting_usergear_registration_id": n, "conflicting_gear_name": string}` so the client can say which item has it and link to it (frontend F10 step 2; it names only the caller's own registration) |
| `PUT /gear/insert`, `POST /gear/:id/update` (admin) | changed | `gear_product_code` (*clearable* on update) |
| `POST /contribution/:gear/update` | changed | `gear_product_code` |

The code travels in a query parameter so codes containing `/` work; the request log already records only the number of query parameters (wave 4).

### Cutover (ADR-004 D-3)

`PUT /loadout/:loadout/item/insert` without `usergear_registration_id` → 400 `an item needs usergear_registration_id`; `gear_id` in the body is then ignored as a read-only field. Migration 2.5.

## 5. Not in this contract

Batch moves (the board sends one move per item; revisit if the board's multi-move needs all-or-nothing), item order within a place, an operation to change an item's registration, deleting a private item (not required, P-36), removing all items of one registration in one request (the frontend's ✓ on a registration with several items, F2 step 3, sends one delete per item), and editing waiting contributions by admins (out of scope in BR-033).
