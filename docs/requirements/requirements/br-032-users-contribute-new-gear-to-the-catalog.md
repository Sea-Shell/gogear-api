---
type: Behavioral Requirement
title: BR-032 Users contribute new gear to the catalog, and it is in their closet at once
description: A non-admin adds new gear (with a new maker if needed) as a contribution that waits for admin approval; the same operation puts a registration of it in the contributor's closet, with their measured weight if they weighed it; admins' additions go straight into the catalog.
tags: [catalog, catalog-contributions, admin-approval, inventory, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-032 — Users contribute new gear to the catalog, and it is in their closet at once

**Implementation:** not implemented. **Status:** Accepted. **Priority:** P2 (milestone D). **Basis:** the statement and AC-1 to AC-3 are sourced from anchor decision [LP2](/docs/anchors/loadout-planning-and-packing.md), capability need N10 and the design's flow F8 ("Add to catalog and my closet … appears at once in your closet"; the form fields; "I weighed it" becomes the catalog weight and your measured weight). AC-4 is sourced from F8 step 5 and BR-001. AC-5 is derived from D1: LP2 opens adding new gear only. AC-6 is a specified choice. The field rules (required fields, top category from the sub-category, optional weight) are provisional default P-29. Users may instead add a private item that only they see ([BR-037](/docs/requirements/requirements/br-037-private-items.md), LP8) and send it to the catalog later; it then follows this requirement's rules from AC-3 on.

**Terms:**
- *Contribution*: a gear item a non-admin added to the catalog. Until an admin decides, it is *waiting*. Its other states are in [BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md) and [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md).
- *Unapproved contribution*: a contribution that is waiting or not approved.
- *Contributor*: the user who added the contribution.
- *New maker*: a maker that does not exist yet, named in a contribution.

**Statement:**
A non-admin user shall be able to add new gear to the catalog as a contribution. In the same operation, the API shall put a registration of that gear in the contributor's closet, and the contribution shall wait for admin approval.

**Acceptance criteria:**

- AC-1: A contribution has: a name (required, not blank); a maker (required: an existing maker, or the name of a new maker); a sub-category (required, an existing one; the gear's top category is that sub-category's top category); a catalog weight in whole grams of at least 1 (optional) with its source, "the tag or the maker" or "I weighed it" (required if a weight is given); a size or variant (optional); and a product code (optional, [BR-035](/docs/requirements/requirements/br-035-codes-and-scanning.md)). A request that breaks these rules is refused with 400, and nothing is stored. **[Field rules: provisional default P-29 — open to product-owner change]**
- AC-2: One operation creates the waiting contribution and one registration of it owned by the contributor, both or neither. The weight, if given, becomes the contribution's catalog weight. If its source is "I weighed it", the new registration also gets that value as its measured weight with source weighed ([BR-026](/docs/requirements/requirements/br-026-measured-weight-on-a-registration.md)). The response identifies both.
- AC-3: A new maker is created together with the contribution. Like the contribution, it is visible only to the contributor until an admin approves a contribution that uses it ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)).
- AC-4: Gear an admin adds is not a contribution. It is approved at once, through any path, as today ([BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md)).
- AC-5: Contributing, directly or by sending a private item to the catalog (BR-037 AC-5), is the only way a non-admin adds to the catalog. Non-admins still cannot create categories or top categories, and cannot change or delete any approved catalog record ([BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) AC-7).
- AC-6: The API does not refuse a contribution because its name looks like existing gear. Avoiding duplicates is the client's job, using catalog search (F8 duplicate check); an admin repairs duplicates by merging (BR-033). (Specified choice.)
- AC-7: The new registration is an ordinary registration of the contributor ([BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)). The contribution's container capability follows its sub-category ([BR-021](/docs/requirements/requirements/br-021-container-capability-from-category.md) AC-3).

**Verification semantics:**
As non-admin A, contribute gear with a new maker, an existing sub-category, 412 g "I weighed it" and a product code: one waiting contribution, one registration owned by A with measured weight 412 g weighed, the catalog weight 412 g, the top category taken from the sub-category. Repeat without a name, with an unknown sub-category, and with a weight but no source: 400, nothing stored. Make the registration step fail: no contribution exists either. As B, list makers and gear: neither the new maker nor the gear appears. As admin C, add gear the ordinary way: it is visible to B at once. Any contribution stored without its registration, or visible to B before approval, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP2](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D1](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N10, flow F8 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-002 Catalog contributions require admin approval](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md)
- [BR-033 Admins approve, merge or decline catalog contributions](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md)
- [BR-034 Contributors see outcomes, and can edit, resubmit or replace their contributions](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md)
- [OC-002 Visibility of gear outside the shared catalog](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)
