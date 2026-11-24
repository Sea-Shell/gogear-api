---
type: Behavioral Requirement
title: BR-036 A registration records how many the owner has
description: Each closet registration has a count of identical items the owner has (at least 1, default 1); weights stay per unit; a loadout may use more than the count, which is reported so the client can warn, never refused.
tags: [inventory, user-gear, closet, loadouts, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-036 — A registration records how many the owner has

**Implementation:** AC-1 to AC-5 are implemented in the code of milestone B (2026-11-24; migration `000012`, tests in `pkg/api/registration_weights_test.go` and `migration12_test.go`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone B): `closet_count` is stored with default 1, written by `PUT /usergear/insert` and `POST /usergear/registration/:usergear/update`, and reported by registration reads and loadout item reads. The part of AC-1 that concerns the registration created with a contribution waits for milestone D. **No release, deployment or acceptance is claimed here.** **Status:** Accepted with provisional conditions. **Priority:** P1 (milestone B, with BR-026; the closet count column is part of migration `000012`, not `000011`). **Basis:** the statement and AC-1 are sourced from the owner's answer to Q-B ("perhaps a item-count in the closet should exist as well?"), interpreted by General as a count of how many the user owns ([LP6](/docs/anchors/loadout-planning-and-packing.md), interpretation open to owner correction). AC-3 (warning, not refusal) follows the accepted design's principle P3 ("never block, always show") and is provisional default P-19. AC-2, AC-4 and AC-5 are specified choices.

**Terms:**
- *Closet count*: how many identical items one registration stands for. Example: one registration "Tent peg, MSR Groundhog" with count 8.
- *Use* of a registration in a loadout: the sum of the quantities of the loadout's items that refer to it, plus 1 if it is one of the loadout's bags.

**Statement:**
Each registration shall record how many identical items the owner has, and the API shall report it wherever it reports the registration, without refusing a loadout that uses more.

**Acceptance criteria:**

- AC-1: A registration's count is a whole number of at least 1. A new registration has count 1 unless the request gives one, including the registration created with a contribution ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)). The owner can change it; any other value is refused with 400. Only the owner can read or change it ([BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)). When this requirement takes effect on a database, every existing registration gets count 1 automatically, with no manual step ([BR-022](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md)).
- AC-2: The count never multiplies a weight. Catalog and measured weights are per unit ([BR-026](/docs/requirements/requirements/br-026-measured-weight-on-a-registration.md)); in a loadout only the item quantity multiplies them ([BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md) AC-3).
- AC-3: A loadout may use a registration more than its count. Adding or changing items, making it a bag, copying a loadout, and lowering the count below the current use are never refused for that reason. Reads report the count with each item and registration (BR-029 AC-1, AC-6), so the client can warn, for example "8 pegs planned, you have 6". **[Warning, not refusal: provisional default P-19 — open to product-owner change]**
- AC-4: The count does not change the closet container rules. A registration with count 8 is still one registration with at most one direct container ([BR-020](/docs/requirements/requirements/br-020-container-nesting-consistency.md)), and at most one bag per loadout ([BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md) AC-2).
- AC-5: Registering the same gear several times stays possible, as today. The count does not merge registrations.

**Verification semantics:**
Create a registration without a count: count 1. Set 8, then 0 and 2.5: 0 and 2.5 are 400. Add two items of it with quantity 4 and 5 to one loadout: both accepted, and reads report count 8 with each item. Lower the count to 6: accepted. Check that the loadout total uses quantity × unit weight, not the count. As another user, change the count: 404. Run the release on a database with existing registrations: every one has count 1. A refusal caused only by using more than the count falsifies AC-3.

## Derived from

- [Loadout planning and packing decisions — LP6](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", principle P3 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-023 Loadout items refer to the owner's own closet registrations](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md)
- [BR-012 Gear registrations are readable and changeable only by their owner](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)
