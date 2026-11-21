---
type: Behavioral Requirement
title: BR-003 Google sign-in identifies accounts by Google subject only
description: An account bound to a Google subject (sub) is reached only by a sign-in with that same subject; email never selects or links an already-bound account.
tags: [identity, google-oidc, sign-in, authentication, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-003 — Google sign-in identifies accounts by Google subject only

**Implementation:** implemented in wave 5. Accounts carry a nullable, unique `googleSubject` (migration 000004). Sign-in looks up a bound account by subject alone, before it reads the email, so a changed or unverified token email still reaches it (AC-1) and another subject carrying the email never does (AC-2). A token with a missing or blank subject is 401 and changes nothing (AC-3); the email fallback is gone. The unique index and the conditional bind (`googleSubject IS NULL`) keep one subject per account and never replace a binding (AC-4). Token validation sits behind a seam (`GoogleTokenValidator`), so tests inject claims and run the real handler against a migrated database. **Status:** Accepted. **Priority:** P1 (review SEC-08). **Basis:** sourced from anchor decision D2. AC-3 and AC-4 are necessary consequences of it.

**Terms:**
- *Google subject* (`sub`): the stable account identifier in a Google ID token that the API has validated.
- *Bound account*: an account with a stored Google subject.

**Statement:**
When a user signs in with a valid Google ID token whose subject is bound to an account, the API shall sign the user in as that account. The API shall identify the account only by the subject and shall ignore the token's email for this purpose.

**Acceptance criteria:**

- AC-1: Account A is bound to subject S. A sign-in with subject S yields account A, even if the token's email differs from A's stored email (for example, the user changed their Google email). The result does not depend on the token's `email_verified` value.
- AC-2: A sign-in with a different subject T never yields account A, even if T's token carries A's email with `email_verified` true. What happens to T instead is defined by [BR-004](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md) and [BR-005](/docs/requirements/requirements/br-005-first-google-sign-in.md). Neither of them can yield A, because A is already bound.
- AC-3: A Google ID token with a missing or empty subject is refused. No account is created, changed or signed in, and no service token is issued. (Derived: without a subject the account cannot be bound, and falling back to email would be an email-only link. Today the code falls back to using the email as the subject; that must stop.)
- AC-4: Each Google subject is bound to at most one account, and each account to at most one subject. No sign-in replaces or removes an existing binding.

**Verification semantics:**
Use real Google-token validation, or a test double that keeps the same claim semantics, together with the real sign-in handling and stored data. Prove AC-1 with a bound account and a token carrying the same `sub` and a different email. Prove AC-2 with a second `sub` carrying the bound account's email and `email_verified: true`: the response must not identify account A or issue a token for A. Prove AC-3 with a token without `sub`. The requirement is falsified by any sign-in that yields a bound account for a different subject, or yields any account without a subject.

## Derived from

- [Access control and identity decisions — D2](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-004 Legacy account subject backfill](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md)
- [BR-005 First Google sign-in](/docs/requirements/requirements/br-005-first-google-sign-in.md)
- [BR-006 Issued roles reflect current stored state](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md)
