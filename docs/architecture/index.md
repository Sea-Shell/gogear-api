# GoGear API architecture

Technical realization decisions for accepted requirements. Requirements live in [/docs/requirements/](/docs/requirements/index.md); these documents decide how they are built.

## Decisions

- [ADR-001 Realizing category-driven container capability](/docs/architecture/decisions/adr-001-category-driven-container-capability.md): category container settings, one SQL view for the derived status, a database guard on links, and migration 000009 (BR-021, BR-020, BR-001 AC-6). Accepted; implemented. Its OQ-1 is closed and its seeding goes into migration 000010 by ADR-002 D-7.
- [ADR-002 Zero-touch release, deploy and container seeding](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md): nrkno semantic-release v5.1.6 at a pinned SHA, build and deploy guarded by `released == 'true'` and fed by `tag`, default bump rules, least-privilege release job, a non-cancelling concurrency group, digest-checked deploy with in-run pod and migration-version checks, and the BR-021 AC-5 seeding as migration 000010 (BR-022, BR-021 AC-5). Accepted; implemented, pending verification of the deploy path.

- [ADR-003 Loadout places as rows, registration-backed items, and weights computed on read](/docs/architecture/decisions/adr-003-loadout-places-registration-items-and-computed-weight.md): one `loadout_places` table (Worn, Last minute, bags with per-trip parent and order), items that name the owner's registration with a snapshot when it is deleted, ticks and closed state as columns, and `total_weight` computed as on-your-back weight by views (BR-023 to BR-025, BR-027, BR-029, BR-030). Accepted; not implemented.
- [ADR-004 Expand-then-contract release of the loadout item contract](/docs/architecture/decisions/adr-004-expand-contract-release-of-the-loadout-item-contract.md): every milestone only adds to the API; gear-only item inserts keep working through a legacy path until a cutover merged after the deployed frontend adds items by registration (BR-022, BR-023 AC-1, AC-7, AC-8). Accepted; not implemented.
- [ADR-005 Gear catalog states for contributions and private items, one visibility rule, and a merge that only repoints](/docs/architecture/decisions/adr-005-gear-catalog-states-contributions-and-private-items.md): `approved` / `private` / `waiting` / `not_approved` on the gear row, one visibility predicate for every catalog read, and merge or replace that changes only the registrations' gear (BR-002, BR-032 to BR-034, BR-037, OC-002). Accepted; not implemented.

## Designs

- [ARD-001 Loadout planning and packing: API realization](/docs/architecture/design/ard-001-loadout-planning-and-packing.md): how ADR-003 to ADR-005 compose, the requirement map, invariants, milestones A to E plus the cutover, and the migration sequence. Accepted; not implemented.

## Specifications

- [ARS-001 Loadout loop schema, migrations and API contract](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md): migrations 000011 to 000014 and the cutover, the weight views, response shapes and every new or changed endpoint by milestone. Accepted; not implemented.

## Bundle files

- [Change log](/docs/architecture/log.md)
