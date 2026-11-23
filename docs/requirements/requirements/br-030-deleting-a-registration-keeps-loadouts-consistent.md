---
type: Behavioral Requirement
title: BR-030 Deleting a registration keeps the loadouts that use it consistent
description: When the owner deletes a registration, every loadout item that refers to it stays, marked as no longer in the closet and keeping its last weight; the registration stops being a bag in every loadout and its contents lose their place; all of it happens together or not at all.
tags: [inventory, user-gear, loadouts, deletion, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-030 — Deleting a registration keeps the loadouts that use it consistent

**Implementation:** implemented in milestone A (2026-11-22) (migration `000011`, tests in `pkg/api`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone A): a delete trigger and foreign-key actions detach items (snapshot name and weight kept) and remove the registration's bags in the same transaction. **Status:** Accepted. **Priority:** P1 (milestone A; needed as soon as items refer to registrations). **Basis:** AC-1 and AC-2 are sourced from the accepted design's failure case "Deleted closet item still in a loadout: the card shows 'No longer in your closet' and a Remove action. Its weight still counts until it is removed" (owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md)); which weight is kept is provisional default P-27. AC-3 applies the design's rule for removing a bag (F4: it "moves its items to Not in a bag yet … It never deletes items"). AC-4 is derived from OC-001 G-5. The copy rule is sourced from F3.

**Statement:**
When the owner deletes a registration, the API shall keep every loadout that uses it readable and consistent, without losing any item and without deleting anything other than the registration and its own links.

**Acceptance criteria:**

- AC-1: Every loadout item that refers to the deleted registration stays, with its quantity, notes, place, tick and consumable role. Reads report that its registration no longer exists, its gear name, and the effective unit weight and source the registration had when it was deleted. That kept weight counts in the on-your-back total as before ([BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md) AC-3). **[Keeping the last effective weight: provisional default P-27 — open to product-owner change]**
- AC-2: Such an item can be removed, moved, ticked, and have its quantity, notes and role changed like any other item. It is never copied ([BR-031](/docs/requirements/requirements/br-031-copy-a-loadout.md) AC-2).
- AC-3: In every loadout where the registration was a bag, it stops being a bag. Each item directly in it gets no place, and each bag directly in it becomes top level, as in [BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md) AC-8. Its closed state ends.
- AC-4: The deletion and all effects of AC-1 and AC-3 happen together or not at all. A failed or refused deletion changes no loadout ([BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md) AC-5, [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) G-5).
- AC-5: Deleting a registration never changes another user's data or the catalog.

**Out of scope:** account deletion, which removes all of the user's loadouts anyway ([BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md)).

**Verification semantics:**
Seed registration R used as an item in L1 (in a bag, ticked, measured 650 g guess) and as a bag in L2 holding two items and one nested bag. Delete R. Read L1: the item is still there with its place, tick and quantity, reports "no longer in closet", and still counts 650 g. Read L2: R is no longer a bag, its two items have no place, the nested bag is top level, and no item is missing. Remove and move the L1 item: both succeed. Copy L1: the item is left out and named. Inject a failure during the deletion: R and both loadouts are unchanged. Any item lost, or any loadout left with a place that refers to the deleted registration, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", "Failure, recovery and interruption", flows F3 and F4 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-012 Gear registrations are readable and changeable only by their owner](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)
- [BR-023 Loadout items refer to the owner's own closet registrations](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md)
