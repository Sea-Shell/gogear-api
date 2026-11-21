---
type: Behavioral Requirement
title: BR-005 First Google sign-in creates a new account only when no existing account claims the email
description: A Google subject that is neither bound nor eligible for backfill gets a new non-admin account only with a verified email that no existing account uses; otherwise sign-in is refused.
tags: [identity, google-oidc, sign-in, account-creation, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-005 — First Google sign-in creates a new account only when no existing account claims the email

**Status:** Accepted with provisional conditions. **Priority:** P1 (review SEC-08). **Basis:** the API creates an account on first Google sign-in today, and this requirement keeps that behavior. Who may sign in at all is controlled by the Google OAuth configuration. AC-2 is derived from anchor decision D2. AC-3 and AC-4 are provisional defaults.

**Statement:**
When a sign-in carries Google subject S, and no account is bound to S, and no account is eligible for backfill under [BR-004](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md), the API shall create a new account bound to S only if the token's `email_verified` claim is true and no existing account has the token's email. Otherwise the API shall refuse the sign-in.

**Acceptance criteria:**

- AC-1: A newly created account is bound to S, has admin status false, and is signed in. A later sign-in with S reaches it through [BR-003](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md).
- AC-2: If the token's email belongs to an account that is already bound to another subject, the API neither links that account (BR-003 AC-2) nor creates a second account with the same email. The sign-in is refused. (The refusal is derived from "an email match alone never links an account". Refusing instead of creating a duplicate is part of provisional default P-6.)
- AC-3: If `email_verified` is false or absent, no account is created and the sign-in is refused. **[Provisional default P-6 — open to product-owner change]**
- AC-4: A refused sign-in returns 403, creates or changes no account, and issues no service token. **[Provisional default P-6 — open to product-owner change]**
- AC-5: Email equality uses the same rule as BR-004 AC-4.

**Verification semantics:**
With real sign-in handling and stored data, sign in with a new `sub`: (a) verified, unused email: one new non-admin account bound to that `sub`; (b) verified email used by an account bound to another `sub`: 403 and the account count is unchanged; (c) unverified email: 403 and no account created. The requirement is falsified by a new account created without verified email, a second account sharing an existing account's email, or a new account with admin status true.

## Derived from

- [Access control and identity decisions — D2](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-003 Google sign-in identifies accounts by Google subject only](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md)
- [BR-004 Legacy account subject backfill](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md)
