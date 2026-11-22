---
type: Behavioral Requirement
title: BR-011 Direct user creation is admin-only and creates non-admin accounts
description: The direct account-creation operation (as opposed to Google sign-in) creates an account only for an admin caller, and the created account is never an admin.
tags: [users, account-creation, authorization, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-011 — Direct user creation is admin-only and creates non-admin accounts

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. **Status:** Accepted with a provisional caller restriction. **Priority:** P0 (review SEC-01). **Basis:** AC-2 is sourced from anchor decision D3 ("no client can set the admin flag through profile create"). The statement's admin-only restriction is a provisional default.

**Statement:**
When an authenticated user asks the API to create a user account directly, the API shall create it only if the caller is an admin. **[Provisional default P-5 — open to product-owner change]** Google sign-in is a separate path, governed by [BR-005](/docs/requirements/requirements/br-005-first-google-sign-in.md).

**Acceptance criteria:**

- AC-1: A non-admin direct-creation request is refused with 403 under [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). No account is created.
- AC-2: An account created directly always has admin status false. A request that asks for admin status true is handled as defined in [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md) and creates no admin.
- AC-3: A directly created account has no Google subject. It can be bound later only under [BR-004](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md).

**Verification semantics:**
As non-admin A, create a user: 403, and the account count is unchanged. As admin C, create a user with admin status true in the body: no admin account exists afterwards. As C, create a user without admin status: a non-admin account exists. The requirement is falsified by any account created by a non-admin, or any admin account created through this operation.

## Derived from

- [Access control and identity decisions — D3](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-010 Admin status changes are admin-only](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md)
