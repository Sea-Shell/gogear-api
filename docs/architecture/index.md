# GoGear API architecture

Technical realization decisions for accepted requirements. Requirements live in [/docs/requirements/](/docs/requirements/index.md); these documents decide how they are built.

## Decisions

- [ADR-001 Realizing category-driven container capability](/docs/architecture/decisions/adr-001-category-driven-container-capability.md): category container settings, one SQL view for the derived status, a database guard on links, and migration 000009 (BR-021, BR-020, BR-001 AC-6). Accepted; implemented. Its OQ-1 is closed and its seeding goes into migration 000010 by ADR-002 D-7.
- [ADR-002 Zero-touch release, deploy and container seeding](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md): nrkno semantic-release v5.1.6 at a pinned SHA, build and deploy guarded by `released == 'true'` and fed by `tag`, default bump rules, least-privilege release job, a non-cancelling concurrency group, digest-checked deploy with in-run pod and migration-version checks, and the BR-021 AC-5 seeding as migration 000010 (BR-022, BR-021 AC-5). Accepted; implemented and verified by [run 45061018365](https://github.com/Sea-Shell/gogear-api/actions/runs/45061018365) (2026-11-22). Amended by D-11 (pod identity from the Deployment template, pod spec and owner instead of the `imageID` digest): D-11 verified by the same run. Not yet exercised in production: I-4's "differs" branch.

- [ADR-003 Loadout places as rows, registration-backed items, and weights computed on read](/docs/architecture/decisions/adr-003-loadout-places-registration-items-and-computed-weight.md): one `loadout_places` table (Worn, Last minute, bags with per-trip parent and order), items that name the owner's registration with a snapshot when it is deleted, ticks and closed state as columns, and `total_weight` computed as on-your-back weight by views (BR-023 to BR-025, BR-027, BR-029, BR-030). Accepted; milestone A parts implemented; the milestone B parts (weights from measured values, consumable role, stage) are implemented in code (milestone B).
- [ADR-004 Expand-then-contract release of the loadout item contract](/docs/architecture/decisions/adr-004-expand-contract-release-of-the-loadout-item-contract.md): every milestone only adds to the API; gear-only item inserts keep working through a legacy path until a cutover merged after the deployed frontend adds items by registration (BR-022, BR-023 AC-1, AC-7, AC-8). Accepted; expand step (milestone A) implemented, cutover not.
- [ADR-005 Gear catalog states for contributions and private items, one visibility rule, and a merge that only repoints](/docs/architecture/decisions/adr-005-gear-catalog-states-contributions-and-private-items.md): `approved` / `private` / `waiting` / `not_approved` on the gear row, one visibility predicate for every catalog read, and merge or replace that changes only the registrations' gear (BR-002, BR-032 to BR-034, BR-037, OC-002). Accepted; not implemented.
- [ADR-006 Pre-migration backup and one-action restore inside the application](/docs/architecture/decisions/adr-006-pre-migration-backup-and-restore.md): plain-SQL version check before golang-migrate, a verified `VACUUM INTO` snapshot taken while no listener is open, three kept per kind, stdout announcement, restore through the `GOGEAR_RESTORE_BACKUP` env setting, dirty-state handling, and Q-8 and Q-9 as two policy constants (BR-038). Accepted; not implemented (task A6b, part of the milestone A merge).

## Designs

- [ARD-001 Loadout planning and packing: API realization](/docs/architecture/design/ard-001-loadout-planning-and-packing.md): how ADR-003 to ADR-005 compose, the requirement map, invariants, milestones A to E plus the cutover, and the migration sequence. Accepted; milestone A implemented, milestone B implemented in code (milestone B), milestones C to E and the cutover not.

## Specifications

- [ARS-001 Loadout loop schema, migrations and API contract](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md): migrations 000011 to 000014 and the cutover, the weight views, response shapes and every new or changed endpoint by milestone. Accepted; milestone A implemented, milestone B implemented in code (milestone B), milestones C to E and the cutover not.
- [ARS-002 Pre-migration backup and restore: startup sequence, names, messages and tests](/docs/architecture/spec/ars-002-pre-migration-backup-and-restore.md): startup order, `database.backup-dir`, `GOGEAR_RESTORE_BACKUP`, file names and state files, stdout lines, build and workflow changes, required tests and task A6b (BR-038). Accepted; not implemented.

## Bundle files

- [Change log](/docs/architecture/log.md)
