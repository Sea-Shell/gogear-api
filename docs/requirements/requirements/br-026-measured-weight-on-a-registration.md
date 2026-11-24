---
type: Behavioral Requirement
title: BR-026 The owner can record their own weight on a registration, with its source
description: A registration can carry the owner's unit weight in whole grams, marked as weighed or as a guess; it overrides the catalog weight in every loadout using the registration and never changes the catalog.
tags: [inventory, user-gear, weight, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-026 — The owner can record their own weight on a registration, with its source

**Implementation:** AC-1 to AC-4 are implemented in the code of milestone B on branch `loadout-loop-milestone-b` (2026-11-24; migration `000012`, tests in `pkg/api/registration_weights_test.go` and `migration12_test.go`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone B): `PUT /usergear/insert` and `POST /usergear/registration/:usergear/update` take `measured_weight` with `measured_weight_source`, registration reads and loadout reads report them, and every loadout uses the effective weight. **Not merged to `main`, not released, not deployed, and not accepted.** AC-5 waits for milestone D (merge and replace). Before this branch only the catalog `gear_weight` existed; a registration update changed only `max_container_weight`. **Status:** Accepted. **Priority:** P1 (milestone B). **Basis:** the statement, AC-1, AC-3 and AC-4 are sourced from capability need N5 of the accepted design ("your weight on a registration, with its source (weighed or guess)"; owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md)) and its weighing flow F6 ("Every loadout using that registration updates. The catalog weight is never changed by a user."). AC-2 is sourced from D4. AC-5 is sourced from [DC-2](/docs/anchors/loadout-planning-and-packing.md), accepted by the owner on condition that weights are kept, and extended to the contributor's replace by provisional default P-31. AC-1's value range is provisional default P-25.

**Terms:**
- *Measured weight*: the owner's own weight for one unit of a registration, with a source: **weighed** (shown as "weighed by you") or **guess** ("your guess").
- *Catalog weight*: the gear record's weight (shown as "maker's spec"). For a user's own contribution it is the weight given when adding it ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)).
- *Effective unit weight* and *weight source* of a registration: its measured weight and that source if it has one; otherwise its catalog weight, source "maker's spec"; otherwise none, source "no weight".

**Statement:**
The API shall let the owner of a registration set, change and remove a measured unit weight with its source, use it in place of the catalog weight wherever that registration's weight counts, and never change catalog data because of it.

**Acceptance criteria:**

- AC-1: The owner can set a measured weight as a whole number of grams, at least 1, together with its source, weighed or guess. A value without a source, a source without a value, any other source, a value below 1 or a fraction is refused with 400 and changes nothing. The owner can change both, or remove the measured weight; afterwards the registration's effective unit weight is its catalog weight again. **[Whole grams ≥ 1: provisional default P-25 — open to product-owner change]**
- AC-2: Only the registration's owner can set, change, remove or read it ([BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)). Anyone else gets the "not found" outcome and nothing changes. Admins get no exception (P-3).
- AC-3: Setting, changing or removing a measured weight never changes the catalog weight, any other registration, or any other user's data. Two users who own the same catalog gear each keep their own measured weight.
- AC-4: The measured weight is the registration's, not a loadout's. Every loadout that uses the registration reports the new effective unit weight on its next read, with no further action ([BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md) AC-2). The only exception is an item whose registration was deleted ([BR-030](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md)).
- AC-5: The measured weight stays on the registration when the registration's gear changes through an admin merge ([BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md), DC-2) or a contributor's replace ([BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md), P-31), and so does its source (BR-033 AC-8). Sending a private item to the catalog never changes it ([BR-037](/docs/requirements/requirements/br-037-private-items.md) AC-5).

**Out of scope:** the sanity check for unlikely values (F6 step 5) is the client's; the API accepts any value allowed by AC-1. Weight tolerances and uncertainty are computed by the client.

**Verification semantics:**
Use real request handling and stored data. As A, set 687 g "weighed" on a registration used in two loadouts: both loadouts report 687 g and source weighed on their next read; the catalog gear and B's registration of the same gear are unchanged. Change to 650 g "guess", then remove it: the loadouts report 650 g guess, then the catalog weight with source maker's spec. Send 0, 12.5, a value without source and source "measured": 400, nothing changes. As B, set A's measured weight: 404, unchanged. Merge the registration's contribution into existing gear: the measured weight is still 687 g. Any change to catalog data, or any loadout that still reports the old weight after a successful change, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1, DC-2](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D1, D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N5, weight model, flow F6 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-012 Gear registrations are readable and changeable only by their owner](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)
- [BR-029 Loadouts report every weight input and a current on-your-back total](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md)
