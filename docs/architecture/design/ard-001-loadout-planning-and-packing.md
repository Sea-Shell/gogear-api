---
type: Architecture Design
title: ARD-001 Loadout planning and packing - API realization
description: How gogear-api realizes BR-023 to BR-037 (including private items), OC-002 and the 2026-11-22 amendments - registration-backed loadout items, a place table per loadout for bags, Worn and Last minute, progress as columns, weights computed by views, gear catalog states with one visibility rule, codes, five additive milestones and a final cutover so the deployed frontend never breaks.
tags: [architecture, loadouts, places, packing, weight, catalog-contributions, private-items, migration, release, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# ARD-001 — Loadout planning and packing: API realization

**Status:** Accepted (architect, 2026-11-22). Milestone A implemented (2026-11-22). Milestone B implemented in code on branch `loadout-loop-milestone-b` (2026-11-24; local only: not merged, not released, not deployed, not accepted). Milestones C to E and the cutover are not. Decisions: [ADR-003](/docs/architecture/decisions/adr-003-loadout-places-registration-items-and-computed-weight.md) (places, items, progress, weights), [ADR-004](/docs/architecture/decisions/adr-004-expand-contract-release-of-the-loadout-item-contract.md) (release and compatibility), [ADR-005](/docs/architecture/decisions/adr-005-gear-catalog-states-contributions-and-private-items.md) (contributions, private items, visibility, merge). Exact contract: [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md). An independent critique is recommended before milestone A is implemented. **Amended 2026-11-22:** reconciled with BR-037, the amended OC-002 and BR-033 AC-8 and AC-9, and the updated frontend design (F8 to F10); BR-030 confirmed in milestone A by the dispatcher; the plan note on BR-023's partial status added. No open markers remain. **Amended after the critic review (Loom qa `279ad7b0`):** binding one-merge rule per migration (F1); ID sequence kept in the 000011 rebuild (F2); gear used by detached items cannot be deleted (F3); legacy items read their gear live (F4); a closet default parent that lost capability is skipped (F5); owner-change guard, down-migration order, public filter in A and the `gear_weight` 0 → `null` note (advisories).

## In brief

**What the system will do:** A loadout item stops being "catalog gear × quantity" and becomes "one of my registrations × quantity, in one place, ticked or not, consumable or not". Each loadout keeps a small list of places: a built-in Worn, an optional Last minute and any number of bags, which are the owner's container registrations, nested for this trip only. Registrations gain the owner's own measured weight, a closet count and an own code. Gear gains a catalog state, so a user can add gear that is private, or waiting for approval, and use it at once while nobody else sees it. A loadout's `total_weight` is always computed at read time as its on-your-back weight.

**Why this design:** The rules the frontend depends on ("every item in exactly one place", "removing a bag never loses an item", "a deleted closet item stays in the loadout with its weight", "a merge keeps everything I entered") are realized by the shape of the data and by foreign-key actions, not by handler code that must remember them. Weights are computed by one set of views, so they cannot go stale. Visibility of private and unapproved gear is one predicate. The release is additive until one final cutover, so neither repository has to hold merges.

**Normal example (the requirements index's example of success):** The owner makes "Exos 58" and "Lid" bags of "Hardangervidda" (`PUT /loadout/7/place/insert` twice; Lid starts inside Exos because it is inside Exos in the closet). They sort 24 items with `POST /loadout/7/item/{id}/update {"place_id": …}` into the bags, Worn and Last minute, and set a 10 kg goal. On the phone they tick five items (`POST …/item/{id}/tick {"ticked": true}`) and close Lid (`POST …/place/{lid}/close {"closed": true}`). On the laptop `GET /loadout/7/full` returns the same ticks and closed state, and `total_weight` uses their measured weights. A tick sent later from an offline phone for an item removed meanwhile gets 404 for that item only.

**Most important limits:** The client computes base, worn and skin-out weight and the uncertainty (BR-029 AC-7). Until the cutover (ADR-004 D-3), items added by catalog `gear_id` alone are still accepted, so BR-023 AC-1 and AC-7 are met only after it. A private item cannot be deleted or replaced; its only way out of `private` is sending it to the catalog (BR-037, P-36).

## Derived from

- Anchor [Loadout planning and packing decisions](/docs/anchors/loadout-planning-and-packing.md) (LP1 to LP8, DC-1, DC-2 as accepted with the owner's condition); [access control](/docs/anchors/access-control-and-identity.md) (D1, D4, D9, D11); [release](/docs/anchors/release-and-deployment.md) (RD1, RD2)
- Requirements BR-023 to BR-037, OC-002, and the 2026-11-22 amendments of BR-001, BR-002, BR-009, BR-012, BR-014, BR-015, BR-016, BR-033 (AC-8, AC-9), OC-001; provisional defaults up to P-36 ([index](/docs/requirements/index.md))
- Dispatcher decisions of 2026-11-22: BR-030 belongs in milestone A; BR-023 is marked partially implemented after A (until the cutover)
- Frontend design "Loadout planning and packing" (frontend repository `docs/design/design-spec/loadout-planning-and-packing.md`, as amended 2026-11-22): weight model, F1 to F10, N1 to N12, DD-9 to DD-11, failure and offline behavior
- [ADR-001](/docs/architecture/decisions/adr-001-category-driven-container-capability.md), [ADR-002](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md)
- Code at `f82fb7b` (read this session): migrations 000001 to 000010, `pkg/models/loadout.go`, `usergear.go`, `gear.go`, `pkg/api/routes.go`, `catalog.go`, `loadout_helpers.go`, `user.go` (account deletion order), `main.go` (`runMigrations`), `utils.SQLiteDSN` (`_txlock=immediate`, `_foreign_keys=on`), go-sqlite3 v1.14.47 with SQLite 3.53.2, golang-migrate v4.19.1

## Satisfies

Milestones as given by the dispatcher (A to E). Where the requirements index orders differently, see "Milestones" below.

| Requirement | Milestone | Satisfied by | Evidence |
|---|---|---|---|
| BR-023 AC-1 | A (gear-only path), cutover (refusal) | ADR-004 D-2, D-3; ARS-001 item insert | API test incl. foreign/unknown registration 404; cutover test 400 for gear-only |
| BR-023 AC-2 to AC-6, AC-9 | A (AC-6 from D) | ADR-003 D-2 | verification semantics of BR-023 |
| BR-023 AC-7 | A + cutover | migration 000011 binding; cutover migration | migration tests |
| BR-023 AC-8 | every milestone | Swagger regenerated; ADR-004 D-1 | Swagger diff review |
| BR-024 AC-1 to AC-12 | A | ADR-003 D-1, D-5; ARS-001 place endpoints | BR-024 verification incl. races |
| BR-025 AC-1 to AC-9 | A (AC-8 stage part with B) | ADR-003 D-4; tick and close endpoints | BR-025 verification with two sessions |
| BR-026 AC-1 to AC-5 | B (AC-5 with D) | migration 000012; `registration_effective_weight`; ADR-005 D-4 | BR-026 verification |
| BR-027 AC-1 to AC-5 | B (AC-4 with C) | `loadout_items.consumable` | BR-027 verification |
| BR-028 AC-1 to AC-6 | B | migration 000012; stage rule in ARS-001 | BR-028 verification |
| BR-029 AC-1 | A (catalog parts), B (measured, count, role, stage), D (state), E (own code) | ARS-001 response shapes | field-presence tests per milestone |
| BR-029 AC-2, AC-3 | A | ADR-003 D-6 views | 1 994 g example; re-read after catalog edit |
| BR-029 AC-4 | A (counts, total, closed places), B (stage, dates, goal) | `Loadout` fields on `/loadout/list` | list test |
| BR-029 AC-5 | A | `GearListItem.gear_weight` | catalog list test |
| BR-029 AC-6 | A, B, D, E | `UserGear` fields | registration list test |
| BR-029 AC-7 | — | client computes | — |
| BR-030 AC-1 to AC-5 | **A** (accepted by the dispatcher), snapshot uses measured weight from B | ADR-003 D-3; delete trigger and FK actions | BR-030 verification incl. injected failure |
| BR-031 AC-1 to AC-8 | C | ARS-001 copy | BR-031 verification |
| BR-032 AC-1 to AC-7 | D | ADR-005 D-1, D-3; `new-gear` | BR-032 verification |
| BR-033 AC-1 to AC-7 | D | ADR-005 D-1, D-4; review routes | BR-033 verification incl. race |
| BR-033 AC-8 (merge keeps everything the user provided) | D | ADR-005 D-4: only registration `gearId` (and detached items' `detachedGearId`) written; no loss path, so its 409 is unreachable | row-diff test before and after merge |
| BR-033 AC-9 | D | review list reads `waiting` only | private item never in the list |
| BR-034 AC-1 to AC-7 | D | ADR-005 D-1, D-4; contribution routes; `gear_contribution_merges` | BR-034 verification |
| BR-035 AC-1 to AC-7 | E | migration 000014; lookup | BR-035 verification |
| BR-036 AC-1 to AC-5 | B | `closetCount` | BR-036 verification |
| OC-001 G-3, invariant | A to D | triggers of ADR-003 D-5 and ADR-005 D-5; merge exception ADR-005 D-4 | direct-SQL trigger tests; merge row-diff test |
| OC-002 G-1 to G-5 | D | ADR-005 D-2 predicate; public filter | OC-002 verification; route-table test |
| BR-001 AC-7, AC-8 | D, E | ADR-005; product code on admin gear routes | BR-001 tests extended |
| BR-002 AC-1 to AC-5 | D | ADR-005 | BR-002 verification |
| BR-009 AC-8 | D | ADR-005 D-6 | account deletion test with private, waiting, approved gear |
| BR-012 amendment | B, E (delete effects A) | registration columns; BR-030 trigger | BR-012 tests extended |
| BR-014 amendment (copy) | C | copy endpoint 404 for foreign | BR-031 AC-1 test |
| BR-015 AC-6, AC-7 | A | item and place triggers; API checks | BR-015 verification incl. L1→L2 move 404 |
| BR-016 AC-5, AC-6 | A (AC-5), D (AC-6) | public items with `gear_name`; approved-only filter | public view tests |
| BR-037 AC-1 to AC-9, BR-002 AC-6 | D | ADR-005 D-1 (`private` state, creation with `catalog_state: "private"`, edit, one-way send, no replace, no delete), D-2, D-6 | BR-037 verification semantics |
| BR-022 (every merge deploys safely) | all | ADR-004 | deployed-frontend replay suite per milestone |

Satisfied elsewhere: BR-020 and BR-021 closet rules are unchanged (ADR-001); the frontend's offline queue and "try without" are client concerns (BR-025 out of scope, BR-029 AC-3 note).

## Problem

The API stores a loadout item as catalog gear, quantity and notes, and a stored total that goes stale. The reworked frontend needs the physical item, where it travels, packing progress that survives devices, honest per-item weights, trip state, safe copies, contributions and private gear, and codes. Every change deploys on merge while an older frontend is live.

## Constraints and forces

- SQLite with enforced foreign keys; migrations run at start in one transaction each (ADR-002); `loadouts` cannot be rebuilt (it is a parent), `loadout_items` can.
- All writes are `BEGIN IMMEDIATE`, so concurrent writes are serialized; this is the concurrency mechanism for every race criterion.
- Ownership: 404 for foreign or wrong-parent targets; no admin exceptions except the BR-033 merge.
- Never refuse for capacity, goal or closet count (P3 "never block, always show").
- Stored data is test data; migrations must still apply cleanly and automatically.

## Invariants and guarantees

| Guarantee | Enforcement | Falsified by |
|---|---|---|
| I-1 Every item is in exactly one place | one nullable `placeId`; place trigger (same loadout); FK `SET NULL` on place removal | a stored `placeId` of another loadout or a missing place |
| I-2 Every loadout has exactly one Worn | `AFTER INSERT` trigger; partial unique index; backfill; no delete operation for Worn | a loadout with 0 or 2 Worn rows |
| I-3 Bags nest without cycles, inside bags of the same loadout | API checks; parent and cycle triggers | direct-SQL cycle insert succeeding |
| I-4 Items and bags use only the loadout owner's registrations | API 404; owner triggers; `trg_registration_owner_fixed` (a registration never changes owner) | direct-SQL foreign registration insert succeeding |
| I-5 A registration's deletion never loses an item and is all or nothing | `BEFORE DELETE` snapshot trigger + FK actions in one statement | an item missing or still pointing at a deleted registration or bag |
| I-6 Totals and effective weights are current | computed by views only; no stored totals | any read after a successful weight change showing the old value |
| I-7 Private and unapproved gear is invisible to everyone but its creator (review list aside) | ADR-005 D-2 predicate everywhere; registration trigger; public filter | any route returning it to another user |
| I-8 A merge or replace changes only which gear registrations (and detached items) point to | ADR-005 D-4 statement list | row diff of user-owned data showing other changes |
| I-9 The deployed frontend's requests keep working until the cutover | additive contract (ADR-004 D-1, D-2) | replay suite failing |

## Options considered

The material choices and their rejected alternatives are developed in the ADRs: place columns on items (ADR-003 P-A) and reuse of closet links (P-C) versus a place table (P-B, chosen); stored versus computed totals (ADR-003 W-A, W-B); coordinated release, breaking release, and API versioning versus expand-then-contract (ADR-004 R-A to R-D); a separate contributions table and kept merged rows versus state on the gear row (ADR-005 S-A to S-C). Progress as a separate resource or table was rejected in ADR-003 D-4: it would need its own lifecycle rules for removal and copy that columns get for free.

## Decision

Realize the work as ADR-003, ADR-004 and ADR-005 decide, with the contract of ARS-001.

## Structure and flows

```text
users ─┬─ user_gear_registrations ──(gearId)── gear ── manufacture
       │      │  measuredWeight, closetCount, ownCode     gearCatalogState, gearCreatorUserId, gearProductCode
       │      │
       │      ├──< user_container_registration   (closet nesting, unchanged)
       │      │
       └─ loadouts ──< loadout_places            (worn | last_minute | bag → registrationId, parentPlaceId, position, closed)
              │  stage, dates, goal      ▲
              └──< loadout_items ────────┘ placeId
                     registrationId (SET NULL) | detached snapshot, quantity, notes, ticked, consumable

views: registration_effective_weight → loadout_item_weight → loadout_on_back_weight
```

**Board read:** `GET /loadout/{id}/full` reads loadout (with total and counts), places and items in one transaction.

**Move:** item update with `place_id`; checks in order: item and place belong to the loadout (404), then capability of a target bag (409); one `UPDATE`.

**Registration deletion:** one `DELETE`; the trigger snapshots items; FK actions detach items, delete bag rows, unplace their contents and lift their child bags.

**Merge (admin) / replace (creator):** one transaction; repoint registrations and detached items; delete the old gear and an unused pending maker.

## Input / operation coverage

Every new operation is listed in ARS-001 section 4 with its refusals. Paired operations: add/remove bag (removal unplaces contents, AC-8); add/remove Last minute; tick/untick and close/open (set-to-value, idempotent); submit/decline/resubmit; create registration/delete registration (BR-030); copy (source only read). Inputs at the edges: `place_id: null` (no place), absent (unchanged); parent absent (closet default) vs `null` (top level); `:place = 0` (400); catalog weight `NULL` or `0` (none); detached items in copy (left out), in totals (snapshot weight), in ticks and moves (allowed).

## Failure and recovery

- Every write is one transaction; a failure rolls back all effects (BR-030 AC-4, BR-031 AC-7, BR-032 AC-2, BR-033 AC-6). No partial-success responses exist.
- Offline tick replay: each request names one item and sets a value; a removed item gives 404 and changes nothing; replays are idempotent (BR-025 AC-2, AC-7).
- Migrations: each runs in one transaction; a failure exits the pod, as today (ADR-002 D-8 evidence line). Migration 000011's rebuild is the riskiest step and has its own test on a version-10 database with orphans.
- A missed API check is caught by a trigger (the request fails instead of storing a broken state); the handler logs it.

## Security / trust boundaries

- Asset: other users' private and unapproved gear, makers and codes; other users' loadouts and registrations.
- Threats: a catalog query that forgets the predicate (leak); a body ID naming another user's registration or another loadout's place (cross-owner write); an admin catalog endpoint reading non-approved rows (leak through admin); the public view listing hidden gear.
- Enforcement: ADR-005 D-2 predicate and route-table test; owner and same-loadout triggers plus API 404s; admin routes use the predicate; public filter approved-only. The review list shows contribution fields and the contributor's username only (BR-033 AC-1).

## Verification and composition

1. **Deployed-frontend replay suite** (ADR-004), run in every milestone.
2. **Migration tests**: 000011 on a version-10 database (matched, unmatched, orphan items; loadouts without items); 000012 view replacement; 000013 on existing gear (all `approved`); 000014; cutover.
3. **Requirement verification semantics** of each BR and OC as API tests through `newRouter`, as the existing `*_authz_test.go` do.
4. **Direct-SQL trigger tests** for I-1 to I-4 and the registration visibility trigger.
5. **Composition checks**: delete a registration that is a bag containing a bag with ticked items, then copy the loadout (detached item left out, nesting correct); merge a contribution whose registration is a bag in two loadouts with ticks and a closed state (I-8 row diff); BR-029 AC-3 example after each of: measured weight change, catalog edit, merge.

## Milestones and migration sequence

| Milestone | Requirements | Migration | Endpoints (ARS-001 section 4) |
|---|---|---|---|
| A | BR-023 (except AC-1 refusal and AC-7 completion), BR-024, BR-025, BR-029 AC-1 catalog parts, AC-2, AC-3, AC-4 counts, AC-5; **BR-030 (recommended here)**; BR-015 AC-6, AC-7; BR-016 AC-5 | `000011_loadout_places_and_registration_items` | full read, place insert/update/order/close/delete, item tick, item insert/update changes, import removed, catalog `gear_weight` |
| B | BR-026, BR-027, BR-028, BR-036, rest of BR-029 | `000012_weights_roles_stage` | registration and loadout field additions, `consumable` |
| C | BR-031, BR-014 amendment | none | copy |
| D | BR-002, BR-032, BR-033, BR-034, BR-037, OC-002, BR-001 AC-7, BR-009 AC-8, BR-016 AC-6 | `000013_gear_catalog_states` | `new-gear`, contribution and review routes; predicate in every catalog read |
| E | BR-035, BR-001 AC-8 | `000014_codes` | code lookup; code fields |
| Cutover | BR-023 AC-1, AC-7 | next free number | gear-only insert refused |

**Why BR-030 is in A (accepted by the dispatcher):** milestone A's migration decides the foreign-key action on `loadout_items.registrationId`. Without `SET NULL` and the snapshot trigger, deleting a registration that is used in a loadout would fail with a foreign-key error from the day items reference registrations. In A the snapshot uses the catalog weight; B's view replacement adds the measured weight automatically. BR-036 has no structural dependency and stays in B.

**Plan note — BR-023 status after A:** when milestone A is implemented, the requirements index must mark BR-023 as **partially implemented**: AC-1 (refusing an item from catalog gear alone) and AC-7 (every stored item refers to a registration) are pending the cutover (ADR-004 D-3). The index update is part of the milestone A implementation task; the cutover task marks BR-023 implemented.

**Merge rule (binding, BR-022): one migration and everything that depends on it reach `main` in one merge.** Every merge to `main` deploys (ADR-002), and a migration runs as soon as the new pod starts. Migration 000011 drops `loadouts.totalWeight` and `loadout_items.gearId`, which today's handlers read (`loadout_helpers.go` `loadoutColumns`, `LoadoutItemsByLoadout`, `LoadoutItemsOwned`, `LoadoutRecalculateWeight`; `loadout_items.go`). If the migration reaches `main` without the handler changes, or the handlers without the migration, the deployed API is broken. So milestone A is **one merge**: the work units below may be separate tasks and commits on a milestone branch, but they are integrated and verified together and merged to `main` as one PR. The same rule holds for every later migration: B's 000012, D's 000013 and E's 000014 each merge together with the code that reads or writes their columns, and the cutover migration merges with the removal of the legacy insert path. A PR that contains a migration but not its dependent code, or the reverse, must not be merged. CI (`go test` on PRs) would probably catch the mismatch, but the plan must not rely on it.

**Recommended task boundaries for planning** (the task-manager decides the plan; all A units integrate into one milestone-A merge):
- A1 migration 000011 with its migration and trigger tests (including the sequence test of ARS-001 2.1); A2 item contract (insert incl. legacy path, update, list, full read, tick) and removal of import; A3 places (insert with closet default, update, order, close, delete) with BR-024 race tests; A4 weights in reads (views, `total_weight`, counts, `GearListItem.gear_weight`, public view) and the replay suite; A5 Swagger and docs; **A6 integration: the full test suite and the replay suite on the combined branch, then the single merge to `main`.** **A6b (added 2026-11-22, BR-038): the automatic pre-migration backup and restore of [ADR-006](/docs/architecture/decisions/adr-006-pre-migration-backup-and-restore.md) and [ARS-002](/docs/architecture/spec/ars-002-pre-migration-backup-and-restore.md) section 9. It is a precondition of the milestone A merge: the PR that brings migration 000011 to `main` must contain it, so the first production start that applies 000011 makes a verified backup.**
- B: one migration task; registration fields; loadout stage, dates, goal; consumable (one merge with its migration).
- C: copy as one task.
- D: D1 migration and predicate helper with route-table test applied to all catalog reads; D2 `new-gear` (contribution and private item); D3 owner routes (edit, send, replace; private items included where BR-037 allows); D4 review routes with merge and the BR-033 AC-8 row-diff test; D5 account deletion.
- A also updates the requirements index for BR-023 (plan note above).
- E: migration and code fields; lookup.
- Cutover: after the frontend board release is deployed.

## Consequences

- Handlers become thinner for removal and deletion effects; triggers and views carry rules. Tests must cover triggers directly.
- Every catalog read changes in D.
- `total_weight` changes meaning (on-your-back) for all clients from milestone A.

## Open questions

None open for the architecture. Closed on 2026-11-22: OQ-1 (private items, now BR-037, reconciled in ADR-005 and ARS-001), OQ-2 (BR-030 in A, accepted), OQ-3 (BR-023 partial status, plan note above), Q-7 (answered by the frontend design's F10; the lookup's separate `own` result and the own-code conflict response serve it).

Recorded realization choice, open to the specifier if it disagrees: an admin who sends their own private item to the catalog gets a `waiting` contribution, as BR-037 AC-5 states, rather than approved gear as BR-032 AC-4 says for admins' direct additions. The admin can approve it in the review list.
