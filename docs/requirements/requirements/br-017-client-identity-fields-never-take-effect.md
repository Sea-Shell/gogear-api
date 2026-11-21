---
type: Behavioral Requirement
title: BR-017 Identity and privilege values in request bodies never take effect
description: Owner, target-record and parent identifiers, and admin status, sent by a client in a request body never override the authenticated identity, the request path or stored state; conflicting values are rejected.
tags: [authorization, input-handling, ownership, admin, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-017 — Identity and privilege values in request bodies never take effect

**Implementation:** partly implemented: wave 1 covered user operations and the admin-status field; wave 2 adds loadout and loadout-item operations (`loadout_id`, `loadout_item_id`, `user_id` in bodies). Registrations and containers are later waves. **Status:** Accepted with a provisional conflict outcome. **Priority:** P0 (review SEC-01, SEC-03, SEC-05). **Basis:** the statement is derived from anchor decisions D3 ("no client can set the admin flag through profile create/update") and D4 (scope to the authenticated owner). AC-2 is a provisional default.

**Terms — identity and privilege values:**
- the owner (user) of user-owned data;
- the identifier of the record being updated or deleted, when the request path names it;
- the parent identifier, when the request path names the parent (for example, the loadout of a loadout item);
- a user's admin status, in profile create and update.

**Statement:**
The API shall decide the owner of user-owned data from the authenticated identity, the target and parent records from the request path, and admin status from stored state. Values a client supplies in the request body shall never override these.

**Acceptance criteria:**

- AC-1: If the body leaves a value out, or gives the same value the API determined, the request proceeds, and that body value has no effect of its own.
- AC-2: If the body gives a different value, the request is refused with 400 and changes nothing. **[Provisional default P-2 — open to product-owner change; the alternative is to silently ignore the value]** Examples: a registration create naming another owner; a loadout update whose body loadout ID differs from the path; a profile update or create carrying admin status true when the stored or default value is false.
- AC-3: AC-2 applies to every caller, admins included, for admin status in profile create and update ([BR-010](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md)).
- AC-4: Under no outcome is a record other than the one determined by the API created, changed or deleted.

**Verification semantics:**
For each operation that accepts these values, send (a) no value, (b) the matching value, and (c) a conflicting value. Check the response status and the stored data. The requirement is falsified if (c) changes any stored data or returns a success status, or if (a) or (b) behave differently from each other.

## Derived from

- [Access control and identity decisions — D3, D4](/docs/anchors/access-control-and-identity.md)

## Related

- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
