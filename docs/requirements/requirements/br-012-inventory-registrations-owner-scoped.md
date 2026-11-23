---
type: Behavioral Requirement
title: BR-012 Gear registrations are readable and changeable only by their owner
description: Listing, reading, creating, updating and deleting user gear registrations (inventory) is limited to the authenticated owner, with no admin override.
tags: [inventory, user-gear, ownership, authorization, security-remediation]
timestamp: 2026-11-22T00:00:00Z
---

# BR-012 — Gear registrations are readable and changeable only by their owner

**Amendment of 2026-11-22 (loadout planning, [LP1](/docs/anchors/loadout-planning-and-packing.md)):** a registration gains values its owner can change: a measured weight with its source ([BR-026](/docs/requirements/requirements/br-026-measured-weight-on-a-registration.md)), a closet count ([BR-036](/docs/requirements/requirements/br-036-closet-count-of-a-registration.md)) and an own code ([BR-035](/docs/requirements/requirements/br-035-codes-and-scanning.md)); this requirement applies to all of them. A successful delete now also affects the owner's loadouts that use the registration ([BR-030](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md)). A registration's gear can change only through an admin merge or the owner's replace of their own contribution (BR-033, BR-034); AC-4 is unchanged. Not implemented.

**Implementation:** implemented in wave 3, with adversarial tests in `pkg/api/usergear_authz_test.go`. Get, update and delete bind the registration ID and the caller in one SQL statement; list is scoped to the caller and refuses other user IDs with 403, admins included. Category, top-category and manufacturer filters are parsed as integers and bound (a non-numeric value is ignored); the single-record get takes no filters. Delete runs in one transaction and removes container links only after the owned registration was deleted. A list shows a registration once even if legacy data put it in several containers; since wave 7 ([BR-020](/docs/requirements/requirements/br-020-container-nesting-consistency.md)) the database no longer allows that state. NULL gear columns no longer break reads. Wave 6: a registration insert naming gear that is not in the catalog is 404 (the foreign key enforces it); registrations are indexed by owner and gear. **Status:** Accepted. **Priority:** P0 (review SEC-05, SEC-07). **Basis:** the statement and AC-1 to AC-4 are sourced from anchor decision D4. AC-5 and AC-6 are derived from it. AC-7 is a provisional default.

**Terms:**
- *Gear registration*: one item in a user's inventory. It records that a user owns a piece of catalog gear. Its owner is the user it is registered to.

**Statement:**
The API shall list, return, create, update and delete gear registrations only for the authenticated user who owns them.

**Acceptance criteria:**

- AC-1: Listing the registrations of a user other than the caller is refused with 403 under [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). Listing the caller's own registrations returns only registrations the caller owns.
- AC-2: Getting, updating or deleting a registration the caller does not own is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md), and the registration is unchanged.
- AC-3: A created registration is always owned by the caller. Owner identifiers in the body are governed by [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md).
- AC-4: An update never changes a registration's owner.
- AC-5: A refused delete has no side effects. In particular, it removes no container links ([BR-013](/docs/requirements/requirements/br-013-container-links-owner-scoped.md)).
- AC-6: No request parameter can widen the returned records beyond the caller's own, including filters such as category or top category. Example: a category filter of `0 OR 1=1`, or any other non-numeric value, returns either 400 or only the caller's matching records, never a 5xx error and never another user's registration. (Derived: a filter that changes the query's meaning defeats owner scoping. This is review finding SEC-07.)
- AC-7: Admins get no exception. An admin is limited to their own registrations like every other user. This removes the current ability of admins to list other users' registrations. **[Provisional default P-3 — open to product-owner change]**

**Verification semantics:**
Seed registrations for A and B. As A, list B (403), and get, update and delete B's registration (404, with B's registration and its container links unchanged). Create with a body naming B as owner: no registration owned by B is created. Send injected and non-numeric category filters: the result contains only A's records or is a 400. Repeat the list as admin C against B: 403. The requirement is falsified by any response containing B's registration data or any stored change to B's registrations.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-030 Deleting a registration keeps the loadouts that use it consistent](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md)
- [BR-026 The owner can record their own weight on a registration, with its source](/docs/requirements/requirements/br-026-measured-weight-on-a-registration.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
