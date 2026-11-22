---
type: Behavioral Requirement
title: BR-016 Public loadouts are readable by slug without signing in; private ones are not
description: The unauthenticated public view returns a loadout and its items by slug only while the loadout is public; private and nonexistent slugs give the same not-found response.
tags: [loadouts, public-sharing, privacy, security-remediation]
timestamp: 2026-11-22T00:00:00Z
---

# BR-016 — Public loadouts are readable by slug without signing in; private ones are not

**Amendment of 2026-11-22 (loadout planning, [LP1](/docs/anchors/loadout-planning-and-packing.md), [LP2](/docs/anchors/loadout-planning-and-packing.md)):** new AC-5 (the public view keeps working after items refer to registrations) and AC-6 (it never shows unapproved contributions or private items, LP8, [OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md) G-5). The public view stays secondary for the reworked frontend. Not implemented.

**Implementation:** implemented in wave 2 (public loadouts); AC-5 implemented in milestone A (2026-11-22: items carry `gear_name`, the list drops an item that has no gear row, `total_weight` follows the on-your-back view); AC-6 waits for milestone D; with tests in `pkg/api/loadout_authz_test.go`. A blank slug is stored as no slug (NULL) and never matches a public read; empty item lists return `[]`. **Status:** Accepted. **Priority:** P0 (completes the visibility rule in D4). **Basis:** sourced from anchor decision D4. AC-2 is derived from it. AC-4 is a specified choice of the requirements author and matches current behavior.

**Statement:**
The API shall return a loadout and its items by slug, without authentication, if and only if the loadout is marked public at the time of the request.

**Acceptance criteria:**

- AC-1: A public loadout and its items can be read by slug without a token.
- AC-2: A private loadout's slug gets the same status and body as a slug that does not exist (404). No details of the private loadout or its items are revealed.
- AC-3: After the owner makes a loadout private, or deletes it, the next public read of its slug returns 404.
- AC-4: The public view is read-only. No change can be made through it.
- AC-5 (added 2026-11-22): After loadout items refer to registrations ([BR-023](/docs/requirements/requirements/br-023-loadout-items-are-own-registrations.md)), the public view still lists each item with its gear and quantity, and any total it reports follows [BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md) AC-3. It is not required to show places, packing progress, stage, the owner's own codes or closet counts. One exception: an item whose registration was deleted ([BR-030](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md)) and whose gear record no longer exists is not listed. This can happen only with orphan rows stored before foreign keys were enforced; its kept weight may still count in the total. (Specified choice.)
- AC-6 (added 2026-11-22): The public view never shows an item whose gear is an unapproved contribution or a private item ([BR-037](/docs/requirements/requirements/br-037-private-items.md)), or any detail of that gear. Its weight may still count in the total. **[Leaving out private items is sourced from LP8 ("drop private items when its shared"). Treating unapproved contributions the same way, and counting the weight: provisional default P-34 — open to product-owner change]**

**Verification semantics:**
Without a token, read a public slug (200, with items), a private slug and an unknown slug. The last two responses must be byte-identical in status and body. Make the public loadout private and read it again (404). The requirement is falsified by any private-loadout data in a public response.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- [Loadout planning and packing decisions — LP1, LP2](/docs/anchors/loadout-planning-and-packing.md) (AC-5, AC-6)

## Related

- [BR-014 Loadouts are readable and changeable only by their owner](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md)
