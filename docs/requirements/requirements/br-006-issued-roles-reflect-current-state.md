---
type: Behavioral Requirement
title: BR-006 Issued tokens carry only the role the account currently has
description: Every service token issued at sign-in or refresh grants admin capability only if the account's stored admin status is true at issuance; a demoted admin cannot refresh into an admin token.
tags: [authentication, token-refresh, roles, admin, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-006 — Issued tokens carry only the role the account currently has

**Implementation:** implemented in wave 1 (users, roles, token refresh), with adversarial tests in `pkg/api`. **Status:** Accepted. **Priority:** P1 (review SEC-09, and the refresh step of SEC-01). **Basis:** sourced from anchor decision D5. AC-2 to AC-4 are necessary consequences of it. AC-5 is sourced from anchor decision D10.

**Statement:**
Whenever the API issues a service token, at Google sign-in or at token refresh, the token shall grant admin capability if and only if the account's stored admin status is true at the moment of issuance.

**Acceptance criteria:**

- AC-1: An admin is demoted (stored admin status becomes false). Refreshing a still-valid admin token then yields a token that does not authorize admin-only operations. Using it for an admin-only operation gets 403 under [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md).
- AC-2: A non-admin is promoted. The next refresh or sign-in yields a token that authorizes admin-only operations. ("Reflect current state" works in both directions.)
- AC-3: Refresh identifies the account only by the user identity in the presented service token, never by email. If that account no longer exists, refresh is refused with 401 and no token is issued.
- AC-4: A token issued while the account's stored admin status is false is never accepted as authorizing an admin-only operation. This holds for any service configuration, including configurations where token audiences or other role markers might collide.
- AC-5: This requirement governs newly issued service tokens only. A service token issued before a demotion or an account deletion keeps its role and stays valid until it expires (60 minutes by default). No server-side revocation and no per-request check of the stored role is required. (Accepted decision D10, which answers former open question Q-3. Not provisional.)

**Verification semantics:**
Use the real token middleware, refresh handling and stored data. Issue an admin token, set the stored admin status to false, refresh, and call an admin-only operation (for example a catalog delete): 403, and the record is unchanged. Repeat with a promotion. Refresh a token of a deleted account: 401. The requirement is falsified by any issued token whose admin capability differs from the stored admin status at issuance.

## Derived from

- [Access control and identity decisions — D5, D10](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-010 Admin status changes are admin-only](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md)
- [BR-003 Google sign-in identifies accounts by Google subject only](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md)
