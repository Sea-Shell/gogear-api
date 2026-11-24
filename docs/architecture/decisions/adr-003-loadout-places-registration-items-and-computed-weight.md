---
type: Architecture Decision
title: ADR-003 Loadout places as rows, registration-backed items, and weights computed on read
description: How gogear-api stores where each loadout item travels (one place row per bag, Worn and Last minute; an item points at one place or none), how items refer to the owner's registrations and survive a registration's deletion, where packing progress lives, and why a loadout's total is computed on every read instead of stored.
tags: [architecture, loadouts, places, bags, packing, weight, sqlite, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# ADR-003 — Loadout places as rows, registration-backed items, and weights computed on read

**Status:** Accepted (architect, 2026-11-22). Milestone A parts implemented (2026-11-22). The milestone B parts (weights from measured values, consumable role, stage) are implemented in code on branch `loadout-loop-milestone-b` (2026-11-24; local only: not merged, not released, not deployed, not accepted). Later parts are not. Answers handoff question Q-C. **Amended 2026-11-22 after the critic review (Loom qa `279ad7b0`):** D-3 (`detachedGearId` without `ON DELETE` action, legacy items read live) and D-5 (a registration's owner never changes); the decisions are otherwise unchanged. Exact tables, triggers, views and endpoints are in [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md); the overall design is [ARD-001](/docs/architecture/design/ard-001-loadout-planning-and-packing.md).

## In brief

**What the system will do:** Each loadout gets a small table of *places*: one row for Worn (created automatically with the loadout), at most one row for Last minute, and one row per bag. A bag row names one of the owner's registrations and, optionally, a parent bag in the same loadout. Each loadout item names one of the owner's registrations and at most one place; "no place" is simply no place. Ticks are a column on the item and "closed" is a column on the place. A loadout's total is not stored: one SQL view computes the on-your-back weight from current registration and catalog weights every time it is read.

**Why this design:** It makes "every item is in exactly one place" true by construction (one nullable column), and it lets the database do most of the hard work: removing a bag or deleting a registration moves the contents to "no place" through foreign-key actions in the same statement, so the effects can never be half done. Computing the total on read removes the stale-total bug that exists today (wave 6 note) and needs no bookkeeping when an admin edits a catalog weight or merges a contribution.

**Normal example:** Loadout L1 has places `Worn` (id 40), `Exos 58` (bag, id 41) and `Lid` (bag, id 42, parent 41). The stove item has `place_id = 41`, the rain shell `place_id = 40`, the spork `place_id = null`. The owner removes Exos 58 from the trip: row 41 is deleted, the stove's `place_id` becomes `null` and Lid's parent becomes `null` (top level), all in one statement. Reading L1 then reports a total computed from the current weights.

**Most important limits:** Place IDs are opaque numbers; the client tells Worn from bags by `kind`. Capacity and the weight goal are reported, never enforced. The container-capability rule for bags is enforced by the API only, not by a database trigger, because a copy must reproduce a bag that has since lost its capability (BR-031 AC-3).

## Derived from

- [BR-023](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md), [BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md), [BR-025](/docs/requirements/requirements/br-025-packing-progress-saved-per-loadout.md), [BR-027](/docs/requirements/requirements/br-027-consumable-role-per-loadout-item.md), [BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md), [BR-030](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md), [BR-031](/docs/requirements/requirements/br-031-copy-a-loadout.md), [BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md) AC-6 and AC-7, [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) G-3 and G-5
- [Loadout planning and packing decisions — LP3](/docs/anchors/loadout-planning-and-packing.md) (Worn and Last minute are per-loadout places)
- [ADR-001](/docs/architecture/decisions/adr-001-category-driven-container-capability.md) (one view is the only definition of a derived rule; the database repeats the API's rules)
- Current code at `f82fb7b`: `loadout_items(gearId NOT NULL, quantity, notes)`, `loadouts.totalWeight` recomputed by `LoadoutRecalculateWeight` on item writes only (`pkg/api/loadout_helpers.go`); every transaction is `BEGIN IMMEDIATE` (`utils.SQLiteDSN` adds `_txlock=immediate`); bundled SQLite 3.53.2 (`go-sqlite3 v1.14.47`)

## Satisfies

| Requirement | Satisfied by |
|---|---|
| BR-024 AC-1 (Worn always, at most one Last minute) | D-1: `AFTER INSERT` trigger on `loadouts` creates Worn; partial unique indexes on kind; no delete operation for Worn |
| BR-024 AC-2, AC-3, AC-5, AC-6, AC-10 | D-1 (one `placeId` per item; bag parent must be a bag of the same loadout); D-5 (database triggers repeat ownership, same-loadout and cycle rules); immediate transactions serialize concurrent requests |
| BR-024 AC-4 (trip nesting separate from closet links) | D-1: places never read or write `user_container_registration` except to choose the starting parent once |
| BR-024 AC-8, BR-030 AC-3 (removing a bag or the registration) | D-1: `parentPlaceId` and `loadout_items.placeId` are `ON DELETE SET NULL`; a bag row is `ON DELETE CASCADE` from its registration |
| BR-024 AC-9 (bag order) | D-1: `position` on bag rows; one reorder operation |
| BR-023 AC-1, AC-2, AC-4, AC-5 | D-2: item stores `registrationId`, never a copy of the gear; several items may name the same registration |
| BR-030 AC-1, AC-2, AC-4 | D-3: `registrationId ON DELETE SET NULL` plus a `BEFORE DELETE` trigger that writes the snapshot, all in the deleting statement |
| BR-025 AC-1 to AC-5, AC-9 | D-4: `ticked` on items, `closed` on places; removed with their row |
| BR-027 AC-1 to AC-3 | D-4: `consumable` on items |
| BR-029 AC-2, AC-3, AC-4 | D-6: views `registration_effective_weight`, `loadout_item_weight`, `loadout_on_back_weight` |
| BR-031 AC-7 | one immediate transaction (ARS-001 copy) |

## Decision question

How should a loadout store the place of each item, per-trip bag nesting, packing progress and the link to the owner's registrations, and what should a loadout's total be, so that BR-023 to BR-025, BR-029 and BR-030 hold under concurrent requests on SQLite with enforced foreign keys?

## Drivers and constraints

- Every item in exactly one place, also under concurrent moves (BR-024 AC-5, AC-10).
- Removing a bag or deleting a registration must never lose an item and must be all or nothing (BR-024 AC-8, BR-030 AC-4).
- Trip nesting is separate from closet links (BR-024 AC-4), but starts from them.
- A total that is always current after catalog edits, measured-weight changes and merges (BR-029 AC-2). Today's stored `totalWeight` goes stale after a catalog edit.
- SQLite: `ALTER TABLE` cannot add a constraint to an existing column, so changing `loadout_items` needs a table rebuild. `loadout_items` is not referenced by any table, so it can be rebuilt with foreign keys on; `loadouts` is a parent and cannot be rebuilt (migration 000005 note), but `DROP COLUMN` works on 3.53.2.
- The owner "expects places to be stored on their own" but leaves it to the architect (anchor clarification).

## Options considered

### Places

**P-A — Place columns on the item and the loadout.** `loadout_items.placeKind ∈ {none, worn, last_minute, bag}` plus `placeBagId`; a `loadout_bags` table for bags; `loadouts.hasLastMinute`, `wornClosed`, `lastMinuteClosed`.
- Works: one row per item, so still one place per item.
- Costs: the item needs a CHECK tying `placeKind` to `placeBagId`; three different homes for "closed"; removing a bag must update `placeKind` and `placeBagId` together, which a single `ON DELETE SET NULL` cannot do (it would leave `placeKind = 'bag'` with no bag), so it needs a trigger or handler code; removing Last minute is a handler update of every item.
- Failure mode: a handler that forgets one of the paired updates leaves an item "in a bag" that does not exist.

**P-B — One `loadout_places` table for every closable place (chosen).** Rows of kind `worn`, `last_minute` or `bag`; the item has one nullable `placeId`.
- Works: "no place" is `NULL`; every other place is a row. "Closed" has one home. Removing any place is a delete, and `ON DELETE SET NULL` moves its items to no place and its child bags to top level in the same statement.
- Costs: Worn is a row that must exist for every loadout (an `AFTER INSERT` trigger and a backfill); same-loadout membership needs triggers, because a composite foreign key with `SET NULL` would also null the item's `loadoutId`.
- Failure mode: a missing Worn row. Prevented by the trigger on every insert path, including copy, and the backfill.

**P-C — Nest bags through closet container links.** Use `user_container_registration` as the trip nesting and keep only "which registrations are bags" per loadout.
- Rejected: BR-024 AC-4 requires that trip nesting never changes closet links and vice versa. Two trips with different nesting of the same bags would be impossible.

P-B wins: it needs no paired updates, puts BR-024 AC-8 and BR-030 AC-3 into foreign-key actions, and gives one closed-state column for BR-025. P-A's extra handler logic is exactly the kind of rule that drifts.

### Total weight

**W-A — Keep a stored `totalWeight`, recomputed on every write that can change it.** Writes that change it: item and place changes, registration measured weight, closet changes, catalog gear weight edits by admins (affecting every user's loadouts), merges and replaces.
- Failure mode: one forgotten write path gives a stale total, which is today's bug (wave 6 note). The catalog edit path crosses ownership boundaries.

**W-B — Compute on read with one view (chosen).** `loadout_on_back_weight` sums current effective weights.
- Costs: a per-read aggregate. Data is small (two users today; loadouts of tens of items). A list read computes one sum per loadout.
- Failure mode: slow list reads at much larger scale. Revisit trigger below.

W-B wins: it satisfies BR-029 AC-2 by construction and has one definition of AC-3, as ADR-001 did for container capability.

## Decision

- **D-1 — Places.** Table `loadout_places(placeId, loadoutId, placeKind, registrationId, parentPlaceId, position, closed)`. Worn is created by an `AFTER INSERT ON loadouts` trigger and backfilled for existing loadouts; Last minute and bags are added and removed by the owner. A bag's parent is another bag of the same loadout. Bag `position` orders bags; a new bag goes last; removal leaves gaps, which keeps the others' order.
- **D-2 — Items name a registration.** `loadout_items.registrationId` (owner's registration, `ON DELETE SET NULL`) replaces `gearId` as the item's identity. Gear, maker, category and weights are always read through the registration, so a merge or replace (one update of `user_gear_registrations.gearId`) is seen by every item at once and changes nothing stored on the item. The response keeps a read-only `gear_id` derived from the registration ([ADR-004](/docs/architecture/decisions/adr-004-expand-contract-release-of-the-loadout-item-contract.md)). An item's registration cannot be changed after creation; to use another registration, remove and add. This keeps BR-015 AC-6 trivially true for updates.
- **D-3 — Detached items.** An item whose registration is gone keeps a snapshot: `detachedReason = 'registration_deleted'`, the gear ID, gear name, effective unit weight and its source at deletion time. A `BEFORE DELETE ON user_gear_registrations` trigger writes the snapshot, then the foreign-key action sets `registrationId` to `NULL`, in the same statement. Every path that deletes a registration (owner delete, account deletion) gets this without handler code. The same columns carry the temporary `detachedReason = 'legacy'` of ADR-004; a legacy item has no snapshot and reads its gear live. `detachedGearId` has no `ON DELETE` action, so gear that a detached item references cannot be deleted (the catalog delete answers 409, as it does today for gear in loadouts); merge and replace repoint it first (ADR-005 D-4). *(Amended 2026-11-22 after the critic review, finding F3: the earlier `ON DELETE SET NULL` let an admin delete such gear and left legacy items without name, weight or `gear_id`.)*
- **D-4 — Progress and role are columns.** `loadout_items.ticked`, `loadout_items.consumable`, `loadout_places.closed`. Progress is part of the loadout, not a separate resource or table: it has the item's and place's lifecycle (BR-025 AC-5) for free, and copy simply does not copy it (BR-025 AC-1). Ticks and closing have their own set-to-value operations (ARS-001), so an offline replay can never change anything else about the item.
- **D-5 — Database guards.** Triggers repeat the API's structural rules, as migrations 000005 and 000008 do for container links: an item's or bag's registration belongs to the loadout's owner, and a registration's owner never changes (otherwise a direct owner change would leave existing items and bags pointing across owners); an item's place and a bag's parent belong to the same loadout; a parent is a bag; no bag is inside itself at any depth. The API checks first so it can answer with the defined status and order (404, then 400, then 409; BR-024 AC-11). Container capability for bags is checked by the API only (see limits).
- **D-6 — Weights computed by views.** `registration_effective_weight` (measured weight if set, else catalog weight, else none; with its source) is the only definition of effective unit weight. `loadout_item_weight` applies it to items (or the snapshot for detached items). `loadout_on_back_weight` is the only definition of BR-029 AC-3: items not in Worn × quantity, plus each bag's own unit weight once. `loadouts.totalWeight` and `LoadoutRecalculateWeight` are removed. The response field `total_weight` stays and now means on-your-back grams.
- **D-7 — Catalog weight "none".** A catalog `gearWeight` that is `NULL` or `≤ 0` is reported as no weight (`null`, source `none`). Admin gear create without a weight stores `NULL` from now on. Today's `GearInput` stores `0` for an absent weight, so `0` cannot be told apart from "unknown".

## Consequences

- Removing a bag, removing Last minute and deleting a registration need no item updates in handler code.
- Loadout reads join four tables and a view; list reads compute a total and counts per loadout.
- The rebuild of `loadout_items` (ARS-001 migration 000011) drops its old triggers; the quantity rule moves into a CHECK constraint on the new table.
- Container-capability rule for bags lives in one place, the API (add bag, move into a bag, set a bag's parent). A handler that forgets it would let a non-container receive contents in a trip; the API tests in ARS-001 cover each path.

**Revisit triggers:** a measured `/loadout/list` p95 above 300 ms for a real user (then cache totals per request or index the aggregate); a requirement to order items within a place (out of scope today); a requirement that copies refuse bags that lost capability.

## Confirmation

- Migration test on a database at version 10 with items that match, do not match, and have missing parents: afterwards every loadout has one Worn row, matched items point to the owner's lowest-ID registration of the same gear, the rest follow ADR-004.
- API tests from the verification semantics of BR-024, BR-025 and BR-030, including the race tests of BR-024 AC-10.
- A direct-SQL test that inserts a cycle, a foreign registration, or a place of another loadout: each is refused by a trigger.
- BR-029 AC-3 example (1 994 g) read through `GET /loadout/{id}/full` and `/loadout/list`, then again after an admin catalog edit and a measured-weight change.
- A test that deletes a registration used as an item and as a bag: item snapshot written, bag gone, contents at no place, in one statement (inject a failure after the delete in the same transaction: nothing changed).
