---
type: Behavioral Requirement
title: BR-008 Users update only their own profile
description: A profile update succeeds only when the caller updates their own account, identified by the request path, and it never changes admin status or account identity.
tags: [users, profile, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-008 — Users update only their own profile

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. **Status:** Accepted. **Priority:** P0 (review SEC-01, SEC-02). **Basis:** the statement and AC-1 are sourced from anchor decision D3. AC-3 is derived from it. AC-2 is a provisional default.

**Statement:**
When an authenticated user asks to update a user profile, the API shall apply the update only if the profile addressed by the request path is the caller's own account. The update shall never change any account's admin status.

**Acceptance criteria:**

- AC-1: A user updates another user's profile. The request is refused with 403 under [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md), and the target account is unchanged.
- AC-2: Admins get no exception. An admin updating another user's profile is also refused with 403, except for admin-status changes through [BR-010](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md). **[Provisional default P-3 — open to product-owner change]**
- AC-3: The account changed is always the one the request path names. Any account identifier or admin status in the request body is governed by [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md). It never redirects the update to another account, and it never changes admin status, even when the caller is an admin.
- AC-4: A user updating their own profile succeeds for the profile fields that can be updated today. This requirement does not change which ordinary profile fields can be edited.

**Verification semantics:**
As non-admin A, update B through B's path: 403, and B's stored record is unchanged. As A, update through A's path with a body naming B's ID: B is unchanged. As A, send admin status true: A's stored admin status stays false, and a following refresh yields a non-admin token. As admin C, update B: 403. The requirement is falsified by any stored change to an account other than the caller's, or any change of admin status through this operation.

## Derived from

- [Access control and identity decisions — D3](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-007 Other users' email and admin status are hidden from non-admins](/docs/requirements/requirements/br-007-user-profile-visibility.md)
- [BR-010 Admin status changes are admin-only](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md)
