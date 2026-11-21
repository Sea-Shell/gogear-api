---
type: Behavioral Requirement
title: BR-015 Loadout items are reachable only through their own, owned loadout
description: Loadout item operations succeed only when the item belongs to the loadout in the request path and that loadout is owned by the caller; refused operations change no item and no loadout total.
tags: [loadouts, loadout-items, ownership, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-015 — Loadout items are reachable only through their own, owned loadout

**Status:** Accepted. **Priority:** P0 (review SEC-04). **Basis:** sourced from anchor decision D4 (parent/child membership). AC-4 is derived from it. AC-5 is a provisional default.

**Statement:**
The API shall list, add, update and remove loadout items only through a loadout the caller owns. An update or removal shall succeed only if the addressed item belongs to that loadout.

**Acceptance criteria:**

- AC-1: Listing or adding items through a loadout the caller does not own is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). No item is created.
- AC-2: Updating or removing an item is refused with the "not found" outcome unless both of these hold: (a) the caller owns the loadout named in the path, and (b) the item belongs to that loadout.
- AC-3: All pairings behave as follows. Own loadout and own item: allowed. Own loadout and foreign item: refused. Foreign loadout and the item of that foreign loadout: refused. Own loadout and an item of another of the caller's own loadouts: refused, because the item is not a member of the addressed loadout.
- AC-4: A refused item operation changes no item and no loadout's stored total weight. This covers both the loadout in the path and the loadout the item really belongs to.
- AC-5: Admins get no exception. **[Provisional default P-3 — open to product-owner change]**

**Verification semantics:**
Seed loadouts L1 and L2 owned by A, and L3 owned by B, each with items. Run every pairing in AC-3 for update and delete. Record the items and the total weight of all three loadouts before and after each attempt. The requirement is falsified by any refused attempt that changes an item or a total, or any attempt in a refused pairing that succeeds.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-014 Loadouts are readable and changeable only by their owner](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
