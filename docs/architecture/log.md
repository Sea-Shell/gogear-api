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
