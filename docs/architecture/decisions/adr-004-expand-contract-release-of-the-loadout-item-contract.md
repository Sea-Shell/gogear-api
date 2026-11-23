---
type: Architecture Decision
title: ADR-004 Expand-then-contract release of the loadout item contract
description: How the loadout-loop changes reach production through automatic deploys (BR-022) without breaking the deployed frontend - every milestone only adds to the API contract, old gear_id item inserts keep working through a temporary legacy path, and a final cutover change removes that path and converts the remaining legacy items once the deployed frontend adds items by registration.
tags: [architecture, release, compatibility, loadouts, migration, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# ADR-004 — Expand-then-contract release of the loadout item contract

**Status:** Accepted (architect, 2026-11-22). Expand step (milestone A) implemented 2026-11-22; the cutover is not. **Amended 2026-11-22 after the critic review (Loom qa `279ad7b0`):** D-2 states that legacy items read their gear live and block its deletion (F3, F4); D-1 records the `gear_weight` 0 → `null` change and the replay suite seeds a 0-weight gear (advisory A4). Every migration merges together with its dependent code ([ARD-001](/docs/architecture/design/ard-001-loadout-planning-and-packing.md) merge rule, F1). Contract details are in [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md).

## In brief

**What the system will do:** Every merge of this work only *adds* to the API: new fields, new endpoints, new tables. Fields the deployed frontend reads keep their names and types. One old behavior is kept for a while on purpose: adding a loadout item by catalog `gear_id` alone. The API binds such an item to the caller's own registration of that gear when there is one; otherwise it stores it as a temporary *legacy item*. A last, separate API change (the *cutover*) removes that path and converts or removes the legacy items. The cutover is merged only after the deployed frontend adds items by registration.

**Why this design:** Both repositories deploy every release-worthy merge automatically (BR-022, and the frontend's own `release.yaml`). The deployed frontend (`gogear-frontend` `main` at `aabc27d`) adds items with `{loadout_id, gear_id, quantity, notes}` and reads `gear_id` from item lists and the public view. A breaking item contract would break it at the next API deploy. Expand-then-contract needs neither held merges nor a coordinated deploy, and it costs one small code path and one data conversion that BR-023 AC-7 requires anyway.

**Normal example:** API milestone A deploys. A user on the old frontend adds "Spork" from the catalog: they own a Spork registration, so the item is bound to it and is a normal item. They add "Thermos", which they do not own: it is stored as a legacy item and still shows in both frontends, with its catalog weight. Later the reworked frontend with the board is deployed; it adds items by `usergear_registration_id`. Then the cutover merges: gear-only inserts are refused with 400, and the migration binds the Thermos item if a Thermos registration now exists, otherwise removes it.

**Most important limit:** Until the cutover, BR-023 AC-1 (no item from catalog gear alone) and AC-7 (every stored item refers to a registration) are **not yet met**. The requirements index must say "partially implemented" for BR-023 in that period.

## Derived from

- [BR-022 A merge to main releases and deploys a new version with no manual step](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md); [Release and deployment decisions — RD1, RD2](/docs/anchors/release-and-deployment.md)
- [BR-023](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md) AC-1, AC-7, AC-8; [BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md) AC-5
- Requirements concern (specifier result, Loom record `59a4e354`): "BR-023 changes the item contract incompatibly and BR-022 deploys on merge"
- Evidence: deployed frontend `src/api/types.ts` and `src/components/LoadoutWorkspace.tsx`, `src/pages/MobileChecklistPage.tsx`, `src/pages/PublicLoadoutPage.tsx` at `aabc27d` (item insert by `gear_id`, item reads use `gear_id`, `total_weight` read from loadouts; `/loadout/{id}/import` not called). The `frontend-rework` branch has no code changes beyond design docs yet (same `aabc27d`), and its release plan step 1 uses today's API, that is, `gear_id` inserts.

## Satisfies

| Requirement | Satisfied by |
|---|---|
| BR-022 AC-1, AC-2 (every merge deploys, no manual step) | D-1: no merge needs to wait for a deploy window; all schema changes are migrations that run at start |
| BR-023 AC-7 (stored items converted or removed automatically) | D-2 (expand migration binds what it can) and D-3 (cutover migration binds or removes the rest) |
| BR-023 AC-8 (new contract documented in Swagger) | D-1: Swagger regenerated in every milestone; the legacy path is documented as deprecated |
| BR-023 AC-1 | D-3 (cutover) |
| BR-016 AC-5 (public view keeps working) | D-1: public item fields kept, additions only |

## Decision question

How do the incompatible loadout item changes reach production when every merge deploys automatically and the deployed frontend reads and writes items by `gear_id`?

## Options considered

**R-A — Expand, then contract (chosen).** Described above.
- Benefits: no held merges, no coordinated deploy, no broken frontend at any time; each milestone ships as soon as it is merged.
- Costs: one legacy insert path (one function also used by both migrations); a `detachedReason = 'legacy'` value; a separate small cutover change; BR-023 is only partly met until the cutover.
- Failure mode: the cutover merges while a frontend that still sends gear-only inserts is deployed. That frontend's "add to loadout" then fails with 400; everything else keeps working (reads keep `gear_id`). Guarded by D-3's precondition.

**R-B — One coordinated release.** Hold API milestone A on a branch until the reworked frontend's board is ready, then merge both close together.
- Costs: holds every milestone behind the frontend; the two repositories deploy in separate workflows, so there is still a window in which one side is new and the other old; long-lived branches conflict with other merges.
- Rejected: it does not remove the window, only shortens it, and it delays all API work.

**R-C — Break now, accept the outage.** Merge the strict contract in milestone A.
- Rejected: leaves the deployed frontend unable to add items until the reworked frontend ships. The dispatcher's constraint excludes it.

**R-D — Version the API (`/api/v2`).** Run the old and new contracts side by side.
- Rejected: duplicates every loadout route for a single client that will move anyway; more code and tests than R-A for the same outcome.

## Decision

- **D-1 — Additive contract in every milestone (A to E).** No field the deployed frontend reads is removed, renamed or retyped: `Loadout.total_weight` (now on-your-back grams, ADR-003 D-6), `LoadoutItem.gear_id` (read-only, from the registration or the detached snapshot), `quantity`, `notes`, the public item fields. Item update keeps accepting `{loadout_item_id, quantity, notes}`. New fields may be added to responses, and new optional request fields to requests. **One deliberate value change:** a catalog weight that is `0` or missing is reported as `gear_weight: null` instead of `0` from milestone A (ADR-003 D-7), in gear, registration and loadout reads. The deployed frontend tolerates `null` there (it reads weights through `?? 0` and `!= null` checks, `UserGearPage` `ensureWeight`; found by the critic review, Loom record `279ad7b0`). The replay suite must include gear with weight 0 to keep proving this. The unused `POST /loadout/{id}/import` is removed in milestone A (no client calls it; [BR-031](/docs/requirements/requirements/br-031-copy-a-loadout.md) copy replaces it), which BR-023 AC-1 allows ("any import operation that is kept").
- **D-2 — Legacy insert until the cutover.** `PUT /loadout/{id}/item/insert` with `gear_id` and without `usergear_registration_id`: the gear must exist and be visible to the caller (otherwise 404, as today). The item is bound to the caller's lowest-ID registration of that gear if one exists. Otherwise it is stored with `registrationId = NULL`, `detachedReason = 'legacy'` and `detachedGearId = gear_id`, and no snapshot. Its name, maker, category and weight are read **live** from that gear on every read, exactly as today's items are, so an admin's catalog edit shows at once. The gear cannot be deleted while a legacy item references it: the catalog delete stays 409, as it is today for gear in loadouts (ARS-001 2.1, "Gear referenced by detached items"). A request with both fields is 400. Migration 000011 applies the same binding rule to every stored item and keeps the unmatched ones as legacy items, so no existing loadout loses an item before the cutover.
- **D-3 — Cutover.** A separate change, merged when the deployed frontend no longer sends gear-only inserts. It (a) refuses gear-only inserts with 400 ("an item needs `usergear_registration_id`"), and (b) runs a migration that binds each legacy item by the D-2 rule and deletes the rest. After it, BR-023 AC-1 and AC-7 are met. **Precondition (checked when the cutover PR is reviewed, not a deploy step):** the frontend `main` that is deployed adds items only by registration. The cutover can land between any two other milestones.
- **D-4 — Legacy items behave like detached items.** They can be read, moved, ticked, re-quantified and removed; they are never copied (BR-031 AC-2: no registration). The reworked frontend sees them with `registration_exists = false` and `detached_reason = "legacy"`.

## Consequences

- The requirements index should mark BR-023 as partially implemented after milestone A, naming AC-1 and AC-7 as pending the cutover.
- The task plan needs one extra work item (the cutover) with a cross-repository dependency on the frontend board release.
- `total_weight` changes meaning for the old frontend. For items it adds (no place, no measured weight before milestone B) the value equals today's catalog sum, so nothing it shows changes until a user sets measured weights or uses Worn in the new frontend.

**Revisit trigger:** a second API client appears (then versioning, R-D, may become cheaper than per-change compatibility).

## Confirmation

- A test suite that replays the deployed frontend's exact requests (item insert by `gear_id`, item list, item update, loadout list, public view) against each milestone's build: all succeed with the fields the frontend reads. Its seed data includes gear with weight `0` (reported as `null`), a legacy item, and an attempt to delete gear that only a legacy item uses (409; the item keeps its live name, weight and `gear_id`).
- Migration 000011 test: owned-gear items bound, others legacy, none lost.
- Cutover migration test: legacy items bound or deleted; afterwards no item has `detachedReason = 'legacy'`; a gear-only insert is 400.
