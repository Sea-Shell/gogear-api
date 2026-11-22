---
type: Obligation Contract
title: OC-002 Visibility of gear outside the shared catalog (unapproved contributions and private items)
description: Shared guarantee that every operation reading or referencing gear or makers applies one rule - gear is visible to a user only if it is approved or the user added it (an unapproved contribution or a private item) - so such gear never reaches other users through any read, lookup, reference or aggregate.
tags: [catalog, catalog-contributions, private-items, visibility, authorization, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# OC-002 — Visibility of gear outside the shared catalog (unapproved contributions and private items)

**Amendment of 2026-11-22 (anchor decision [LP8](/docs/anchors/loadout-planning-and-packing.md)):** private items exist ([BR-037](/docs/requirements/requirements/br-037-private-items.md)). G-1, G-3 to G-5, the failure semantics, the invariant and the verification now cover them under the same rule. The title is broader; the file name and ID are kept. Not implemented.

**Implementation:** not implemented. **Status:** Accepted. **Priority:** P2 (milestone D; it must hold from the first contribution). **Basis:** the visibility rule is sourced from anchor decision [LP2](/docs/anchors/loadout-planning-and-packing.md) ("others see it only after approval") and D1 / [BR-002](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) AC-1. G-2 to G-5 are derived from it. Private items are covered by LP8 ("drop private items when its shared") and BR-037.

**Why a contract:** Many independently written operations read or reference catalog gear: catalog list, search and get, maker and category lists, code lookup ([BR-035](/docs/requirements/requirements/br-035-codes-and-scanning.md)), registration create, merge and replace targets, loadout reads and the public loadout view. Each one must apply the same meaning of "visible". One operation that forgets it publishes a contribution before approval, or a private item at all.

**Participants (by responsibility):**
- *Authenticated identity*: who is asking (as in [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) G-1).
- *Catalog read or reference*: any operation that returns gear or makers, or accepts a gear or maker identifier.
- *Contribution lifecycle*: submission, approval, merge, decline, resubmission and replace ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md) to [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md)), and sending a private item to the catalog ([BR-037](/docs/requirements/requirements/br-037-private-items.md) AC-5).

**Guarantees:**
- G-1: Gear is *visible* to user U if and only if it is approved, or U added it and it is an unapproved contribution or a private item. A maker is visible to U if and only if it is approved (an existing maker, or a new maker of an approved contribution), or it is the new maker of U's own unapproved contribution or private item. Being an admin does not make gear visible, except for waiting contributions in the review operations of [BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md). A private item is never visible to an admin, not even there. **[That the contributor also sees their unapproved contribution in catalog lists, searches and lookups, not only in their closet and loadouts: provisional default P-30 — open to product-owner change]**
- G-2: Gear or a maker that is not visible to the caller behaves exactly as if it did not exist. A single read gives the same "not found" status and body as an unknown ID. Lists, searches, lookups, filters and counts leave it out. Example: B's catalog list for sub-category "Insulation" has the same entries and count before and after A contributes "Nano Puff Hoody" to it.
- G-3: A write that names gear or a maker not visible to the caller treats it as unknown. Examples: B registering A's waiting contribution or A's private item gets the same outcome as for an unknown gear ID, and no registration is created; a merge or replace target must be approved gear.
- G-4: Visibility is decided within the operation that reads or uses the gear. An approval, merge, decline, replace or send to the catalog that runs at the same time is either fully before the operation or fully after it. A merged or withdrawn contribution is visible to nobody afterwards.
- G-5: The public loadout view ([BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md)) is read by anyone, so it shows only approved gear. An item whose gear is an unapproved contribution or a private item is left out of its item list. Its weight may still count in the reported total, which reveals no gear detail.

**Semantic input/output:**
- Input: the caller (or no caller, for the public view), the operation, and the gear or maker identifiers involved.
- Output: results that contain only gear and makers visible to the caller, or "not found" / "unknown" for those that are not.

**Failure semantics:**
- A not-visible contribution or private item is never distinguishable from a nonexistent one by status, body, list position or count.
- A failed visibility check never falls back to showing the gear.

**Invariant:**
- No response to anyone other than the contributor contains the name, maker, product code, weight, category, size or existence of an unapproved contribution, or the name of its new maker, except the admin review list of BR-033 for waiting contributions.
- No response to anyone other than its owner contains any of these values, or the existence, of a private item or its private maker.

**Verification semantics:**
Exercise the real request handling and stored data. As A, contribute X with new maker M and product code R. As B (and as admin C outside the review list), call every catalog list, search, get, maker list, category list, code lookup, registration create with X, and the public view of A's public loadout that uses X: X and M never appear, counts are unchanged, single reads of X and M return the same response as unknown IDs, and registering X gives the unknown-gear outcome. As A, the same reads show X and M, marked as waiting. Approve X: B now sees X and M. Merge another contribution Y: nobody can read Y afterwards. Race approvals with B's reads: each read shows X either fully or not at all. Repeat with a private item P of A, and also check the admin review list: P never appears to B or C. Send P to the catalog: C sees it in the review list, B still does not. Any appearance of an unapproved contribution to someone other than its contributor (outside the review list), or of a private item to someone other than its owner, falsifies the contract.

## Derived from

- [Loadout planning and packing decisions — LP2, LP8](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D1](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-037 Users can add private items that only they see, and send them to the catalog later](/docs/requirements/requirements/br-037-private-items.md)
- [BR-002 Catalog contributions require admin approval](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
