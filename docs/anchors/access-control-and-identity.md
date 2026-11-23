---
type: Anchor
title: Access control and identity decisions (security remediation)
description: Product-owner decisions on catalog editing rights, Google identity binding, user directory visibility, ownership of user data, token refresh roles and lifetime, the permanent first admin, email as profile data, editable profile fields, container nesting and category-based container capability for the GoGear API security remediation.
tags: [access-control, identity, authorization, security-remediation, product-decision]
timestamp: 2026-11-22T00:00:00Z
---

# Access control and identity decisions

**Status:** Accepted by the product owner on 2026-11-21 (D1–D5, and the follow-up decisions D6–D10 the same day). D11 was added on 2026-11-22: the owner's statement is accepted, and its detailed rules were adopted on General's proposal and stay open to owner change. Relayed to the requirements author by the orchestrating session for the review-mitigation work.

**Why this exists:** The engineering review of 2026-11-12 (revision `6fb972b`, sections 3, 9 and 10) found that ordinary users can make themselves admin, change or delete other users' data, and link Google identities by email. Fixing these defects needs explicit rules about who may do what. This document records the rules the product owner decided. The requirements in `/docs/requirements/` derive from it.

## Decisions

### D1 — Catalog editing

The catalog is gear, manufacturers, categories and top categories. Users may eventually contribute to the catalog, but only through admin approval. Interim rule for this remediation: catalog changes (create, update, delete) are admin-only. The approval-based contribution is planned and not yet implemented. Its workflow is not specified beyond "contributions require admin approval".

*Refined on 2026-11-22 by [LP2](/docs/anchors/loadout-planning-and-packing.md):* a user's new gear goes into the shared catalog, the contributor can use it at once, and other users see it only after an admin approves it. Users may also keep private items that only they see, and send them to the catalog later ([LP8](/docs/anchors/loadout-planning-and-packing.md)). The workflow rules from the accepted design are recorded there: designer's choice DC-1 (open to owner change) and DC-2 (accepted by the owner with a condition).

### D2 — Google identity

Accounts are bound to the stable Google subject identifier (`sub`). An existing account with no stored `sub` gets it filled in once, automatically, on the next sign-in, matched by email. This backfill is allowed only when Google reports the email as verified (`email_verified` is true). After binding, sign-in is by `sub` only. An email match alone never links an account.

### D3 — User directory

Any authenticated user may view other users' profiles. Email and admin status are hidden from non-admins, except on their own profile. Users may update or delete only their own account. Changes to admin status are admin-only. No client can set the admin flag through profile create or update.

### D4 — Ownership of user data

Every read and write of user-owned data must be limited to the authenticated owner. User-owned data is inventory (user gear registrations), containers and their links, loadouts, and loadout items. The rule includes parent/child membership, for example that an item belongs to the loadout it is addressed through. It must hold at the point where data is changed. Admins may override it only where explicitly stated. Private loadouts are visible only to their owner. Public loadouts are visible by slug without authentication.

### D5 — Token refresh

Roles in issued tokens reflect only the current stored state of the user. A demoted admin cannot refresh into an admin token.

## Follow-up decisions (answers to open questions, 2026-11-21)

The product owner answered open questions Q-1 to Q-5 of the requirements bundle. The requirements author chose the precise rules the answers need; those choices are marked as provisional defaults in the requirements, not here.

### D6 — The first admin is permanent (answers Q-1)

"The first admin is the permanent admin." No admin may remove admin status from the first admin. Other admin-status changes stay as in D3. The precise meaning of "first admin" and what happens on demotion or deletion attempts are in BR-019.

### D7 — Email is profile data, not identity (answers Q-2)

The stored profile email is profile data only. It is never used to identify a user. The single exception is the one-time backfill match of D2. Whether the stored email follows a changed Google email does not matter to the product owner, so there is no requirement to synchronise it.

### D8 — Editable profile fields (answers Q-5)

Users may edit every field of their own profile except the binding to their Google account (the Google subject), their admin status and their user ID.

### D9 — Containers can store anything (answers Q-4)

A container can store anything, including other containers. Nesting depth is unlimited. Which registrations can be containers is refined by D11. A gear item inside one container can itself hold items. The physical-consistency rules this needs are in BR-020.

### D10 — Issued service tokens are not revoked early (answers Q-3)

This concerns the GoGear service JWTs that the API issues at sign-in and refresh, not Google tokens. After a user is demoted or deletes their account, service tokens issued before that keep their role and stay valid until they expire (60 minutes by default). No server-side revocation and no per-request check of the stored role is required. Refresh already issues only the current stored role and refuses deleted accounts (D5, BR-006). This is an accepted decision, not a provisional default.

### D11 — The category decides what can be a container (2026-11-22, refines D9)

Owner's statement: "it should be the category or sub-category that dictates if an item is a container… now it's the item itself that needs to be marked as a container, which does not scale well if users are going to add items." The owner confirmed that D9's "a container can store anything" means a container can hold anything. It does not mean that anything can be a container. The earlier reading "any registration can act as a container" (provisional default P-11) was a misunderstanding and is superseded.

Adopted on General's proposal, open to owner change:

- A registration can hold other registrations only if its gear's catalog classification is a container classification. What a container holds stays unrestricted: any registration, including other containers, at unlimited depth (D9). One direct container per registration and no cycles stay (BR-020).
- Admins set container capability on catalog categories, as part of the admin-only catalog (D1). A sub-category inherits its top category's setting unless the sub-category has its own explicit value.
- A gear item's own container mark is no longer the source of truth. Gear gets its container status from its classification, so gear added later by users gets it automatically.

Starting values (added 2026-11-22, from [release decision RD2](/docs/anchors/release-and-deployment.md)): existing data is not left for admins to mark by hand after deploy. The settings are seeded from the old per-gear container flags when the rule takes effect. The detailed seeding rule is in BR-021 AC-5 (provisional default P-18, adopted on General's proposal, open to owner change).

The resulting requirements are BR-021 and the amended BR-020 and BR-001.

## Context (not a requirement)

At the time of the decision there are only two registered users (the owner and a friend), and Google OAuth is restricted to specific test users. There is no live exposure. This explains the remediation's timing. It does not relax any requirement.

All data in the database is generated test data, not real user data (stated by the product owner on 2026-11-21). So preserving existing stored data is not a product obligation for these changes. This adds no requirement and relaxes none.

## References

- Engineering review: `gogear-api-engineering-review-2026-11-12.md` (external file, sections 3, 9 and 10).
