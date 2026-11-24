---
type: Behavioral Requirement
title: BR-027 Each loadout item can be marked as a consumable for this loadout
description: A loadout item carries a consumable role (food, water, fuel) that is off by default, belongs to that item in that loadout only, and lets the client compute base weight as on your back minus consumables.
tags: [loadouts, loadout-items, weight, consumables, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-027 — Each loadout item can be marked as a consumable for this loadout

**Implementation:** AC-1 to AC-3 and AC-5 are implemented in the code of milestone B on branch `loadout-loop-milestone-b` (2026-11-24; migration `000012`, tests in `pkg/api/loadout_stage_test.go`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone B): item insert and update take `consumable`, every item read reports it, and the role belongs to the loadout item. **Not merged to `main`, not released, not deployed, and not accepted.** AC-4 (a copy keeps the role) waits for milestone C and is not tested yet. **Status:** Accepted. **Priority:** P1 (milestone B). **Basis:** the statement and AC-1 to AC-3 are sourced from capability need N4 of the accepted design ("Consumable role per loadout item"; owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md)) and its weight model ("Base weight = on your back minus consumables"). AC-2's "per item, not per registration" is derived from N4 ("per loadout item"). AC-4 is a specified choice. Consumables may be shared catalog gear or the owner's private items ([BR-037](/docs/requirements/requirements/br-037-private-items.md), LP8); the role works the same for both.

**Terms:**
- *Consumable*: an item whose role in this loadout is food, water or fuel. The interface groups them as "Food · water · fuel" and does not distinguish the three.

**Statement:**
The API shall store, for each loadout item, whether it is a consumable in that loadout.

**Acceptance criteria:**

- AC-1: A new item is not a consumable. The owner can switch the role on and off. Reads of the loadout report the role of every item.
- AC-2: The role belongs to the item, not to its registration or gear. Changing it changes no other item, including another item of the same registration in the same loadout ([BR-023](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md) AC-2) and items in other loadouts. Example: "Water bottle 1 L" is a consumable in L1 (filled) and not in L2.
- AC-3: Bags have no consumable role. The role does not change an item's place, tick or weight.
- AC-4: A copied item keeps its role ([BR-031](/docs/requirements/requirements/br-031-copy-a-loadout.md) AC-2). (Specified choice: the role describes what the item is on this kind of trip, not packing progress; dropping it would silently change the copy's base weight.) **[Provisional default P-28]**
- AC-5: Only the loadout's owner can read or change the role ([BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md)).

**Verification semantics:**
Add two items of the same registration to L1 and one to L2. Mark one L1 item as consumable: only that item reports the role; the other two are unchanged, and their places, ticks and weights are unchanged. Switch it off again. Copy L1: the copied item keeps its role. Change the role as another user: 404. An item whose role changed without a request for that item falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1](/docs/anchors/loadout-planning-and-packing.md)
- Accepted frontend design "Loadout planning and packing", capability need N4, weight model (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-029 Loadouts report every weight input and a current on-your-back total](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md)
