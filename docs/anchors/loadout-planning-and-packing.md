---
type: Anchor
title: Loadout planning and packing decisions
description: Product-owner decisions of 2026-11-22 for the API support behind the reworked frontend's Pick, Sort, Pack loadout loop - yes to the API work, shared catalog with contributor-first use, private items for everyone (LP8, replaces LP2's no-private-items rule), at-the-door as a per-loadout place, copying loadouts, barcode and QR scanning, the same item more than once in a loadout with a closet count, and codes unique per user - plus a designer choice open to owner change (DC-1) and the owner-accepted merge rule (DC-2).
tags: [loadouts, packing, weight, catalog-contributions, barcode, product-decision]
timestamp: 2026-11-22T00:00:00Z
---

# Loadout planning and packing decisions

**Status:** Accepted by the product owner on 2026-11-22, in the design session that accepted the frontend design "Loadout planning and packing — Pick, Sort, Pack" (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`). Relayed to the requirements author by the orchestrating session (General); in the handoff the decisions are numbered O1 to O5. Two further rules are the designer's choices: they are recorded below as such and stay open to owner change.

**Why this exists:** The reworked frontend lets a user build a trip's loadout from their own gear while watching the weight, sort it into bags, and pack it bag by bag on a phone. Today the API stores a loadout item only as catalog gear, a quantity and a note, and keeps no packing progress, no user weights and no trip state. The owner wants the API to provide what the design needs. The requirements in `/docs/requirements/` (BR-023 to BR-037, OC-002, and amendments to earlier requirements) derive from this document.

**What the design relies on (summary, not a requirement):** every picked item is in exactly one place for the trip (a bag, Worn, Last minute, or not in a bag yet); weights say where they come from (weighed by you, the maker's figure, or a guess); packing progress survives closing the app and switching devices; nothing that is not saved is shown as saved.

## Decisions

### LP1 — Yes to the API work (handoff O1)

The owner said yes to the API work the accepted design needs (the design's capability needs N1 to N10).

### LP2 — Contributions are usable at once by the contributor (handoff O2)

~~There are no private items.~~ *(Superseded by LP8 the same day.)* Anything a user adds goes into the shared catalog ("the pool") so that other users can find and use it. While it waits for admin approval, the person who added it can use it at once, in their closet and their loadouts, marked as waiting. Other users see it only after an admin approves it.

This keeps decision [D1](/docs/anchors/access-control-and-identity.md) ("contributions require admin approval") and BR-002 AC-1, and it decides what BR-002 left open: what proposers see.

**Changed later on 2026-11-22 (see LP8):** asked how generic consumables such as "Food, 3 days" or "Water 1 L" fit "no private items", the owner said: "consumables is a good point. i did not think about that. i guess private items needs to be considered anyways." The owner then chose private items for everyone (LP8). Still in force from LP2: gear a user sends to the shared catalog is usable at once by them and visible to others only after approval.

### LP3 — "At the door" is a per-loadout place (handoff O3)

"At the door" is not a property of gear. It depends on the trip, so it is a place in a loadout, called **Last minute**. Every loadout also has a built-in **Worn** place.

### LP4 — Copying a loadout (handoff O4)

Copying is offered from the loadout list. The copy options are:

- gear and quantities (always copied);
- bags and where each item goes;
- notes and the weight goal.

Ticks and closed bags are never copied, so the new trip starts unpacked. The original loadout is never changed.

### LP5 — Barcode scanning is a wanted future way to add gear (handoff O5)

Scanning a barcode or QR code is a wanted way to add gear. Photo recognition may come later. Scanning has the lowest priority of this work: it may be specified now and built last.

### LP6 — The same item can be in a loadout more than once; the closet keeps a count (answer to handoff Q-B, 2026-11-22)

Owner's words: "same item 2 times, yes. perhaps a item-count in the closet should exist as well?"

*General's interpretation, open to owner correction:* (a) one loadout may contain the same owned item more than once, for example placed in different bags; (b) a closet registration gets a count of how many of that item the user owns. Whether a loadout that uses more than the owned count is warned or refused was not decided by the owner; the requirements default to a warning, following the design principle "never block, always show" (provisional).

### LP7 — QR codes; a code must not repeat within one user's gear (answer to handoff Q-D, 2026-11-22)

Owner's words: "use QR codes. No, not realy. its not a requirement from me, but a user should not have the same QR code multiple times. that breaks the function."

*General's interpretation, open to owner correction:* QR codes are in scope, including a user's own printed QR stickers on their bags and items. Codes need not be unique across the whole catalog: many owners share a maker's product code. Within one user's closet, a code must identify at most one item, because a duplicate breaks scanning.

### LP8 — Private items for everyone (answer to requirements Q-6, 2026-11-22; supersedes LP2's "no private items")

Owner's words, choosing option 1 ("private items for everyone; can be sent to the catalog later") of requirements open question Q-6: "A. This on the other hand poses a problem when later we look at shareable loadouts. but we could just drop private items when its shared. so it could be a simple question to answer."

*General's interpretation, open to owner correction:*

- Any user can add gear only for themselves (a *private item*). Nobody else sees it, and it never goes to the admin queue. The user can use it in their closet and loadouts like any gear.
- The user can later send a private item to the catalog. From then on it is a contribution, and admin approval (D1, BR-002) applies.
- Shared and public loadouts leave out private items, in the same way as contributions that are not approved yet.

**Note, not a requirement:** the owner sees the sharing experience as "a simple question to answer" when a loadout is shared, for example whether to send its private items to the catalog first. That is future frontend design work. No API requirement for a share-time prompt follows from it now.

### Clarification on "places" (2026-11-22)

The owner first read "places" as trip destinations. General clarified that a place is where an item travels within the loadout (a bag, Worn, Last minute, or none). The owner expects places to be stored on their own but leaves that to the architect. This adds no decision.

## Designer's choices

These rules come from the accepted design (flow F8). DC-1 stays open to owner change. The owner accepted DC-2 explicitly, with a condition.

- **DC-1 — A contribution that is not approved stays usable by its contributor.** It stays in their closet and loadouts so a rejection never breaks a packed trip. It is still not visible to others. The contributor can edit it and send it again, or replace it with an existing catalog item.
- **DC-2 — An admin's "merge into existing" moves the contributor's use to the existing item.** When an admin finds that a contribution duplicates an existing catalog item, the contributor's registrations, and so every loadout item using them, then refer to the existing item. The contributor's own measured weight is kept.
  **Accepted by the owner on 2026-11-22, with a condition.** Owner's words: "yes, this is ok. as long as custom info provided by the use, like wheight is kept." *General's interpretation:* a merge keeps everything the user provided on their own items. That covers the measured weight and its source, the closet count, the maximum container weight, own codes, container links, and in loadouts the notes, quantities, places, bag nesting and order, ticks, closed states and consumable roles. Only the gear the registration refers to changes.

## Context (not a requirement)

- The reworked frontend is meant to be the API's only client, so a documented breaking change to the loadout item contract is acceptable. This is General's statement in the handoff, not an owner decision recorded here.
- All stored data is generated test data ([access-control anchor, context note](/docs/anchors/access-control-and-identity.md)). Keeping existing loadout items is not a product obligation. Every data change must still apply automatically on deploy ([RD2](/docs/anchors/release-and-deployment.md)).
