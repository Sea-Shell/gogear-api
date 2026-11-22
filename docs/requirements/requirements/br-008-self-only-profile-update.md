---
type: Behavioral Requirement
title: BR-008 Users update only their own profile
description: A profile update succeeds only when the caller updates their own account, identified by the request path; it can change username, name and email, and never changes admin status, user ID or the Google subject.
tags: [users, profile, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-008 — Users update only their own profile

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. Profile update already changes only username, name and email, and never the password, admin status or user ID; wave 5 added that it never changes the Google subject and that an email change sets the edited-email mark. So AC-4 to AC-6 match current behavior. Wave 7 checked the update statement against them and found no gap (its `SET` list names only username, name, email and the mark), and added tests that a hostile body (Google subject, mark and ID fields in several spellings) changes none of them, that only a real email change sets the mark, and that the user can never clear it (`TestUpdateUser_NeverTouchesSubjectIDAdminOrMarkExceptViaEmail`). These tests pass on the earlier code too: they guard the behavior rather than expose a defect. **Status:** Accepted. **Priority:** P0 (review SEC-01, SEC-02). **Basis:** the statement and AC-1 are sourced from anchor decision D3. AC-3 is derived from it. AC-2 is a provisional default. AC-4 and AC-5 are sourced from anchor decision D8 and AC-6 from D7.

**Statement:**
When an authenticated user asks to update a user profile, the API shall apply the update only if the profile addressed by the request path is the caller's own account. The update shall never change any account's admin status.

**Acceptance criteria:**

- AC-1: A user updates another user's profile. The request is refused with 403 under [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md), and the target account is unchanged.
- AC-2: Admins get no exception. An admin updating another user's profile is also refused with 403, except for admin-status changes through [BR-010](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md). **[Provisional default P-3 — open to product-owner change]**
- AC-3: The account changed is always the one the request path names. Any account identifier or admin status in the request body is governed by [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md). It never redirects the update to another account, and it never changes admin status, even when the caller is an admin.
- AC-4: A user updating their own profile can change every profile field: username, name and email. These are the fields a profile shows ([BR-007](/docs/requirements/requirements/br-007-user-profile-visibility.md)). (Sourced from anchor decision D8. The list of fields is a specified choice of the requirements author: the stored password is not a profile field, because it is never shown and there is no password sign-in.)
- AC-5: No profile update ever changes the account's Google subject, admin status or user ID, whatever the request contains (anchor decision D8). Admin status in the body is handled as in [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md). Values that are not profile fields, such as the edited-email mark of [BR-004](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md), cannot be changed or cleared by the user through a profile update.
- AC-6: The email is profile data only (anchor decision D7). Changing it never changes which account a Google sign-in reaches ([BR-003](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md)). It gives the account the edited-email mark ([BR-004](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md) AC-8).

**Verification semantics:**
As non-admin A, update B through B's path: 403, and B's stored record is unchanged. As A, update through A's path with a body naming B's ID: B is unchanged. As A, send admin status true: A's stored admin status stays false, and a following refresh yields a non-admin token. As admin C, update B: 403. As A, change A's username, name and email: all three are stored, and A's Google subject, admin status and user ID are unchanged. The requirement is falsified by any stored change to an account other than the caller's, or any change of admin status through this operation.

## Derived from

- [Access control and identity decisions — D3, D7, D8](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-007 Other users' email and admin status are hidden from non-admins](/docs/requirements/requirements/br-007-user-profile-visibility.md)
- [BR-010 Admin status changes are admin-only](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md)
