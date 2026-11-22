---
type: Behavioral Requirement
title: BR-019 The first admin is a permanent admin
description: One account, fixed once as the first admin, can never lose admin status or be deleted through the API, whoever asks; other admins can still be demoted.
tags: [users, roles, admin, account-deletion, authorization]
timestamp: 2026-11-21T00:00:00Z
---

# BR-019 — The first admin is a permanent admin

**Implementation:** implemented in wave 7 (review mitigation). Migration 000007 adds the single-row table `permanent_admin` (no foreign key to `users`) and designates the lowest-ID admin when it runs (AC-1, P-8); with no admin yet, insert and update triggers designate the first account that becomes admin by any means, and `INSERT OR IGNORE` on the singleton row means nothing can move the designation afterwards (AC-2). Because the row has no foreign key and user IDs are never reused, a permanent admin removed outside the API is not replaced (AC-8). `SetUserAdmin` and `DeleteUser` check the designation inside the same transaction as the change: revoking is 403 for every caller, granting is a no-op 200 (AC-3, AC-4), self-deletion is 403 with the account and its data unchanged (AC-5), other admins may be revoked, also by themselves (AC-7). Profile update still cannot change admin status (AC-6). Adversarial tests: `pkg/api/permanent_admin_test.go`, `migration7_test.go`. **Status:** Accepted with provisional conditions. **Priority:** P1. **Basis:** the statement and AC-3 are sourced from anchor decision D6 ("No admin may remove admin status from the first admin. The first admin is the permanent admin."). AC-2 and AC-6 are derived from it. AC-1, AC-4, AC-5 and AC-7 are provisional defaults chosen by the requirements author, because D6 does not say how the first admin is identified or what happens on a deletion attempt.

**Terms:**
- *Permanent admin*: the one account designated by AC-1. D6 calls it "the first admin".
- *Takes effect*: the first time an API version that implements this requirement runs against a given database.

**Statement:**
The API shall never remove admin status from the permanent admin and shall never delete the permanent admin's account, whoever makes the request.

**Acceptance criteria:**

- AC-1: When this requirement takes effect, the permanent admin is the account with the lowest user ID among the accounts whose stored admin status is true. If no account is an admin at that moment, the permanent admin is the first account whose stored admin status becomes true afterwards, by any means. **[Provisional default P-8 — open to product-owner change. The product owner should confirm that this selects their own account.]**
- AC-2: Once designated, the permanent admin never changes while that account exists. Promoting an account with a lower user ID, demoting other admins, or any other request does not move the designation. No operation hands it to another account. (Derived: "permanent" means it cannot move.)
- AC-3: A request to set the permanent admin's admin status to false is refused, whoever sends it: another admin, or the permanent admin themselves. The stored admin status stays true, and the next refresh of the permanent admin's token still yields an admin token ([BR-006](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md)).
- AC-4: The refusal in AC-3 is 403 and changes nothing ([BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md)). A request to set the permanent admin's admin status to true succeeds and changes nothing. **[Provisional default P-9 — open to product-owner change]**
- AC-5: A request by the permanent admin to delete their own account is refused with 403. The account and all of its data remain. Other users cannot delete it either ([BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md) AC-1 and AC-2). So the permanent admin's account cannot be deleted through the API at all. **[Provisional default P-9 — open to product-owner change]**
- AC-6: Profile create and update never change any admin status ([BR-010](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md) AC-3), so they cannot demote the permanent admin either.
- AC-7: Admins other than the permanent admin get no protection. Any admin may revoke another admin's status, and an admin who is not the permanent admin may revoke their own. Because the permanent admin always stays an admin, at least one admin always exists once a permanent admin is designated. **[Provisional default P-10 — open to product-owner change]**
- AC-8: Scope limit: apart from counting toward the designation in AC-1, changes made directly to stored data outside the API (for example by an operator) are not governed. The API cannot prevent them. If the permanent admin's account stops existing that way, no account becomes the permanent admin in its place.

**Verification semantics:**
Seed admins with user IDs 3 and 5 and a non-admin with ID 1, then let the requirement take effect: account 3 is the permanent admin. Promote account 1. As admin 5, revoke 3's admin status: 403, 3 stays admin. As 3, revoke 3's own status: 403. As 3, delete account 3: 403, and the account and its data are unchanged. As 1, revoke 1's own status, and as 3, revoke 5's status: both succeed. Repeat with no admin at take-effect time, then set one account's admin status to true outside the API: that account is the permanent admin. The requirement is falsified by any stored admin status of false for the permanent admin, any deletion of its account through the API, or any change of the designation.

## Derived from

- [Access control and identity decisions — D6, D3](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-010 Admin status changes are admin-only](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md)
- [BR-009 Users delete only their own account](/docs/requirements/requirements/br-009-self-only-account-deletion.md)
- [BR-018 Refused requests change nothing and use a defined status](/docs/requirements/requirements/br-018-denial-outcomes.md)
