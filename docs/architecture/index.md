# GoGear API architecture

Technical realization decisions for accepted requirements. Requirements live in [/docs/requirements/](/docs/requirements/index.md); these documents decide how they are built.

## Decisions

- [ADR-001 Realizing category-driven container capability](/docs/architecture/decisions/adr-001-category-driven-container-capability.md): category container settings, one SQL view for the derived status, a database guard on links, and migration 000009 (BR-021, BR-020, BR-001 AC-6). Accepted; implemented. Its OQ-1 is closed and its seeding goes into migration 000010 by ADR-002 D-7.
- [ADR-002 Zero-touch release, deploy and container seeding](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md): nrkno semantic-release v5.1.6 at a pinned SHA, build and deploy guarded by `released == 'true'` and fed by `tag`, default bump rules, least-privilege release job, a non-cancelling concurrency group, digest-checked deploy with in-run pod and migration-version checks, and the BR-021 AC-5 seeding as migration 000010 (BR-022, BR-021 AC-5). Accepted; not implemented.

## Bundle files

- [Change log](/docs/architecture/log.md)
