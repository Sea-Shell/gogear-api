---
type: Behavioral Requirement
title: BR-031 Copy a loadout in one all-or-nothing operation
description: The owner copies one of their loadouts into a new loadout in one operation, choosing whether bags and places and whether notes and goal come along; gear and quantities always come, ticks and closed bags never; items whose registration is gone are left out and named; the original never changes.
tags: [loadouts, copy, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-031 — Copy a loadout in one all-or-nothing operation

**Implementation:** AC-1 to AC-8 are implemented in the code of milestone C (2026-11-25, written against `main` at `ba3b4f7`; no migration; tests in `pkg/api/loadout_copy_test.go`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone C): `POST /loadout/{id}/copy` takes `loadout_name`, `copy_places` and `copy_notes_and_goal` (all three required; a missing, null or wrongly typed one is 400, which is how AC-8 treats an unstated option) and answers 201 with the new loadout and `left_out`. It runs in one transaction. With `copy_places` off, no copied item has a place, an item that was in Worn included (AC-3 as written; the architecture draft said otherwise, so the point awaits confirmation, Loom question `223df704`). **No release, deployment or acceptance is claimed here.** The old `/loadout/{id}/import` was removed in milestone A. **Status:** Accepted. **Priority:** P2 (milestone C). **Basis:** the statement and AC-2 to AC-6 are sourced from anchor decision [LP4](/docs/anchors/loadout-planning-and-packing.md) and capability need N7 ("one all-or-nothing operation, with the options in F3"; owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md)); leaving out and naming items whose registration is gone is sourced from F3 step 4 and SC-004. AC-1 is derived from D4 and BR-014. AC-7 is sourced from the design's failure case "A half-made copy is never left behind". What else is or is not copied (AC-4 description, AC-5 except ticks and closed bags, AC-2 role) and AC-8 are provisional default P-28.

**Statement:**
The API shall copy one of the caller's loadouts into a new loadout of the caller in one all-or-nothing operation, with the copy options of LP4, and shall report which items it left out.

**Acceptance criteria:**

- AC-1: The source must be a loadout the caller owns. Any other source, including another user's public loadout, is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md), and nothing is created. The copy is owned by the caller ([BR-014](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md) AC-4). The caller gives the copy's name, with the same rules as for creating a loadout.
- AC-2: Always copied: every item of the source whose registration still exists, with its registration, quantity and consumable role. Each copied item is a new item of the copy; several items of one registration stay several items. Items whose registration no longer exists ([BR-030](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md)) are left out, and the response names each left-out item by its gear name. Example (SC-004): "Thermos 0.75 L" and "Z Lite Sol" are no longer in the closet; the copy has every other item, and the response names those two.
- AC-3: Option "bags and where each item goes". On: the copy has the same bags with the same nesting and order, a Last minute place if the source has one, and each copied item in the same place as in the source. Off: the copy has no bags and no Last minute, and every copied item has no place. Registrations that were bags of the source are not copied as items. A bag whose gear is no longer container-capable is copied as a bag like the others and behaves as in [BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md) AC-6.
- AC-4: Option "notes and weight goal". On: each copied item's notes, the goal and the loadout description are copied. Off: none of them are. **[Description with this option: provisional default P-28]**
- AC-5: Never copied: ticks, closed states ([LP4](/docs/anchors/loadout-planning-and-packing.md)), and also the stage (the copy is in planning), done time, trip dates, public flag (the copy is private) and slug (the copy has none). **[Everything except ticks and closed states: provisional default P-28]**
- AC-6: The source loadout, its items, places, progress and values are unchanged by the copy.
- AC-7: All or nothing: if any part of the copy fails, no new loadout exists, nothing else changes, and the response is not a success. The copy reflects the source at one moment: a change to the source made at the same time is either fully in the copy or not in it at all.
- AC-8: The caller states each option. How an option that is not stated is treated is a realization choice that must be documented in the API description. A response that succeeds identifies the new loadout.

**Verification semantics:**
Seed L1 with three bags (one nested), Last minute, ten items in various places (two of the same registration), notes, a goal, a description, ticks, two closed bags, stage done, dates, public with a slug. Delete two registrations used in L1. Copy with both options on: the copy has eight items in the same places, the same bags and nesting and order, notes, goal and description, no ticks, nothing closed, stage planning, no dates, private, no slug; the response names the two left-out items. Copy with both options off: eight items, no places, no bags, no notes, goal or description. Compare L1 before and after: unchanged. Copy another user's loadout: 404, nothing created. Inject a failure after the first copied item: no new loadout exists. Any partial copy, or any change to the source, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1, LP4](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N7, flow F3, scenario SC-004 (frontend repository, `docs/design/`)

## Related

- [BR-014 Loadouts are readable and changeable only by their owner](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md)
- [BR-025 Packing progress is saved per loadout and shared across devices](/docs/requirements/requirements/br-025-packing-progress-saved-per-loadout.md)
