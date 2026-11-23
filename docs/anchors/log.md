---
type: Log
title: Anchors change log
description: Chronological record of changes to the anchors bundle.
tags: [log]
timestamp: 2026-11-23T00:00:00Z
---

# Anchors change log

## 2026-11-23

- **Amendment:** [Release and deployment decisions, RD3](/docs/anchors/release-and-deployment.md) records the owner's answer to Q-8: stop without upgrading if the automatic pre-migration backup fails. Q-9 was not answered.

## 2026-11-22

- **Amendment:** [Release and deployment decisions](/docs/anchors/release-and-deployment.md) gains RD3. The application backs up its own database before it changes the schema, with no user step, because copying databases in Kubernetes is hard and an external database may be out of the user's reach. The owner's words are recorded verbatim, with the reading labelled. It leads to BR-038.
- **Amendment:** [Loadout planning and packing decisions](/docs/anchors/loadout-planning-and-packing.md) gains LP8. The owner chose private items for everyone, which can be sent to the catalog later; shared loadouts leave them out. LP8 answers requirements Q-6 and supersedes LP2's "no private items" sentence, which is struck through. The owner's share-time idea is recorded as a note, not a requirement. DC-2 (merge) is now accepted by the owner, on condition that everything the user provided on their own items is kept. [Access control and identity decisions](/docs/anchors/access-control-and-identity.md) D1's note is updated to match.
- **Creation:** [Loadout planning and packing decisions](/docs/anchors/loadout-planning-and-packing.md) records the owner's decisions from the session that accepted the frontend design "Pick, Sort, Pack": LP1 yes to the API work, LP2 no private items (contributions usable at once by the contributor, visible to others after approval), LP3 Last minute and Worn are per-loadout places, LP4 copy options, LP5 barcode scanning as a future way to add gear. Designer's choices DC-1 (a not-approved contribution stays usable by its contributor) and DC-2 (an admin merge moves the contributor's registrations to the existing gear and keeps their measured weight) are recorded as open to owner change. The same day the owner answered three follow-up questions, recorded with their words and General's interpretations: LP6 (the same item may be in a loadout more than once; the closet keeps a count), LP7 (QR codes; a code must not repeat within one user's gear, but need not be unique across the catalog), and a partial reopening of LP2: whether private items exist is open again (requirements Q-6).
- **Amendment:** [Access control and identity decisions](/docs/anchors/access-control-and-identity.md) D1 gains a note that LP2 refines its contribution workflow.

- **Creation:** [Release and deployment decisions](/docs/anchors/release-and-deployment.md) records the owner's rule that deploying a new version needs no manual step and happens through the existing workflow (RD1), and its consequence that data changes shipped with a release need no follow-up by hand (RD2). RD2 supersedes requirements provisional default P-14.
- **Amendment:** [Access control and identity decisions](/docs/anchors/access-control-and-identity.md) D11 gains a note on starting values: container settings are seeded from the old per-gear flags (RD2), not set by hand after deploy.
- **Amendment:** [Access control and identity decisions](/docs/anchors/access-control-and-identity.md) gains decision D11: the category or sub-category decides whether gear can be a container, and D9's "a container can store anything" is about what a container holds. The owner's statement is recorded verbatim; the detailed rules were adopted on General's proposal and stay open to owner change. Supersedes requirements provisional default P-11.

## 2026-11-21

- **Creation:** [Access control and identity decisions](/docs/anchors/access-control-and-identity.md) records product-owner decisions D1–D5 for the security remediation.
- **Creation:** Bundle [index](/docs/anchors/index.md) and this log.
- **Amendment:** [Access control and identity decisions](/docs/anchors/access-control-and-identity.md) gains follow-up decisions D6 (permanent first admin), D7 (email is profile data, not identity), D8 (editable profile fields), D9 (containers can store anything) and D10 (issued service tokens are not revoked early), which answer the requirements bundle's open questions Q-1 to Q-5. The context note now records that all stored data is generated test data.
