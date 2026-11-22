---
type: Anchor
title: Release and deployment decisions
description: Product-owner decision that a merge to main releases and deploys a new version of the GoGear API to the owner's Kubernetes server through the existing release workflow, with no manual step, and the consequences for data changes that ship with a release.
tags: [release, deployment, ci-cd, product-decision]
timestamp: 2026-11-22T00:00:00Z
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

## Context (not a requirement)

The repository requires every GitHub Action to be pinned to a full commit SHA. The release action that the workflow used until now no longer runs its release step upstream, which is the likely reason releases stopped (research of 2026-11-22). This explains the timing of RD1. It adds no requirement.

## References

- Release workflow on `main` at `a8526c3`: `.github/workflows/release.yaml` (evidence of the existing path, not product authority).
