---
type: Behavioral Requirement
title: BR-033 Admins approve, merge or decline catalog contributions
description: Admins see the waiting contributions and decide each once - approve (it becomes ordinary catalog gear), merge into existing gear (the contributor's registrations move to it, keeping their measured weight), or not approve with a required reason that the contributor sees.
tags: [catalog, catalog-contributions, admin-approval, admin, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-033 — Admins approve, merge or decline catalog contributions

**Amendment of 2026-11-22:** the owner accepted DC-2 (merge) on condition that everything the user provided on their own items is kept. New AC-8 states that condition, and AC-4 is no longer open to owner change. Private items ([BR-037](/docs/requirements/requirements/br-037-private-items.md)) never reach this list. Not implemented.

**Implementation:** not implemented. **Status:** Accepted with provisional conditions. **Priority:** P2 (milestone D). **Basis:** the statement and AC-1, AC-3 and AC-5 are sourced from capability need N10 and flow F8 step 5 of the accepted design ("Approve, Merge into existing… and Not approved (which requires a reason)"; owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md), [LP2](/docs/anchors/loadout-planning-and-packing.md)) and from D1 (approval by an admin). AC-4 and AC-8 are sourced from [DC-2](/docs/anchors/loadout-planning-and-packing.md), which the owner accepted on 2026-11-22 "as long as custom info provided by the use, like wheight is kept" (General's interpretation in the anchor). AC-5's "stays usable" is sourced from designer's choice DC-1. The "who added it" part of AC-1 is sourced from F8; the limit on what admins see is derived from D4 and P-3. AC-2, AC-6 and the new-maker rule of AC-4 are specified choices. AC-4 is the one place where an admin's action changes user-owned data; it is an explicit exception to P-3 and to OC-001's invariant.

**Statement:**
Admins shall be able to see every waiting contribution and decide each one once: approve it, merge it into existing catalog gear, or not approve it with a reason.

**Acceptance criteria:**

- AC-1: Only admins can list waiting contributions and decide them. A non-admin is refused with 403 and an unauthenticated caller with 401 ([BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md)). For each waiting contribution the list shows its fields ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md) AC-1), whether its maker is new, its product code, who added it, and when. It shows nothing else of the contributor's data: no registrations, measured weights, own codes or loadouts.
- AC-2: Only a waiting contribution can be decided. Deciding one that is not waiting (approved, merged, not approved, or withdrawn by a replace) is refused with 409, and nothing changes. A not-approved contribution that its contributor sends again is waiting again and can be decided (BR-034 AC-2). When two decisions on the same contribution run at the same time, exactly one takes effect; the other is refused with 409.
- AC-3: **Approve:** the contribution becomes ordinary catalog gear, and its new maker an ordinary maker. Both are visible to every user from then on ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)). The contributor's registrations and loadout items are unchanged; only their reported contribution state disappears.
- AC-4: **Merge into existing:** the admin names approved catalog gear G that is not the contribution itself. Every registration of the contribution then refers to G. Each keeps its measured weight, closet count, maximum container weight, own code and container links. Every loadout item and bag using those registrations is unchanged apart from now reporting G ([BR-023](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md) AC-5). The contribution then no longer exists as gear for anyone. Its new maker is removed unless other gear uses it. Its product code passes to G if G has none ([BR-035](/docs/requirements/requirements/br-035-codes-and-scanning.md) AC-6). If G's gear is not container-capable, a registration that is a bag keeps what it holds and receives nothing new ([BR-021](/docs/requirements/requirements/br-021-container-capability-from-category.md) AC-6, [BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md) AC-6). A G that does not exist or is not approved is refused with the "not found" outcome; naming the contribution itself is refused with 400. **[Sourced from DC-2, accepted by the owner with the condition of AC-8. Removing the unused new maker: specified choice.]**
- AC-5: **Not approved:** the admin must give a reason that is not blank; otherwise the request is refused with 400 and nothing changes. The contribution becomes "not approved", with that reason. It stays invisible to other users and stays usable by the contributor in their closet and loadouts. **[Staying usable: designer's choice DC-1, open to owner change]**
- AC-6: Each decision and all its effects happen together or not at all.
- AC-7: The contributor can see every outcome ([BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md) AC-1).
- AC-8 (added 2026-11-22, owner's condition on DC-2): A merge keeps everything the user provided on their own items. Only the gear that their registrations refer to changes. Kept on each registration: the measured weight and its source, the closet count, the maximum container weight, the own code, and its container links (as container and as content). Kept in each of the user's loadouts that use those registrations: every item's quantity, notes, place, tick and consumable role, and every bag's parent, order and closed state. Nothing of the user's data is removed. A merge that would need to lose any of these values is refused with 409 and changes nothing; no such case is known under the current requirements. Example: A's registration of contribution "Nano Puff Hoody" has measured weight 298 g weighed, count 1 and own code `GG-A-0007`, and is ticked in Worn of loadout L1; after the merge into "Nano Puff Hoody (Patagonia)" all of these are the same, and only the gear name shown changes. **[Sourced from the owner's acceptance of DC-2 in the anchor]**
- AC-9: Private items ([BR-037](/docs/requirements/requirements/br-037-private-items.md)) are never in the list of AC-1, and no decision of this requirement applies to them. A private item enters the list only when its owner sends it to the catalog.

**Out of scope:** admin edits to a waiting contribution before deciding it. After approval, admins change it like any catalog gear (BR-001).

**Verification semantics:**
Seed contributions X, Y and Z by A. As non-admin B, list and decide: 403, nothing changes. As admin C, list: each shows its fields and who added it, and nothing of A's registrations or loadouts. Approve X: B now sees X and its new maker. Merge Y into approved gear G: A's registration of Y now reports G with the same measured weight, place and tick in A's loadouts; Y no longer exists as gear; Y's unused new maker is gone; G has Y's product code if it had none. Merge into a waiting contribution: 404; into Y itself: 400. Decline Z without a reason: 400; with a reason: A sees the reason, B still cannot see Z, A's loadouts are unchanged. Decide X again: 409. Race two decisions on one contribution: exactly one takes effect. Before the merge of Y, record every value AC-8 lists for A's registrations of Y and for their items and bags in A's loadouts; after it, compare: any value lost or changed, other than the gear reported, falsifies AC-8. Add a private item as A: it never appears in C's list. Any decision that leaves partial effects, or any contributor data shown in the list, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1, LP2, DC-1, DC-2](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D1, D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N10, flow F8 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-002 Catalog contributions require admin approval](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md)
- [BR-032 Users contribute new gear to the catalog, and it is in their closet at once](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) — AC-4 is its one stated admin exception.
