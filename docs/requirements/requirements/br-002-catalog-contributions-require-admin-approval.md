---
type: Behavioral Requirement
title: BR-002 Catalog contributions require admin approval (planned)
description: Planned, not implemented. Non-admin users may in future contribute catalog changes, which take effect only after admin approval.
tags: [catalog, admin-approval, planned, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# BR-002 — Catalog contributions require admin approval (planned)

**Status:** Planned. **Not yet implemented and not part of the current remediation.** **Basis:** sourced from anchor decision D1.

**Statement:**
When contributions are introduced, a catalog change proposed by a non-admin user shall take effect in the catalog only after an admin approves it.

**Scope limit:**
The product owner has decided only that contributions require admin approval. Everything else is unspecified and must not be inferred from this document. This includes how users propose changes, what can be proposed, how approval or rejection works, and what proposers see. Those decisions belong to the product owner when the feature is planned.

**Acceptance criteria (apply once the feature exists):**

- AC-1: A non-admin contribution never appears in, or changes, catalog data that other users read before an admin has approved it.
- AC-2: Until this feature is implemented, [BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) applies without exception. No partial contribution path is exposed to non-admins.

**Verification semantics:**
Now: verified through BR-001. No contribution path exists for non-admins. Later: submit a contribution as a non-admin and read the catalog as another user before and after admin approval. The requirement is falsified if the contribution is visible or effective before approval.

## Derived from

- [Access control and identity decisions — D1](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-001 Catalog changes are admin-only](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md)
