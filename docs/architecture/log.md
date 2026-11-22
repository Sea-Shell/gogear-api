---
type: Log
title: Architecture change log
description: Chronological record of changes to the GoGear API architecture bundle.
tags: [log]
timestamp: 2026-11-22T00:00:00Z
---

# Architecture change log

## 2026-11-22

- **Creation:** [ADR-001 Realizing category-driven container capability](/docs/architecture/decisions/adr-001-category-driven-container-capability.md) decides how BR-021 and amended BR-020 AC-1 are built: a `top_category_is_container` bool and a three-valued `category_container` field (`"inherit"` removes a sub-category's setting); a read-only derived `gear_is_container`; views `category_container_setting` and `gear_container_capability` as the only place the rule is computed; link triggers that refuse non-capable containers; the capability read inside the immediate link transaction (AC-8); and a new migration 000009 that renames the old per-gear flag to `gearIsContainerLegacy` instead of editing 000008.
- **Creation:** Bundle [index](/docs/architecture/index.md) and this log.
- **Status change:** [ADR-001](/docs/architecture/decisions/adr-001-category-driven-container-capability.md) is now marked implemented: migration 000009, the derived status in every response, the 409 on links and the admin API for the settings are in place. The decisions are unchanged.
- **Creation:** [ADR-002 Zero-touch release, deploy and container seeding](/docs/architecture/decisions/adr-002-zero-touch-release-deploy-and-seeding.md) decides how BR-022 and amended BR-021 AC-5 are built: the nrkno reusable workflow v5.1.6 at `a68fb147`, outputs `released` and `tag` with the guard `needs.release.outputs.released == 'true'`, no `.semrelrc.yml` (the defaults match past releases, T-1), release permissions `contents: write` and `pull-requests: read`, a workflow-wide concurrency group that never cancels a running run, an in-run check of pod digest, restarts and migration version, a local non-publishing rehearsal, the stale `deployment.yaml` pin left as is, and seeding in a new migration 000010 that sets only sub-categories without an explicit setting.
- **Amendment:** [ADR-001](/docs/architecture/decisions/adr-001-category-driven-container-capability.md) OQ-1 closed by P-18 and ADR-002 D-7; its D-5 seeding statement now lives in migration 000010 with an added guard that never overwrites an explicit setting.
