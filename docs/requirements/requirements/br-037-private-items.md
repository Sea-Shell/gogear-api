---
type: Behavioral Requirement
title: BR-037 Users can add private items that only they see, and send them to the catalog later
description: Any user can add gear only for themselves (a private item, with a new maker if needed) together with a closet registration of it; only the owner sees and uses it, it never reaches the admin queue or public views, and sending it to the catalog turns it into a waiting contribution without changing the owner's closet or loadouts.
tags: [catalog, private-items, catalog-contributions, inventory, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-037 — Users can add private items that only they see, and send them to the catalog later

**Implementation:** not implemented. **Status:** Accepted with provisional conditions. **Priority:** P2 (milestone D, with the contribution flow). **Basis:** the statement and AC-2, AC-3, AC-5 and AC-6 are sourced from anchor decision [LP8](/docs/anchors/loadout-planning-and-packing.md) (the owner chose "private items for everyone; can be sent to the catalog later" and "drop private items when its shared"; General's interpretation, open to owner correction). AC-1 mirrors the contribution rules of [BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md) (provisional default P-35). AC-4 and AC-7 are provisional default P-36. AC-8 is derived from D1 and LP8. The client decides how a user chooses between a private item and a contribution when adding gear; that is frontend design (F8 needs a "keep it private" choice).

**Terms:**
- *Private item*: a gear record a user added only for themselves. Its *owner* is that user. It is not part of the shared catalog.
- *Private maker*: a new maker named in a private item. Only the item's owner sees it.
- *Send to the catalog*: the owner's request that turns a private item into a contribution.

**Statement:**
Any user shall be able to add a private item that only they can see and use. The API shall keep it out of the shared catalog, the admin queue and public views, until the owner sends it to the catalog as a contribution.

**Acceptance criteria:**

- AC-1: A private item has the same fields, with the same rules, as a contribution ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md) AC-1): name, maker (existing or new), sub-category, optional weight with source, size or variant, and product code. One operation creates the private item and one registration of it owned by the caller, both or neither. A weight with source "I weighed it" also becomes that registration's measured weight. Any user can add one, admins included. A request that breaks the rules is refused with 400, and nothing is stored. **[Field rules and the paired registration: provisional default P-35 — open to product-owner change]**
- AC-2: Only its owner can see a private item or its private maker. To everyone else, admins included, they behave exactly as if they did not exist, in every operation ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)). A private item is never in the admins' list of contributions ([BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md)), and no admin decision applies to it.
- AC-3: The owner can use a private item like catalog gear: register it more than once, use its registrations in loadouts and as bags if its sub-category is a container category ([BR-021](/docs/requirements/requirements/br-021-container-capability-from-category.md)), set measured weights, counts and own codes, and find it by its product code ([BR-035](/docs/requirements/requirements/br-035-codes-and-scanning.md)). Wherever the API reports it to the owner, it says the item is private.
- AC-4: The owner can change a private item's fields under the rules of AC-1. A changed catalog weight changes no measured weight. Deleting a private item is not required. A private item with no registrations left stays, and stays private. **[Provisional default P-36 — open to product-owner change]**
- AC-5: The owner can send a private item to the catalog. It then becomes a waiting contribution of that owner, with the same fields, and its private maker becomes the contribution's new maker. From then on [BR-002](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) and BR-032 to [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md) apply: admins see it in their list, and other users see it only after approval. Sending changes nothing in the owner's registrations or loadouts. Nobody but the owner can send it; anyone else gets the "not found" outcome.
- AC-6: A public loadout view never shows an item whose gear is a private item, or any detail of that gear ([BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md) AC-6, OC-002 G-5). Its weight may still count in the total. (Sourced from LP8: "drop private items when its shared".)
- AC-7: Sending is one way. Once a private item is a contribution, it never becomes private again. A contribution that is not approved stays usable by its contributor, as DC-1 says, but it is not a private item. **[Provisional default P-36]**
- AC-8: A private item is not catalog data. Adding, changing or sending it is not a catalog change under [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md), and it never changes catalog data that other users read. (Derived from D1 and LP8.)
- AC-9: Deleting the owner's account removes their private items and private makers ([BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md) AC-8).

**Out of scope:** the share-time question the owner mentioned in LP8 (for example offering to send a loadout's private items to the catalog when it is shared). That is future frontend design, and it needs nothing from the API beyond AC-5.

**Verification semantics:**
As A, add private item P with a new maker M and 120 g "I weighed it": P, M and one registration owned by A exist, and the registration's measured weight is 120 g weighed. As B and as admin C, read every catalog list, search, get, maker list and code lookup, try to register P, and read the admins' contribution list: P and M never appear, single reads give the same response as unknown IDs, and registering P gives the unknown-gear outcome. As A, use P in a loadout as an item and, in a container category, as a bag: both succeed, and P is reported as private. Make that loadout public and read it without a token: P's item is not listed and no detail of P appears. As A, send P to the catalog: C now sees P as a waiting contribution; B still does not; A's registrations and loadouts are unchanged. Try to make it private again: refused. As B, send A's private item: 404. Delete A's account: P and M are gone. Any appearance of a private item to anyone other than its owner falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP8, LP2](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D1, D4](/docs/anchors/access-control-and-identity.md)

## Related

- [OC-002 Visibility of gear outside the shared catalog](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)
- [BR-032 Users contribute new gear to the catalog, and it is in their closet at once](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)
- [BR-016 Public loadouts are readable by slug without signing in; private ones are not](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md)
