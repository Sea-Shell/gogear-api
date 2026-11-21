---
type: Behavioral Requirement
title: BR-007 Other users' email and admin status are hidden from non-admins
description: Any authenticated user may list and view user profiles, but responses to a non-admin never reveal another user's email or admin status; own profile and admin viewers see them.
tags: [users, privacy, directory, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-007 — Other users' email and admin status are hidden from non-admins

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. Review follow-up on AC-6: name fields. The profile views and the user search hide a username or name that contains `@` from non-admin viewers of other users, and the search applies the same rule in SQL (`TestUserDirectory_AnyEmailLikeNameIsHiddenAndNotSearchable`). The rule is `@` only: a value that embeds a real address always contains `@`, and comparing with the stored email needed the same case folding in Go and SQLite, which disagreed for non-ASCII text (`TestUserDirectory_SearchAndResponseAgreeOnNonASCIIText`). The registration and container responses carry the owner's `user_username` and `user_name`, but every one of those queries is owner-scoped, so they can only be the caller's own; no non-owner, including an admin, receives another user's name fields, even with legacy cross-owner links (`TestUserGear_ResponsesNeverCarryOtherUsersNames`). **Status:** Accepted. **Priority:** P0 (review SEC-02, disclosure part). **Basis:** sourced from anchor decision D3. AC-5 keeps existing behavior unchanged.

**Statement:**
The API shall let any authenticated user list user profiles and view a single user profile. When the caller is not an admin, the response shall not reveal the email address or admin status of any user other than the caller.

**Acceptance criteria:**

- AC-1: A non-admin views another user's profile. The response contains the profile but does not contain that user's email address or admin status, in any field or form.
- AC-2: A non-admin views their own profile. The response includes their own email address and admin status.
- AC-3: A non-admin lists users. No entry except the caller's own reveals an email address or admin status.
- AC-4: An admin lists or views users. Responses include every user's email address and admin status.
- AC-5: No response reveals any user's password or password hash. This is current behavior and must be kept.
- AC-6: AC-1 and AC-3 apply to every response that describes another user, not only the user-directory operations. For example, a response that includes another user's name must not include their email.
- AC-7: Without a valid service token, the directory is not available (401).

**Verification semantics:**
Seed users A (non-admin) and B, and an admin C. As A, get B and list all users. Search the raw response bodies for B's email string and for any admin-status field about B; finding either falsifies the requirement. As A, get A: both values are present. As C, get B: both values are present.

## Derived from

- [Access control and identity decisions — D3](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-008 Users update only their own profile](/docs/requirements/requirements/br-008-self-only-profile-update.md)
