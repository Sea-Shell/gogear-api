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
