---
type: Behavioral Requirement
title: BR-002 Catalog contributions require admin approval, and the contributor can use them at once
description: Planned, not implemented. A non-admin's new gear never becomes visible to other users before an admin approves it, but the contributor can use it at once in their closet and loadouts, marked as waiting.
tags: [catalog, admin-approval, catalog-contributions, planned, security-remediation, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-002 — Catalog contributions require admin approval, and the contributor can use them at once

**Amendment of 2026-11-22 (anchor decision [LP2](/docs/anchors/loadout-planning-and-packing.md)):** the owner decided what proposers see, which this requirement had left open: the contributor can use their contribution at once, marked as waiting; others see it only after approval. New AC-3 to AC-5; the statement, title and scope limit are updated. AC-1 and AC-2 are unchanged. The workflow is specified in [BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md) to [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md), and the shared visibility rule in [OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md). LP2 also said "no private items"; the same day the owner chose private items for everyone instead ([LP8](/docs/anchors/loadout-planning-and-packing.md), [BR-037](/docs/requirements/requirements/br-037-private-items.md)). A private item is not a contribution. When its owner sends it to the catalog, it becomes one and this requirement applies (new AC-6).

**Implementation:** still planned. Wave 4 enforced only the interim admin-only rule of BR-001; no contribution or approval flow exists. **Status:** Accepted (amended 2026-11-22); planned, not implemented. **Priority:** P2 (milestone D). **Basis:** the statement and AC-1 are sourced from anchor decision D1; AC-3 and AC-4 from LP2; AC-5 is derived from LP2, which concerns adding gear.

**Statement:**
A catalog contribution by a non-admin user shall become visible to other users only after an admin approves it. Until then, the contributor shall be able to use it as if it were catalog gear, marked as not approved yet.

**Scope limit:**
The owner has decided that contributions require admin approval (D1) and what proposers see (LP2). The workflow comes from the accepted frontend design and is specified in BR-032 to BR-034, partly as designer's choice DC-1 (open to owner change) and DC-2 (accepted by the owner on condition that a merge keeps everything the user provided). Private items, which are not contributions until sent to the catalog, are in BR-037 (LP8).

**Acceptance criteria:**

- AC-1: A non-admin contribution never appears in, or changes, catalog data that other users read before an admin has approved it.
- AC-2: Until this feature is implemented, [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) applies without exception. No partial contribution path is exposed to non-admins.
- AC-3: From the moment it is submitted, the contributor can use their unapproved contribution like catalog gear: register it (more than once), use its registrations in loadouts and as bags if its gear is container-capable, set measured weights and codes, and look it up by its product code. Wherever the API reports it to the contributor, it reports its state ("waiting", or "not approved" with the reason).
- AC-4: To every other user, an unapproved contribution and its new maker behave exactly as if they did not exist, in every operation ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)).
- AC-5: A contribution adds new gear, with a new maker if needed. Proposing changes to existing catalog records is not part of this feature; non-admins still cannot change or delete them (BR-001).
- AC-6 (added 2026-11-22, LP8): A private item that its owner sends to the catalog is a contribution from that moment, with AC-1 to AC-5 applying to it. Before that, it is not a contribution and not catalog data (BR-037).

**Verification semantics:**
Now: verified through BR-001. No contribution path exists for non-admins. Once implemented: as A, contribute X; A registers X twice, puts both registrations in a loadout and sets a measured weight: all succeed, and X is reported to A as waiting. As B, read the catalog, look up X's product code and try to register X: X is not visible, and B gets the unknown-gear outcome. After an admin approves X, B sees X. The requirement is falsified if the contribution is visible or effective for anyone but its contributor before approval, or not usable by the contributor before approval.

## Derived from

- [Access control and identity decisions — D1](/docs/anchors/access-control-and-identity.md)
- [Loadout planning and packing decisions — LP2, LP8](/docs/anchors/loadout-planning-and-packing.md)

## Related

- [BR-001 Catalog changes are admin-only](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md)
- [BR-032 Users contribute new gear to the catalog, and it is in their closet at once](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)
- [BR-033 Admins approve, merge or decline catalog contributions](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md)
- [BR-034 Contributors see outcomes, and can edit, resubmit or replace their contributions](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md)
- [OC-002 Visibility of gear outside the shared catalog](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)
