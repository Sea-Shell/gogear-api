---
type: Behavioral Requirement
title: BR-001 Catalog changes are admin-only
description: Only admins may create, update or delete gear, manufacturers, categories and top categories; all authenticated users may still read the catalog.
tags: [catalog, authorization, admin, security-remediation]
timestamp: 2026-11-22T00:00:00Z
---

# BR-001 — Catalog changes are admin-only

**Implementation:** implemented in wave 4. Gear, manufacturer, top category and category create, update and delete are registered behind one admin-only route middleware (`RequireAdmin`, `RegisterCatalogRoutes`); a non-admin gets 403 and nothing changes, a missing or invalid token gets 401. List, search and get stay open to every authenticated user. Updates change only the fields sent, by the path ID (a JSON `null` counts as not sent); a body ID that differs from the path is 400 (P-2, BR-017); an unknown ID is 404 (BR-018). Delete works on gear with NULL dimensions and names the ID in its message. Since wave 6 a delete of a row that is still referenced (gear in registrations or loadout items, a category, manufacturer or top category used by gear) is 409 and changes nothing, and a create or update naming a missing top category, category or manufacturer is 400. AC-6 (added 2026-11-22, anchor decision D11) is implemented: `top_category_is_container` (bool) on top categories and `category_container` (`container`, `not_container` or `inherit`; `inherit` removes the explicit setting) on categories are written through the same admin-only update and insert routes, so a non-admin gets 403 (401 without a valid token) and nothing changes, and every authenticated user reads the settings (and the read-only effective value `category_is_container`) on get and list. A JSON `null` still means not sent, so a client that echoes a read response back removes nothing; any other `category_container` value is 400 and, as a malformed body, is decided before the 404 for an unknown ID. Tests: `TestCategoryContainerSettings_AdminOnlyWrites` and `TestCategoryContainerSettings_RepresentationRules` in `pkg/api/category_container_api_test.go`, and E2E-001 (`TestE2E001_CategoryDrivenContainersThroughTheAPI`). **Status:** Accepted. **Priority:** P1 (review SEC-10). **Basis:** sourced from anchor decision D1 (interim rule); AC-6 from D11.

**Statement:**
When an authenticated user asks to create, update or delete a catalog record, the API shall perform the change only if that user is an admin. The catalog is gear, manufacturers, categories and top categories.

**Acceptance criteria:**

- AC-1: A non-admin request to create, update or delete a gear item, manufacturer, category or top category is refused with the "admin role required" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md). The stored catalog is unchanged afterwards.
- AC-2: The same request made by an admin succeeds as it does today.
- AC-3: The rule applies identically to all four catalog resource types. There is no catalog type that a non-admin can change.
- AC-4: Reading the catalog (list, search, get) remains available to every authenticated user. This requirement does not restrict reads.
- AC-5: "Admin" means a token that authorizes admin operations under [BR-006](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md). A non-admin cannot become an admin for this purpose by any request body content.
- AC-6: The container settings of top categories and sub-categories ([BR-021](/docs/requirements/requirements/br-021-container-capability-from-category.md)) are catalog data. Setting, changing or removing them, including when a category is created, is a catalog change under AC-1 and AC-2; reading them is a catalog read under AC-4.

**Verification semantics:**
For each of the four catalog types and each of create, update and delete, send the request with a real non-admin token and with a real admin token. The requirement is falsified if any non-admin request returns a success status or if any stored catalog record differs afterwards.

## Derived from

- [Access control and identity decisions — D1, D11](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-021 The catalog category decides whether gear can be a container](/docs/requirements/requirements/br-021-container-capability-from-category.md) — defines the container settings covered by AC-6.
- [BR-002 Catalog contributions require admin approval (planned)](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) — will replace the interim rule for non-admin contributions once implemented.
