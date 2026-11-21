---
type: Behavioral Requirement
title: BR-004 Legacy accounts get their Google subject filled in once, by verified email
description: An existing account without a stored Google subject is bound automatically on its next sign-in, only when the token's email is verified and matches exactly one such account.
tags: [identity, google-oidc, account-binding, migration, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-004 — Legacy accounts get their Google subject filled in once, by verified email

**Status:** Accepted. **Priority:** P1 (review SEC-08). **Basis:** the statement and AC-1, AC-2 and AC-6 are sourced from anchor decision D2. AC-3 and AC-7 are derived from it. AC-4 and AC-5 are provisional defaults.

**Terms:**
- *Unbound account*: an existing account with no stored Google subject. This includes every account created before this requirement and accounts created directly by an admin ([BR-011](/docs/requirements/requirements/br-011-direct-user-creation-admin-only.md)).
- *Eligible account*, for a given sign-in: an unbound account whose stored email equals the token's email (see AC-4).

**Statement:**
When a sign-in carries Google subject S, and no account is bound to S, and exactly one account is eligible, and the token's `email_verified` claim is true, the API shall bind S to that account and sign the user in as it.

**Acceptance criteria:**

- AC-1: After a successful binding, every later sign-in for that account is resolved by subject only ([BR-003](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md)). Email plays no further role in identifying the account.
- AC-2: If `email_verified` is false or absent, nothing is bound and the sign-in is refused. No account is created or changed, and no service token is issued.
- AC-3: An account that is already bound is never eligible, whatever its email is.
- AC-4: Email equality means exact, case-sensitive equality of the stored email and the token's email, after removing leading and trailing whitespace from both. **[Provisional default P-7 — open to product-owner change]**
- AC-5: If more than one unbound account has the token's email, nothing is bound and the sign-in is refused. No account is created or changed, and no service token is issued. **[Provisional default P-6 — open to product-owner change]**
- AC-6: Binding needs no user or admin action beyond the sign-in itself.
- AC-7: Binding and sign-in succeed or fail together. If the binding cannot be stored, no service token is issued. If two sign-ins with different subjects race for the same eligible account, at most one binding is stored, and only the sign-in whose subject was bound succeeds as that account.

**Verification semantics:**
Seed unbound accounts and run real sign-in handling against stored data. Show that a verified, matching token binds and signs in. Show that a second sign-in with a new email but the same `sub` still reaches the account. Show that an unverified matching token binds nothing. Show that a case-different email and duplicate unbound emails bind nothing. Show that concurrent sign-ins with two subjects produce at most one binding. The requirement is falsified by any binding made without verified email, any binding of an already-bound account, or more than one subject bound to an account.

## Derived from

- [Access control and identity decisions — D2](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-003 Google sign-in identifies accounts by Google subject only](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md)
- [BR-005 First Google sign-in](/docs/requirements/requirements/br-005-first-google-sign-in.md)
