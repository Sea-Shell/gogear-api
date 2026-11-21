---
type: Log
title: Requirements change log
description: Chronological record of changes to the GoGear API requirements bundle.
tags: [log]
timestamp: 2026-11-21T00:00:00Z
---

# Requirements change log

## 2026-11-21

- **Creation:** First requirements baseline for access control and identity. It is derived from [anchor decisions D1–D5](/docs/anchors/access-control-and-identity.md) and addresses review findings SEC-01 to SEC-10.
  - Behavioral requirements [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) to [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). BR-002 is planned, not implemented.
  - Obligation contract [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md).
  - Provisional defaults P-1 to P-7 and open questions Q-1 to Q-5 are recorded in the [index](/docs/requirements/index.md).
- **Creation:** Bundle [index](/docs/requirements/index.md) and this log.

- **Implementation status:** Wave 1 implements [BR-006](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md) to [BR-011](/docs/requirements/requirements/br-011-direct-user-creation-admin-only.md) and the user, role and refresh parts of BR-017 and BR-018. Status notes were added to those requirements and an implementation-status section to the [index](/docs/requirements/index.md). Requirement text is unchanged.

- **Implementation status:** Wave 2 implements [BR-014](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md), [BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md), [BR-016](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md), [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) for loadouts and items, and the loadout parts of BR-017 and BR-018. Status notes were added to those documents and to the [index](/docs/requirements/index.md). Requirement text is unchanged. Provisional default P-1 now applies to loadouts: foreign loadouts and items return 404 instead of 403.

- **Implementation status:** Wave 3 implements [BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md), [BR-013](/docs/requirements/requirements/br-013-container-links-owner-scoped.md), [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) for registrations and container links, and the registration and container parts of BR-017 and BR-018. Status notes were added to those documents and to the [index](/docs/requirements/index.md). Requirement text is unchanged. Provisional default P-3 now applies to inventory: admins can no longer list another user's registrations. Open question Q-4 is still open and unenforced.

- **Implementation status:** Wave 4 implements [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) and the catalog parts of [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md) and [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). [BR-002](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) is still planned. Status notes were added to those documents and to the [index](/docs/requirements/index.md). Requirement text is unchanged.

- **Implementation status:** Review fixes on waves 1 to 4. Status notes corrected in [BR-007](/docs/requirements/requirements/br-007-user-profile-visibility.md) (name fields: email-like names hidden and unsearchable; registration and container responses only carry the caller's own names), [BR-014](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md) (import row counts, foreign public loadouts), [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md) (no part remains; account deletion was wave 1, catalog wave 4) and [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) (owner-bound item list, account deletion status, immediate transactions with a busy timeout). Requirement text is unchanged.

- **Implementation status:** Second review pass on wave 4. [BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md) now says "implemented" (it said "partly implemented" while also saying everything is covered), and the lowercase "wave 4" sentence starts in BR-017 and [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md) are capitalised. [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) and the [index](/docs/requirements/index.md) now state that a JSON `null` in an update body is treated as absent. The index records that a catalog update of an unknown ID is 404 before any 400. Requirement text is unchanged.

- **Implementation status:** Wave 5 implements [BR-003](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md), [BR-004](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md) and [BR-005](/docs/requirements/requirements/br-005-first-google-sign-in.md). Status notes were added to those documents and to the [index](/docs/requirements/index.md), including the decision that an account whose owner changed its email is never eligible for backfill. Requirement text is unchanged. Provisional defaults P-6 and P-7 now apply to Google sign-in.

- **Implementation status:** Wave 6 adds the database integrity behind [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md), [BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md) and [BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md) to [BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md): enforced foreign keys, migration 000005 constraints, WAL, and a required Go CI gate. Status notes were added to those documents, [OC-001](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md) and the [index](/docs/requirements/index.md), which also records the status policy (404 missing gear, 400 other bad references, 409 referenced catalog delete and duplicate link). The index and [BR-007](/docs/requirements/requirements/br-007-user-profile-visibility.md) now state that non-admins are shielded from names containing `@`, by the same rule in the response and the search. Requirement text is unchanged.
