---
type: Behavioral Requirement
title: BR-018 Refused requests change nothing and use a defined status
description: Defines the observable outcome of an authorization refusal — no stored change, no disclosure, and a specific status (401, 403, 404 or 400) by refusal reason, with fixed precedence.
tags: [authorization, error-handling, http-status, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-018 — Refused requests change nothing and use a defined status

**Implementation:** implemented: wave 1 covered user, role and refresh refusals; wave 2 adds loadouts and loadout items (foreign, unknown and wrong-parent give one identical 404; 404 takes precedence over a body-conflict 400); wave 3 applies the same to registrations, container lists, container links and listing another user's registrations (403). A malformed ID is 400 before the ownership check. Wave 4 adds the catalog: non-admin writes are 403 and change nothing, unknown catalog IDs are 404 (not 500 or 200). Account deletion was done in wave 1 (see [BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md)), so nothing remained for this requirement. The "protected permanent admin" reason, added with [BR-019](/docs/requirements/requirements/br-019-permanent-first-admin.md), is implemented in wave 7 (403, nothing changed). Container links add 409 outcomes for a second container or a cycle ([BR-020](/docs/requirements/requirements/br-020-container-nesting-consistency.md)), decided after the ownership 404 and the self-link 400. The registration update (`POST /usergear/registration/:usergear/update`) reads its body only after the ownership check, in the same transaction: a foreign or unknown registration is the same 404 for every body, including one that is not JSON or breaks several rules, and the owner gets the 400 (test `TestRegistrationUpdate_RefusalOrderIsOwnershipThenBody` in `pkg/api/registration_weights_test.go`). **Status:** Accepted with provisional status choices. **Priority:** P0 (needed for every access-control requirement). **Basis:** the no-effect and no-disclosure rules (AC-1, AC-2) are derived from anchor decisions D3 and D4. The status classes are specified choices of the requirements author, and the 404 rule is provisional default P-1.

**Statement:**
When the API refuses a request for an authentication or authorization reason, it shall change no stored data, reveal no data the caller may not see, and respond with the status that matches the reason below.

**Acceptance criteria:**

- AC-1: A refused request never returns a success (2xx) status and never changes stored data.
- AC-2: A refused request's response contains no data from the protected record.
- AC-3 — Refusal reasons and statuses:
  - **No valid token** for an operation that needs one: 401.
  - **Admin role required.** The caller is authenticated but not an admin, and the operation is admin-only (catalog changes, admin-status changes, direct user creation): 403.
  - **Another user's account or collection.** The caller addresses another user by user ID, for example to update or delete that user's profile, or to list that user's registrations. Users are visible in the directory ([BR-007](/docs/requirements/requirements/br-007-user-profile-visibility.md)), so refusing reveals nothing new: 403.
  - **User-owned record not available to the caller.** The record is a registration, container link, loadout or loadout item that does not exist, is not owned by the caller, or is not a member of the parent named in the path: 404. The status and body are the same in all three cases, so a caller cannot tell another user's record from a nonexistent one. **[Provisional default P-1 — open to product-owner change. This changes the current 403 for foreign loadouts and loadout items, and the existing tests that expect 403.]**
  - **Protected permanent admin.** The request would revoke the permanent admin's status or delete the permanent admin's account ([BR-019](/docs/requirements/requirements/br-019-permanent-first-admin.md)): 403, for every caller.
  - **Conflicting identity value in the body** ([BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md)): 400.
- AC-4: Precedence when several reasons apply: 401 first, then 403, then 404, then 400. A malformed path identifier (for example, a non-numeric ID) may return 400 before the ownership check, because that reveals nothing about stored records.

**Verification semantics:**
For each refusal reason, run at least one representative operation and compare the status, the response body and the stored data before and after. For the 404 rule, compare the raw responses for a foreign record ID, a nonexistent record ID and a record addressed through the wrong parent: any difference falsifies the rule.

## Derived from

- [Access control and identity decisions — D3, D4](/docs/anchors/access-control-and-identity.md)

## Related

- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
