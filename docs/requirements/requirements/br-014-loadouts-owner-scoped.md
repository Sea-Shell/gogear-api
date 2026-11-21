---
type: Behavioral Requirement
title: BR-014 Loadouts are readable and changeable only by their owner
description: Listing, reading, creating, updating, deleting and importing into loadouts through authenticated operations is limited to the owner; private loadouts are never visible to anyone else.
tags: [loadouts, ownership, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-014 — Loadouts are readable and changeable only by their owner

**Implementation:** implemented in wave 2 (loadouts), with adversarial tests in `pkg/api/loadout_authz_test.go`. Update, delete and import bind loadout ID and owner in one statement and check the affected row count; a body `loadout_id` or `user_id` that differs from the path or caller is refused with 400. **Status:** Accepted. **Priority:** P0 (review SEC-03). **Basis:** sourced from anchor decision D4. AC-3 is derived from it. AC-6 is a provisional default.

**Statement:**
Through its authenticated loadout operations, the API shall list, return, update, delete and import into a loadout only for the authenticated user who owns it. New loadouts shall always be owned by the caller.

**Acceptance criteria:**

- AC-1: Listing loadouts returns only the caller's own loadouts, both public and private.
- AC-2: Getting, updating, deleting or importing gear into a loadout the caller does not own is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). The loadout and its items are unchanged. This applies to public loadouts too: the public, read-only view is available only through [BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md).
- AC-3: An update changes only the loadout named in the request path. A loadout identifier in the body never redirects the update to another loadout ([BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md)). Example from the review: A owns 101, B owns 202, and A sends an update for 101 with body ID 202. Loadout 202 must be unchanged.
- AC-4: A created loadout is always owned by the caller, and no update changes a loadout's owner.
- AC-5: A private loadout is not visible to anyone other than its owner through any operation.
- AC-6: Admins get no exception. **[Provisional default P-3 — open to product-owner change]**

**Verification semantics:**
Seed private and public loadouts for A and B. As A, list (only A's), and get, update, delete and import into B's loadouts (404, B's loadouts and items unchanged). Update 101 with body ID 202: 202 is unchanged. The requirement is falsified by any response containing B's loadout through these operations, or any stored change to B's loadouts.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-015 Loadout items are reachable only through their own, owned loadout](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
