---
type: Behavioral Requirement
title: BR-025 Packing progress is saved per loadout and shared across devices
description: The API stores, per loadout, whether each item is ticked and whether each bag, Worn and Last minute is closed; ticks and closing are set to a stated value, never block, survive moves, are never copied, and a tick for an item that is gone is refused without side effects.
tags: [loadouts, packing, progress, sync, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-025 — Packing progress is saved per loadout and shared across devices

**Implementation:** not implemented. The current frontend keeps ticks in browser session storage only. **Status:** Accepted. **Priority:** P1 (milestone A; the design does not release Pack until this exists). **Basis:** the statement, AC-1 to AC-3 and AC-6 are sourced from capability need N3 of the accepted design ("ticked per item, closed per place … same on phone and laptop"; owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md)) and its Pack flow F5 (closing with items left is allowed after a confirmation; "Open bag again" keeps ticks). AC-7 is derived from the design's offline rule ("sent when the connection returns, oldest first … if a tick is refused, drop it"). The "never copied" part of AC-1 is sourced from [LP4](/docs/anchors/loadout-planning-and-packing.md). AC-2's "set, not toggle" rule, AC-3's list of closable places, AC-4, AC-5 and AC-8 are provisional defaults P-23 and P-24, marked where they apply.

**Terms:**
- *Tick*: an item is ticked when the owner has marked it as packed. A ticked item that has no place means "packed somewhere".
- *Closable place*: a bag, Worn, or Last minute. "No place" cannot be closed.
- *Closed*: the owner has marked a closable place as packed.

**Statement:**
The API shall store, per loadout, whether each item is ticked and whether each closable place is closed, so that every device and session of the owner sees the same packing progress.

**Acceptance criteria:**

- AC-1: A new item is not ticked. A new bag or Last minute place, and Worn of a new loadout, are open. A copied loadout starts with nothing ticked and nothing closed ([BR-031](/docs/requirements/requirements/br-031-copy-a-loadout.md)).
- AC-2: The owner can set an item to ticked or to not ticked, in any place, including no place. A request states the value it wants; it does not toggle. Repeating a request gives the same stored state and succeeds. **[Set-to-value: provisional default P-23]**
- AC-3: The owner can set a closable place to closed or to open, whatever the ticks of its items. Closing a place with items that are not ticked is allowed. Closing or opening never changes any tick, and never changes the closed state of any other place, including bags inside it or around it. A request to close "no place" is refused with 400. **[Closable places and independence of closed states: provisional default P-23]**
- AC-4: Moving an item, or changing its quantity, notes or consumable role, never changes its tick or any closed state. Moving an item into a closed bag is allowed and leaves the bag closed. **[Provisional default P-23]**
- AC-5: Removing an item removes its tick. Removing a bag or Last minute removes its closed state. An item or place added again later starts as in AC-1.
- AC-6: After a progress change has been answered with success, every later read of the loadout, from any device or session of the owner, returns it. Example (SC-002): on the phone the owner closes "Lid" and ticks three items in "Exos 58"; the phone dies; on the laptop "Lid" is closed and the three items are ticked.
- AC-7: A tick request for an item that is no longer in the loadout, or for a loadout that is gone, is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md) and changes nothing. Because each request names one item, the client can tell which tick failed and name the item to the user. The API decides requests in the order they arrive; when two requests set the same item or place, the one decided last is the stored state. The API does not reorder requests by any time the client reports. Consequence (accepted with the design's oldest-first rule): a tick sent late from a device that was offline overrides an earlier change made on another device. **[Last-decided-wins: provisional default P-24 — open to product-owner change]**
- AC-8: Ticking and closing are allowed whatever the loadout's stage, and never change the stage ([BR-028](/docs/requirements/requirements/br-028-loadout-stage-dates-and-goal.md)). **[Provisional default P-24]**
- AC-9: Progress is part of the loadout. Only the loadout's owner can read or change it ([BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md)); anyone else gets the "not found" outcome and nothing changes. Admins get no exception (P-3).

**Out of scope:** storing unsent ticks on the device. That is the client's job, and the design allows it only while labelled as waiting to sync.

**Verification semantics:**
Use real request handling and stored data and two separate sessions of the owner. In session 1, tick three items, close a bag that still has unticked items, and close Worn; in session 2, read the loadout: exactly those ticks and closed states. Send the same tick twice: same state, both succeed. Open the bag again: the ticks are unchanged. Move a ticked item into another bag and into no place, change its quantity: still ticked, closed states unchanged. Close a parent bag: the bag inside it stays open. Close "no place": 400. Remove an item in session 2, then send a tick for it from session 1: 404, and nothing else changed. Set an item ticked in session 1, then not ticked in session 2: the stored state is not ticked. Tick as another user: 404. Copy the loadout: the copy has no ticks and no closed places. Any tick or closed state lost, changed by an unrelated request, or not shown to a second session falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1, LP4](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N3, flow F5, "Failure, recovery and interruption", scenario SC-002 (frontend repository, `docs/design/`)

## Related

- [BR-024 Every loadout item is in exactly one place, and bags nest for the trip](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md)
- [BR-031 Copy a loadout in one all-or-nothing operation](/docs/requirements/requirements/br-031-copy-a-loadout.md)
