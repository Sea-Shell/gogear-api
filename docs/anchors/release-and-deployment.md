---
type: Anchor
title: Release and deployment decisions
description: Product-owner decision that a merge to main releases and deploys a new version of the GoGear API to the owner's Kubernetes server through the existing release workflow, with no manual step, and the consequences for data changes that ship with a release, including the application's own backup before it changes the database schema.
tags: [release, deployment, ci-cd, product-decision]
timestamp: 2026-11-23T00:00:00Z
---

# Release and deployment decisions

**Status:** Accepted by the product owner on 2026-11-22. Relayed verbatim to the requirements author by the orchestrating session for the release-pipeline work.

**Why this exists:** The release workflow last deployed successfully on 2026-06-24 (run 28101251749, found by research on 2026-11-22). No later merge to `main` is known to have reached the server, so changes merged after that date, including the access-control remediation and the container-capability change, are presumed not to be running there. The owner wants the release path to work again, and wants every future version to reach the server without anyone doing anything after the merge.

## Decisions

### RD1 — A merge to main deploys with no manual step

Owner's statements:

- "There should be no manual steps when deploying a new version."
- "Server should be deployed by workflow. There is a tailscale setup there which connects to my server and deploys to kubernetes."

The owner's server configuration and the deployment setup already exist. The release work must not add steps for the owner or any other person to perform.

The precise rules (what counts as a manual step, the no-release case, the evidence) are in [BR-022](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md).

### RD2 — Data changes that ship with a release need no follow-up by hand

This is a consequence of RD1, recorded here because it changes an earlier provisional default. When a release changes how stored data is interpreted, the release itself must bring the data into a usable state. An admin may still change data afterwards, but the product must not depend on an admin doing so before it is usable.

Applied to container capability (anchor decision [D11](/docs/anchors/access-control-and-identity.md)): the category container settings are seeded from the old per-gear container flags when BR-021 takes effect, instead of starting empty and waiting for an admin to mark categories. This supersedes requirements provisional default P-14. The detailed seeding rule was adopted on General's proposal and stays open to owner change (BR-021 AC-5, provisional default P-18).

### RD3 — The application protects its own upgrades with a backup (2026-11-22)

Owner's words: "The backup should probably be handled in the application, no? since it is hosted in kuberntes it not so easy to do copys of databases. and, if the database is hosted outside, then it might even not be within reach for the user. I belive a application should handle all upgrade procedures it self, and not rely on the user for much."

Why it came up: migration 000011 (loadout planning, milestone A) cannot be undone by deploying the older image. The older image does not start on schema version 11, and the down migration loses data and is not on the release path. So a copy of the database from before the upgrade is the only way back.

*General's and the requirements author's reading, open to owner correction:*

- Before the application changes the database schema, it makes a backup of the database itself, automatically. Nobody has to copy database files by hand, in Kubernetes or elsewhere.
- Getting back to a backup should also need as little manual work as possible.
- The principle is broader than SQLite: the application, not its user, owns the safety of its own upgrades. Today the application runs only on a SQLite file on the server's disk. A database hosted outside the application does not exist yet, so no requirement is written for one now.

The rules are in [BR-038](/docs/requirements/requirements/br-038-backup-before-schema-migration.md). It extends RD1 and RD2: a release still needs no manual step, and it must now also leave a way back.

#### Q-8 follow-up — Stop without upgrading if the backup fails (2026-11-23)

In the milestone A discussion, the owner was asked one question: “if the automatic backup fails, should the app stop without upgrading?” The owner replied, “yes, as you recommend.” The coordinating session relayed the question and answer for PR #490.

The owner chose **Refuse**: if the automatic pre-migration backup fails, the application stops without upgrading rather than migrating with a warning. This confirms the existing behavior in [BR-038 AC-8](/docs/requirements/requirements/br-038-backup-before-schema-migration.md). It does not answer Q-9, which concerns a migration that fails *after* a good backup.

## Context (not a requirement)

The repository requires every GitHub Action to be pinned to a full commit SHA. The release action that the workflow used until now no longer runs its release step upstream, which is the likely reason releases stopped (research of 2026-11-22). This explains the timing of RD1. It adds no requirement.

## References

- Release workflow on `main` at `a8526c3`: `.github/workflows/release.yaml` (evidence of the existing path, not product authority).
