---
type: Behavioral Requirement
title: BR-010 Admin status changes are admin-only
description: Only an admin can grant or revoke a user's admin status, through an operation for that purpose; profile creation and update never set admin status for any caller.
tags: [users, roles, admin, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-010 — Admin status changes are admin-only

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. **Status:** Accepted. **Priority:** P0 (review SEC-01). **Basis:** the statement, AC-1 and AC-3 are sourced from anchor decision D3. AC-2 is derived from it.

**Statement:**
The API shall change a user's admin status only when an admin requests that change. The change shall not be possible through profile creation or profile update, whoever the caller is.

**Acceptance criteria:**

- AC-1: A non-admin attempts to change any user's admin status, their own included, by any operation. The attempt is refused, and the stored admin status is unchanged. A dedicated operation refuses with 403. Profile create and update behave as defined in [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md).
- AC-2: An admin can grant and revoke another user's admin status through an operation intended for that purpose. (Derived: D3 lets admins change roles, and profile update can no longer do it.)
- AC-3: Profile create ([BR-011](/docs/requirements/requirements/br-011-direct-user-creation-admin-only.md)) and profile update ([BR-008](/docs/requirements/requirements/br-008-self-only-profile-update.md)) never produce an admin, even when an admin calls them.
- AC-4: After a change, newly issued tokens reflect the new status ([BR-006](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md)).

**Verification semantics:**
As non-admin A, try to grant A admin status through every user operation: A's stored status stays false, and a refreshed token is non-admin. As admin C, grant and revoke B's admin status through the dedicated operation: the stored value changes, and B's next refresh reflects it. The requirement is falsified by any path that changes admin status without an admin caller, or through profile create or update.

## Derived from

- [Access control and identity decisions — D3](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-006 Issued tokens carry only the role the account currently has](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md)
