---
type: Architecture Decision
title: ADR-005 Gear catalog states for contributions and private items, one visibility rule, and a merge that only repoints
description: How gogear-api realizes catalog contributions (BR-032 to BR-034), private items (BR-037) and OC-002 - a catalog state and creator on the gear row itself, one visibility predicate used by every catalog read and reference, and a merge or replace that changes only which gear the user's registrations point to, so everything the user provided is kept.
tags: [architecture, catalog, catalog-contributions, private-items, visibility, authorization, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# ADR-005 — Gear catalog states for contributions and private items, one visibility rule, and a merge that only repoints

**Status:** Accepted (architect, 2026-11-22). Not implemented. **Amended 2026-11-22:** the private-item parts are reconciled with [BR-037](/docs/requirements/requirements/br-037-private-items.md), the amended OC-002 and BR-033 AC-8 and AC-9. Changes: a private item cannot be replaced (BR-037 defines no replace, so it is 409); sending is one way; private items are not in the contributor's contribution list; BR-033 AC-8's refusal has no reachable case in this design (D-4). Exact schema and endpoints are in [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md).

## In brief

**What the system will do:** Every gear row gets a *catalog state*: `approved` (ordinary catalog gear, as all gear is today), `private` (visible only to its creator), `waiting` (a contribution awaiting an admin), or `not_approved` (declined, with a reason). It also records who created it. One rule decides visibility everywhere: gear is visible to a user if it is approved, or if that user created it and it is not approved. Makers get the same treatment with `approved` / `pending`. An admin merge, or a user's replace, changes the `gearId` of the user's registrations to the target gear and deletes the old gear row. Nothing else of the user's data is written.

**Why this design:** The user must be able to register and use their unapproved or private gear like any other gear (BR-002 AC-3, BR-037 AC-3), so it has to be a real gear row that registrations can reference. Putting the state on that row needs no second table for the main flow and makes the visibility rule one SQL predicate. Because loadout items and bags point at registrations (ADR-003 D-2), changing a registration's gear moves every item, bag, tick, note and role with it without touching them, which is what the owner made a condition of accepting DC-2.

**Normal example:** Ada adds "Home-made trail mix" as a private item with 300 g "I weighed it": one gear row (`private`, creator Ada) and one registration (measured 300 g weighed). She uses it in two loadouts. Bob's catalog list, search and code lookup never show it. Later Ada sends it to the catalog: it becomes `waiting` and appears in the admins' review list. An admin finds it duplicates approved gear G and merges: Ada's registration now points to G, still with 300 g weighed, count 1, its own code, its container links; both loadouts keep every place, tick, note and consumable role. The trail-mix gear row is deleted and Ada's contribution list shows "merged into G".

**Most important limits:** Merged and withdrawn gear is deleted, not kept with a state; only merges leave a history row (BR-034 AC-1 does not require listing withdrawn ones). Admins do not see private or unapproved gear outside the review operations, even through admin catalog endpoints.

## Derived from

- [BR-037](/docs/requirements/requirements/br-037-private-items.md) (private items), [BR-002](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) AC-1 to AC-6, [BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md), [BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md), [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md), [OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md), [BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md) AC-8, [BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md) AC-6, [BR-035](/docs/requirements/requirements/br-035-codes-and-scanning.md) AC-4 and AC-6, [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) invariant (the merge exception)
- [Loadout planning and packing decisions — LP2, LP8, DC-1, DC-2](/docs/anchors/loadout-planning-and-packing.md) (LP8: private items for everyone; DC-2 accepted by the owner on the condition that everything the user provided is kept), [BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md) AC-8 and AC-9, provisional defaults P-35 and P-36
- Current code: catalog list and search queries build their own `SELECT` and `COUNT(*)` strings (`pkg/api/gear.go`, `manufacture.go`); catalog writes are behind `RequireAdmin` in `RegisterCatalogRoutes` (`pkg/api/catalog.go`)

## Satisfies

| Requirement | Satisfied by |
|---|---|
| OC-002 G-1, G-2, G-3 | D-2 (one predicate, applied to rows and counts); D-5 (database trigger on registrations) |
| OC-002 G-4 | immediate transactions: a decision and a read never interleave within one statement or transaction |
| OC-002 G-5, BR-016 AC-6, private items in public loadouts | D-2 public-view filter (approved only) |
| BR-002 AC-3, BR-032 AC-2, AC-3, AC-7 | D-1, D-3 (one transaction creates gear, optional new maker and registration) |
| BR-033 AC-2 to AC-6, BR-034 AC-2 to AC-6 | D-1 state machine; D-4 merge and replace; state checked inside the deciding transaction |
| BR-033 AC-8 (keep everything the user provided; 409 if anything would be lost), BR-034 AC-3 | D-4 (only `user_gear_registrations.gearId` and detached items' `detachedGearId` are written; no loss path exists) |
| BR-033 AC-9, BR-037 AC-2 (never in the admin list) | D-1 (review list reads `waiting` only); D-2 |
| BR-009 AC-8, BR-037 AC-9 | D-6 |
| BR-037 AC-1, AC-3 | D-1 state `private`; D-3 `catalog_state = "private"` on creation, for any user |
| BR-037 AC-4, AC-5, AC-7, BR-002 AC-6 | D-1 transitions: edit while `private`; `private → waiting` only; no transition into `private` |
| BR-037 AC-6, BR-016 AC-6 | D-2 public-view filter |
| BR-037 AC-8 | D-1: creating, editing and sending a private item go through owner routes, never through admin catalog routes |

## Decision question

Where do contribution and private-item states live, how is "visible to the caller" applied consistently, and how does a merge keep all of the user's data?

## Options considered

**S-A — State on the gear row (chosen).** Columns `gearCatalogState`, `gearCreatorUserId`, `gearStateReason`, timestamps and the contributed weight source. Merge deletes the gear row and writes a small history row.
- Benefits: registrations reference the gear row directly, as for approved gear; one predicate on the `gear` table; no join for the common case.
- Costs: every catalog query must apply the predicate, including counts. Mitigated by one helper and a route-table test (D-2).
- Failure mode: a new catalog query that skips the helper leaks a private or waiting item. The test enumerates every registered route that returns gear or makers.

**S-B — Separate `gear_contributions` table, gear row created on approval.**
- Rejected: an unapproved contribution must already be registrable and usable in loadouts (BR-002 AC-3), so it needs a gear row before approval; a second table would duplicate the gear fields or still need a state on the gear row.

**S-C — Keep merged gear as a row with state `merged`.**
- Rejected: the merged row would keep its maker referenced (`gearManufactureId NOT NULL`), so the new maker could not be removed (BR-033 AC-4), and every query would need to exclude another state. A history row is cheaper.

**Visibility realization V-A — SQL view per caller** is not possible in SQLite (no session parameters). **V-B — one Go predicate builder plus a route-table test (chosen).** **V-C — row-level filtering in each handler by hand** is today's pattern and is what OC-002 warns against.

## Decision

- **D-1 — States and transitions.**
  - Gear: `approved` | `private` | `waiting` | `not_approved`. Existing gear and gear created by an admin through the catalog endpoints is `approved`.
  - Creating new gear through `PUT /usergear/new-gear` (ARS-001): `catalog_state = "catalog"` gives `waiting` for a non-admin and `approved` for an admin (BR-032 AC-4); `catalog_state = "private"` gives `private` for any user, admins included (BR-037 AC-1).
  - Creator transitions:
    - edit fields while `private`, `waiting` or `not_approved` (BR-037 AC-4, BR-034 AC-2); `not_approved` stays `not_approved` until sent;
    - send: `private → waiting` (BR-037 AC-5) and `not_approved → waiting` (BR-034 AC-2); the reason is cleared. An admin's private item also becomes `waiting` when sent, as BR-037 AC-5 states; the admin can then approve it in the review list;
    - replace with approved gear G while `waiting` or `not_approved` (BR-034 AC-3). Replacing a `private` item is 409: BR-037 defines no replace, and its "send to the catalog" is the only way out of `private`;
    - there is no transition into `private` (BR-037 AC-7) and no delete of a private item (P-36; a private item with no registrations stays).
  - Admin transitions, only from `waiting`: `approve → approved`; `decline (reason) → not_approved`; `merge (G) →` gear deleted. A private item is never in the review list and no decision applies to it (BR-033 AC-9); to an admin's review routes it is "not found" like an unknown ID (OC-002 G-1).
  - Any request that does not match its allowed source state is 409 (BR-033 AC-2, BR-034 AC-4).
  - Makers: `approved` | `pending`. A new maker of a contribution or private item (BR-037's "private maker") is `pending` with the creator recorded; sending a private item to the catalog keeps the same maker row, so it becomes the contribution's new maker (BR-037 AC-5). It becomes `approved` when gear that uses it is approved. It is deleted when the last gear using it is removed by merge, replace or account deletion while it is still `pending`.
- **D-2 — One visibility rule.** In Go, one function returns the SQL predicate and its arguments for a gear alias and a caller:
  `(g.gearCatalogState = 'approved' OR (g.gearCreatorUserId = :caller AND g.gearCatalogState <> 'approved'))`. The maker predicate is analogous. It is used by every catalog list, search, count, single read, code lookup, maker list and gear reference in a write (registration create, merge and replace target: approved only). Category and top-category reads that count or join gear use it too. The public view (no caller) uses `approved` only, in the item list; the weights of hidden items still count in its total (OC-002 G-5). Admin catalog update and delete by ID use the same rule with the admin as caller, so a non-approved row is 404 to them (OC-002 G-1). The only exception is the review list and review decisions, which read `waiting` rows directly.
- **D-3 — Creating new gear and its registration is one transaction.** Optional new maker, gear row, and one registration of the caller (with measured weight `weighed` when the weight source is "I weighed it", BR-032 AC-2). Any failure rolls back all three.
- **D-4 — Merge and replace only repoint.** In one immediate transaction:
  1. check the source state (409) and the target G (approved, not the source; 404 or 400);
  2. `UPDATE user_gear_registrations SET gearId = G WHERE gearId = X`;
  3. `UPDATE loadout_items SET detachedGearId = G WHERE detachedGearId = X` (registration-deleted items keep their snapshot name and weight; a legacy item, if the cutover has not happened yet, then reads G live). This step must run before step 6, because `detachedGearId` has no `ON DELETE` action and would otherwise block deleting X;
  4. product code: copy X's code to G if G has none (BR-035 AC-6, milestone E);
  5. merge only: insert a `gear_contribution_merges` row (creator, contributed name, G, G's name, time);
  6. delete gear X; delete X's maker if it is `pending` and unused.
  **Kept unchanged, by construction:** registration ID, measured weight and source, closet count, maximum container weight, own code and container links (all columns or rows of the registration, not of the gear); every loadout item's place, quantity, notes, tick and consumable role, and every bag row with its parent, order and closed state (they reference the registration ID). Only steps 2 and 3 write user-owned data. This is the OC-001 admin exception, and it is narrower than "changes which gear the registrations refer to" only by step 3, which applies the same repointing to detached items. Container links are not re-checked (the link triggers fire only on link insert and update), and a bag whose new gear is not container-capable keeps its contents (BR-024 AC-6).
  **BR-033 AC-8's 409:** no value listed in AC-8 is stored on or keyed by the gear row, and no uniqueness rule involves a registration's gear, so no merge or replace can lose one. The API therefore has no refusal branch for it; the row-diff test (Confirmation) is the proof. Revisit trigger: any later rule keyed by gear on user-owned data (for example "one registration per gear per user", or a bag rule keyed by gear) creates a loss path, and the merge must then check it and answer 409 before writing.
- **D-5 — Database guard.** A trigger on `user_gear_registrations` insert and `UPDATE OF gearId` refuses gear that is not visible to the registration's owner (approved, or created by that owner). It backs OC-002 G-3 for the one write that references gear across ownership.
- **D-6 — Account deletion.** In the existing deletion transaction, after loadouts and registrations: delete the user's gear in any non-approved state, then the user's `pending` makers that are now unused. Approved gear keeps its row; `gearCreatorUserId` becomes `NULL` through `ON DELETE SET NULL`. A CHECK makes a non-approved gear row without a creator impossible, so a missed delete fails the account deletion instead of publishing the row.

## Consequences

- Every catalog query in `gear.go`, `manufacture.go`, `category.go` and `topCategory.go` changes to use the predicate; the review list and decisions are new admin routes.
- BR-034 AC-1's "merged, with the gear it was merged into" survives G's later deletion through the stored target name.
- Private items reuse the whole contribution machinery; the only additions are one state, one creation choice and one transition.
- The contributor's list (`GET /contribution/list`) shows contributions only (BR-034 AC-1). Private items are not contributions (BR-002 AC-6) and are seen in the owner's closet and catalog reads, marked `private`.

**Revisit trigger:** a requirement to keep withdrawn contributions in the contributor's list, or to let admins edit a waiting contribution before deciding.

## Confirmation

- The OC-002 verification run, extended with a private item: as another user and as an admin outside the review list, no route shows the item or its new maker; counts unchanged; single reads equal to unknown IDs; the public view leaves the item out but counts its weight.
- A route-table test: for every route registered by `main` that returns gear or makers, a private and a waiting gear of user A never appear for user B.
- Merge test seeded with a registration that has measured weight, count 3, own code, a container link, an item in a bag with a tick, a note and the consumable role, a second item in Worn, and a bag row: after the merge, a byte-for-byte comparison of every user-owned row shows changes only in `user_gear_registrations.gearId` (and `detachedGearId` of detached items).
- Race test: an approval and a replace of the same contribution, many times: exactly one succeeds, the other is 409.
