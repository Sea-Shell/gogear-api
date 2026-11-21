---
type: Behavioral Requirement
title: BR-013 Container links involve only the caller's own registrations
description: Listing a container's contents, linking gear into a container and removing a link are allowed only when the container and its contents are registrations owned by the caller.
tags: [containers, inventory, ownership, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-013 — Container links involve only the caller's own registrations

**Implementation:** implemented in wave 3, with adversarial tests in `pkg/api/usergear_authz_test.go`. Listing checks the container owner and returns only contents the caller owns (also for legacy cross-owner links). Create is one statement that inserts only when the caller owns both registrations. Delete removes a link only through its container's owner. A link of a registration to itself is refused with 400 after the ownership check. Wave 6 adds database rules (migration 000005): a duplicate link is refused (409) and a self-link is refused by the database itself; existing duplicates were reduced to one and existing self-links were left for review. The product owner answered open question Q-4 (anchor decision D9); the resulting nesting rules are in [BR-020](/docs/requirements/requirements/br-020-container-nesting-consistency.md), which is only partly implemented. **Status:** Accepted. **Priority:** P0/P1 (review SEC-06). **Basis:** the statement and AC-1 to AC-3 are sourced from anchor decision D4 (ownership of both parent and child). AC-4 is a specified choice of the requirements author. AC-5 is a provisional default.

**Terms:**
- *Container*: a gear registration ([BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)) that holds, or is asked to hold, other registrations. Which registrations may do so is defined in [BR-020](/docs/requirements/requirements/br-020-container-nesting-consistency.md).
- *Container link*: a record saying that one gear registration (the content) is inside a container registration (the parent).

**Statement:**
The API shall list, create and remove container links only when the caller owns the registrations involved, as defined below.

**Acceptance criteria:**

- AC-1: Listing the contents of a container the caller does not own is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). Listing the contents of an owned container returns only content registrations the caller owns.
- AC-2: Creating a link succeeds only if the caller owns both the container registration and the content registration. Otherwise it is refused with the "not found" outcome, and no link is created. All four combinations (own/own, own/foreign, foreign/own, foreign/foreign) behave this way.
- AC-3: Removing a link succeeds only if the caller owns the link's container registration. Otherwise it is refused with the "not found" outcome, and the link remains.
- AC-4: AC-1 also holds for links stored before this requirement whose content belongs to another user: such content is never returned to the container's owner. (Specified choice: this keeps responses owner-scoped even if bad links already exist in stored data.)
- AC-5: Admins get no exception. **[Provisional default P-3 — open to product-owner change]**

**Verification semantics:**
Seed registrations for A and B. As A, attempt the four create combinations: only own/own creates a link. As A, list and remove links on B's container: refused, and stored links are unchanged. Insert a legacy cross-owner link directly into the stored data and list A's container: B's registration is absent. The requirement is falsified by any link created or removed involving a registration not owned by the caller, or any foreign registration in a listing.

**Out of scope:** What may hold gear, nesting, cycles and duplicate links (review SEC-06 and section 9) are not part of this requirement. They are defined in [BR-020](/docs/requirements/requirements/br-020-container-nesting-consistency.md), which applies in addition to this one.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)

## Related

- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
- [BR-020 Containers nest to any depth, with one direct container per registration and no cycles](/docs/requirements/requirements/br-020-container-nesting-consistency.md)
