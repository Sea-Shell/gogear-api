---
type: Behavioral Requirement
title: BR-016 Public loadouts are readable by slug without signing in; private ones are not
description: The unauthenticated public view returns a loadout and its items by slug only while the loadout is public; private and nonexistent slugs give the same not-found response.
tags: [loadouts, public-sharing, privacy, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-016 — Public loadouts are readable by slug without signing in; private ones are not

**Implementation:** implemented in wave 2 (public loadouts), with tests in `pkg/api/loadout_authz_test.go`. A blank slug is stored as no slug (NULL) and never matches a public read; empty item lists return `[]`. **Status:** Accepted. **Priority:** P0 (completes the visibility rule in D4). **Basis:** sourced from anchor decision D4. AC-2 is derived from it. AC-4 is a specified choice of the requirements author and matches current behavior.

**Statement:**
The API shall return a loadout and its items by slug, without authentication, if and only if the loadout is marked public at the time of the request.

**Acceptance criteria:**

- AC-1: A public loadout and its items can be read by slug without a token.
- AC-2: A private loadout's slug gets the same status and body as a slug that does not exist (404). No details of the private loadout or its items are revealed.
- AC-3: After the owner makes a loadout private, or deletes it, the next public read of its slug returns 404.
- AC-4: The public view is read-only. No change can be made through it.

**Verification semantics:**
Without a token, read a public slug (200, with items), a private slug and an unknown slug. The last two responses must be byte-identical in status and body. Make the public loadout private and read it again (404). The requirement is falsified by any private-loadout data in a public response.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-014 Loadouts are readable and changeable only by their owner](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md)
