---
type: Behavioral Requirement
title: BR-034 Contributors see outcomes, and can edit, resubmit or replace their contributions
description: The contributor can list their own contributions with their outcome (waiting, approved, merged into which gear, not approved with the reason); while a contribution is unapproved they can edit it, send it again, or replace it with existing catalog gear in every registration and loadout.
tags: [catalog, catalog-contributions, inventory, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-034 — Contributors see outcomes, and can edit, resubmit or replace their contributions

**Implementation:** not implemented. **Status:** Accepted with provisional conditions. **Priority:** P2 (milestone D). **Basis:** AC-1 is sourced from flow F8 step 4 of the accepted design ("Outcomes, each shown to you as a notice"; the merged notice names the existing item) and from capability need N10 (a reason "which the contributor can see"). AC-2 and AC-3 are sourced from designer's choice [DC-1](/docs/anchors/loadout-planning-and-packing.md) ("Edit and send again", "Replace with a catalog item…, which swaps it in every loadout"), open to owner change. Allowing them also while waiting, and keeping the measured weight on replace, is provisional default P-31. AC-4 to AC-6 are specified choices.

**Statement:**
The contributor shall be able to see the outcome of each of their contributions and, while a contribution is unapproved, edit it, send it again, or replace it with existing catalog gear.

**Acceptance criteria:**

- AC-1: The contributor can list their own contributions. Each shows its name as contributed, its state and when that state was reached: waiting; approved; merged, with the gear it was merged into; or not approved, with the admin's reason. A contribution the contributor withdrew by replacing it (AC-3) need not be listed. Wherever the API reports an unapproved contribution's gear to its contributor (catalog reads, registrations, loadout items), it reports the state, and the reason when not approved.
- AC-2: While a contribution is unapproved, the contributor can change its fields under the rules of [BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md) AC-1. A changed catalog weight changes no measured weight. The contributor can send a not-approved contribution again, with or without changes; it is then waiting, and the earlier reason is no longer its current reason. Editing a waiting contribution keeps it waiting. **[Editing while waiting: provisional default P-31]**
- AC-3: While a contribution is unapproved, the contributor can replace it with approved catalog gear G. Every one of their registrations of it then refers to G, with the same effects as an admin merge ([BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md) AC-4 and AC-8), keeping everything the user provided, including each measured weight. The contribution is then withdrawn: it no longer exists as gear and leaves the admins' list. A G that does not exist or is not approved is refused with the "not found" outcome. **[Allowed while waiting, and keeping the measured weight: provisional default P-31 — open to product-owner change]**
- AC-4: Once a contribution is approved or merged, the contributor can no longer edit, send or replace it; such a request is refused with 409. Approved gear is changed only by admins ([BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md)).
- AC-5: Nobody but the contributor can list, edit, send or replace a contribution through these operations; anyone else gets the "not found" outcome ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)), admins too. Admins act on contributions only through BR-033.
- AC-6: If a contributor's request and an admin's decision on the same contribution run at the same time, one is decided first and the other is judged against the result. Example: the admin approves while the contributor replaces; if the approval comes first, the replace is refused with 409.
- AC-7: Deleting every registration of an unapproved contribution does not withdraw it. (Specified choice; the contributor withdraws by replacing.)

**Out of scope:** remembering which outcomes the contributor has already seen. Showing each outcome once is the client's concern.

**Verification semantics:**
As A, contribute X, Y and Z. Have an admin approve X, merge Y into G, and decline Z with a reason. List A's contributions: X approved, Y merged into G (with Y's contributed name), Z not approved with the reason. Edit Z and send it again: Z is waiting and the admins see it again. Edit a waiting contribution's weight: its catalog weight changes, A's measured weight does not. Replace another unapproved contribution W with approved gear H: A's registrations of W report H with the same measured weight, places and ticks; W is gone from the admins' list. Edit or replace X: 409. As B, list, edit or replace A's contributions: 404. Race a replace with an approval: one wins and the other is 409. Any outcome missing from A's list, or any change by someone other than A, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP2, DC-1, DC-2](/docs/anchors/loadout-planning-and-packing.md)
- Accepted frontend design "Loadout planning and packing", capability need N10, flow F8 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-032 Users contribute new gear to the catalog, and it is in their closet at once](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)
- [BR-033 Admins approve, merge or decline catalog contributions](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md)
