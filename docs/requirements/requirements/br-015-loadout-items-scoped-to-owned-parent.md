---
type: Behavioral Requirement
title: BR-015 Loadout items are reachable only through their own, owned loadout
description: Loadout item operations succeed only when the item belongs to the loadout in the request path and that loadout is owned by the caller; refused operations change no item and no loadout total.
tags: [loadouts, loadout-items, ownership, authorization, security-remediation]
timestamp: 2026-11-22T00:00:00Z
---

# BR-015 — Loadout items are reachable only through their own, owned loadout

**Amendment of 2026-11-22 (loadout planning, [LP1](/docs/anchors/loadout-planning-and-packing.md), [LP3](/docs/anchors/loadout-planning-and-packing.md)):** new AC-6 (an item's registration must be the caller's own, [BR-023](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md)) and AC-7 (places, bags and packing progress are reachable only through their own, owned loadout, [BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md), [BR-025](/docs/requirements/requirements/br-025-packing-progress-saved-per-loadout.md)). AC-1 to AC-5 are unchanged; "item operation" in AC-4 now also covers moves, ticks and role changes. Not implemented.

**Implementation:** implemented in wave 2 (loadout items); AC-6 and AC-7 implemented in milestone A (2026-11-22, registration, place and progress checks in the same operation, `pkg/api/loadout_items_test.go`, `pkg/api/loadout_places_test.go`), with adversarial tests in `pkg/api/loadout_authz_test.go`. Item update and delete bind item ID, parent loadout and owner in one statement; the mutation and the total-weight recalculation share one transaction. Quantity must be at least 1 (400 otherwise); an absent quantity on insert means 1. Wave 6: the database refuses a quantity that is not a positive integer (legacy rows of 0 or below were clamped to 1 and audited), and an item insert naming gear that is not in the catalog is 404. **Status:** Accepted. **Priority:** P0 (review SEC-04). **Basis:** sourced from anchor decision D4 (parent/child membership). AC-4 is derived from it. AC-5 is a provisional default.

**Statement:**
The API shall list, add, update and remove loadout items only through a loadout the caller owns. An update or removal shall succeed only if the addressed item belongs to that loadout.

**Acceptance criteria:**

- AC-1: Listing or adding items through a loadout the caller does not own is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). No item is created.
- AC-2: Updating or removing an item is refused with the "not found" outcome unless both of these hold: (a) the caller owns the loadout named in the path, and (b) the item belongs to that loadout.
- AC-3: All pairings behave as follows. Own loadout and own item: allowed. Own loadout and foreign item: refused. Foreign loadout and the item of that foreign loadout: refused. Own loadout and an item of another of the caller's own loadouts: refused, because the item is not a member of the addressed loadout.
- AC-4: A refused item operation changes no item and no loadout's stored total weight. This covers both the loadout in the path and the loadout the item really belongs to.
- AC-5: Admins get no exception. **[Provisional default P-3 — open to product-owner change]**
- AC-6 (added 2026-11-22): Adding an item, or changing which registration an item or bag refers to, succeeds only if the registration is owned by the loadout's owner. Otherwise it is refused with the "not found" outcome and nothing changes. (Derived from D4: a loadout must not take in another user's inventory.)
- AC-7 (added 2026-11-22): A loadout's bags, its Last minute and Worn places, and its packing progress are parts of that loadout. Reading or changing them succeeds only through a loadout the caller owns, and only for parts that belong to that loadout. Example: moving an item of L1 into a bag of L2, or nesting a bag of L1 inside a bag of L2, is refused with the "not found" outcome, also when the caller owns both loadouts.

**Verification semantics:**
Seed loadouts L1 and L2 owned by A, and L3 owned by B, each with items. Run every pairing in AC-3 for update and delete. Record the items and the total weight of all three loadouts before and after each attempt. The requirement is falsified by any refused attempt that changes an item or a total, or any attempt in a refused pairing that succeeds.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- [Loadout planning and packing decisions — LP1, LP3](/docs/anchors/loadout-planning-and-packing.md) (AC-6, AC-7)

## Related

- [BR-014 Loadouts are readable and changeable only by their owner](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
