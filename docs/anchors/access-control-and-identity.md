---
type: Anchor
title: Access control and identity decisions (security remediation)
description: Product-owner decisions on catalog editing rights, Google identity binding, user directory visibility, ownership of user data, and token refresh roles for the GoGear API security remediation.
tags: [access-control, identity, authorization, security-remediation, product-decision]
timestamp: 2026-11-21T00:00:00Z
---

# Access control and identity decisions

**Status:** Accepted by the product owner on 2026-11-21. Relayed to the requirements author by the orchestrating session for the review-mitigation work.

**Why this exists:** The engineering review of 2026-11-12 (revision `6fb972b`, sections 3, 9 and 10) found that ordinary users can make themselves admin, change or delete other users' data, and link Google identities by email. Fixing these defects needs explicit rules about who may do what. This document records the rules the product owner decided. The requirements in `/docs/requirements/` derive from it.

## Decisions

### D1 — Catalog editing

The catalog is gear, manufacturers, categories and top categories. Users may eventually contribute to the catalog, but only through admin approval. Interim rule for this remediation: catalog changes (create, update, delete) are admin-only. The approval-based contribution is planned and not yet implemented. Its workflow is not specified beyond "contributions require admin approval".

### D2 — Google identity

Accounts are bound to the stable Google subject identifier (`sub`). An existing account with no stored `sub` gets it filled in once, automatically, on the next sign-in, matched by email. This backfill is allowed only when Google reports the email as verified (`email_verified` is true). After binding, sign-in is by `sub` only. An email match alone never links an account.

### D3 — User directory

Any authenticated user may view other users' profiles. Email and admin status are hidden from non-admins, except on their own profile. Users may update or delete only their own account. Changes to admin status are admin-only. No client can set the admin flag through profile create or update.

### D4 — Ownership of user data

Every read and write of user-owned data must be limited to the authenticated owner. User-owned data is inventory (user gear registrations), containers and their links, loadouts, and loadout items. The rule includes parent/child membership, for example that an item belongs to the loadout it is addressed through. It must hold at the point where data is changed. Admins may override it only where explicitly stated. Private loadouts are visible only to their owner. Public loadouts are visible by slug without authentication.

### D5 — Token refresh

Roles in issued tokens reflect only the current stored state of the user. A demoted admin cannot refresh into an admin token.

## Context (not a requirement)

At the time of the decision there are only two registered users (the owner and a friend), and Google OAuth is restricted to specific test users. There is no live exposure. This explains the remediation's timing. It does not relax any requirement.

## References

- Engineering review: `gogear-api-engineering-review-2026-11-12.md` (external file, sections 3, 9 and 10).
