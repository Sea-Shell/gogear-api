---
type: Behavioral Requirement
title: BR-009 Users delete only their own account, and deletion removes all their data or nothing
description: Account deletion succeeds only for the caller's own account and removes the account together with all of its user-owned data in one all-or-nothing outcome.
tags: [users, account-deletion, authorization, data-integrity, security-remediation]
timestamp: 2026-11-22T00:00:00Z
---

# BR-009 — Users delete only their own account, and deletion removes all their data or nothing

**Amendment of 2026-11-22 (anchor decision [LP2](/docs/anchors/loadout-planning-and-packing.md)):** new AC-8 says what happens to the user's catalog contributions. The new loadout data (places, progress, measured weights, counts, own codes) is user-owned data and is covered by AC-3 without change. Not implemented.

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. Wave 6 keeps the delete order (links, items, loadouts, registrations, then the user), which foreign-key enforcement now requires, and the test suite runs it with enforcement on (`TestCascadesWithForeignKeysEnforced`). AC-6 (permanent admin) is implemented in wave 7: the delete transaction refuses the permanent admin with 403 before it removes anything ([BR-019](/docs/requirements/requirements/br-019-permanent-first-admin.md)). **Status:** Accepted with provisional cascade behavior. **Priority:** P0 (review SEC-02). **Basis:** the statement and AC-1 are sourced from anchor decision D3. AC-2, AC-3 and AC-4 are provisional defaults. AC-5 is derived from D4. AC-6 comes from [BR-019](/docs/requirements/requirements/br-019-permanent-first-admin.md) (D6) and AC-7 from D10.

**Statement:**
When an authenticated user asks to delete a user account, the API shall delete it only if the account addressed by the request path is the caller's own.

**Acceptance criteria:**

- AC-1: A user deletes another user's account. The request is refused with 403 under [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). The target account and all of its data remain.
- AC-2: Admins get no exception. An admin deleting another user's account is also refused with 403. **[Provisional default P-3 — open to product-owner change]**
- AC-3: A user deletes their own account. The account and all of its user-owned data are removed: gear registrations, container links that involve those registrations, loadouts, and the items of those loadouts. Afterwards, the former owner's public loadout slugs return 404 ([BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md)), and refreshing the former owner's token is refused ([BR-006](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md) AC-3). **[Provisional default P-4 — open to product-owner change]**
- AC-4: Deletion is all-or-nothing. If any part fails, the account and all of its data remain as they were before the request, and the response is not a success. **[Provisional default P-4]**
- AC-5: Deletion never removes or changes other users' data or the catalog. For example, another user's loadout items that reference the same catalog gear are unchanged.
- AC-6: Exception to AC-3: the permanent admin cannot delete their own account. The request is refused as defined in [BR-019](/docs/requirements/requirements/br-019-permanent-first-admin.md) AC-5.
- AC-7: After a deletion, service tokens issued to the deleted account before it was deleted are not revoked early (anchor decision D10, [BR-006](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md) AC-5). Refreshing them is refused (BR-006 AC-3).
- AC-8 (added 2026-11-22): Deleting an account also removes the user's private items and their private makers ([BR-037](/docs/requirements/requirements/br-037-private-items.md)), and the user's unapproved contributions ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)) and their new makers. Nobody else can see or use them ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)), so this changes nothing other users see, and the contributions leave the admins' list. The user's approved contributions are catalog data and stay (AC-5). **[Provisional default P-32 — open to product-owner change]**

**Verification semantics:**
As A, delete B: 403, and B's account, registrations, links and loadouts are unchanged. As admin C, delete B: 403. As A, delete A: none of A's account, registrations, container links, loadouts or loadout items remain, and B's data is identical to before. Inject a failure partway through: A's account and data are fully intact. The requirement is falsified by any partial deletion or any change to another user's data.

## Derived from

- [Loadout planning and packing decisions — LP2](/docs/anchors/loadout-planning-and-packing.md) (AC-8)
- [Loadout planning and packing decisions — LP8](/docs/anchors/loadout-planning-and-packing.md) (AC-8, private items)
- [Access control and identity decisions — D3, D4](/docs/anchors/access-control-and-identity.md)

## Related

- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
- [BR-019 The first admin is a permanent admin](/docs/requirements/requirements/br-019-permanent-first-admin.md)
