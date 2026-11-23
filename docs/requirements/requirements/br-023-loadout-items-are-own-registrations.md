---
type: Behavioral Requirement
title: BR-023 Loadout items refer to the owner's own closet registrations
description: Every loadout item refers to exactly one gear registration of the loadout's owner; one registration may back several items of the same loadout; an item follows its registration when the registration's gear changes.
tags: [loadouts, loadout-items, inventory, user-gear, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-023 — Loadout items refer to the owner's own closet registrations

**Implementation:** **partially implemented (milestone A, 2026-11-22).** Loadout items are stored as the owner's registrations (migration `000011`, tests in `pkg/api`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone A): AC-2 to AC-4 and AC-8 are implemented, and AC-5 is implemented for an item that follows its registration. **Pending the cutover** ([ADR-004](/docs/architecture/decisions/adr-004-expand-contract-release-of-the-loadout-item-contract.md) D-3): AC-1 (the deprecated `gear_id` path is still accepted and can keep a legacy item that reads its gear live) and AC-7 (stored legacy items are not yet bound or removed). AC-6 and the merge and replace case of AC-5 wait for milestone D. Before milestone A, an item stored only catalog `gear_id`, `quantity` and `notes` (Swagger at `f82fb7b`). **Status:** Accepted. **Priority:** P1 (milestone A; the board, Pack and measured weights depend on it). **Basis:** the statement and AC-1 are sourced from the accepted design's capability need N2 (the owner said yes to it, anchor decision [LP1](/docs/anchors/loadout-planning-and-packing.md)): an item must be "the right physical item, its measured weight, and bags that are real containers". AC-2 is sourced from the owner's answer to Q-B ([LP6](/docs/anchors/loadout-planning-and-packing.md), "same item 2 times, yes"); its details are provisional default P-20. AC-3 keeps current behavior. AC-4 and AC-5 are derived from N2 and DC-2. AC-6 is sourced from LP2. AC-7 is derived from the anchor context note and RD2. AC-8 is a specified choice based on the handoff's compatibility note.

**Terms:**
- *Registration* (closet item): a gear registration as in [BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md). It stands for one physical item, or for a number of identical items the owner has ([BR-036](/docs/requirements/requirements/br-036-closet-count-of-a-registration.md)).
- *Gear of a registration*: the gear record the registration refers to: catalog gear, the owner's own contribution ([BR-002](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md)), or the owner's private item ([BR-037](/docs/requirements/requirements/br-037-private-items.md)).
- *Used in a loadout*: a registration is used in a loadout if at least one item of the loadout refers to it, or if it is one of the loadout's bags ([BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md)).

**Statement:**
Every loadout item shall refer to exactly one registration owned by the loadout's owner. The API shall take the item's physical identity, gear and weights from that registration.

**Acceptance criteria:**

- AC-1: Adding an item to a loadout names one registration. The item is created only if the caller owns the loadout ([BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md)) and the registration. A registration that does not exist or belongs to someone else is refused with the "not found" outcome of [BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md), and nothing is created. An item cannot be created from catalog gear alone, without a registration. This applies to every operation that adds items, including any import operation that is kept.
- AC-2: One registration may back several items of the same loadout. Each item has its own quantity, notes, place, tick and consumable role. Example: registration R7 is "tent pegs" with a closet count of 8; loadout L1 has one R7 item with quantity 4 in the tent bag and another R7 item with quantity 4 in the Last minute place. Adding a registration that is already used in the loadout is never refused for that reason. A registration that is a bag of the loadout may also back items of it (BR-024 AC-2). Using more than the closet count is a warning, not a refusal (BR-036 AC-3). **[Sourced from LP6. That an item is never merged with another item of the same registration is provisional default P-20 — open to product-owner change]**
- AC-3: Quantity works as today: a positive whole number, 1 if not given. Anything else is refused with 400. The quantity multiplies the registration's unit weight ([BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md)).
- AC-4: Reading an item reports which registration it refers to and that registration's current gear (name, maker, category) and weights, as defined in BR-029.
- AC-5: An item follows its registration. If the registration's gear changes (an admin merge, [BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md), or a contributor's replace, [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md)), the next read of the item reports the new gear. The item's quantity, notes, place, tick and consumable role stay as they were.
- AC-6: A registration of the caller's own contribution that is not approved yet, or of the caller's own private item, can be used like any other registration ([BR-002](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) AC-3, [BR-037](/docs/requirements/requirements/br-037-private-items.md) AC-3).
- AC-7: Once this requirement takes effect on a database, every stored loadout item refers to a registration of its loadout's owner. Stored items that refer only to catalog gear are converted or removed automatically, as part of the release, with no manual step ([BR-022](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md), anchor RD2). Removing an item that cannot be matched to a registration of its owner is allowed, because stored data is generated test data. How items are matched is a realization choice. (Derived from the anchor context note and RD2.)
- AC-8: The loadout item contract may change incompatibly (for example, an item no longer being identified by catalog gear alone). The new contract is documented in the generated API description (Swagger). (Specified choice, based on the handoff's note that the reworked frontend is the only client.)
- AC-9: Admins get no exception. **[Provisional default P-3]**

**Verification semantics:**
Use real request handling and stored data. As A, add an item naming A's registration (created), B's registration (404, nothing created), an unknown registration (404) and only a catalog gear ID (refused, nothing created). Add the same registration twice to one loadout with different places and quantities: two items exist, each with its own values; changing or ticking one leaves the other unchanged. Set quantity 0 and −1: 400. Merge or replace the registration's gear, then read both items: they report the new gear and their other values are unchanged. Run the release on a database with old-style items: afterwards every item refers to a registration of its loadout's owner, and no manual step was needed. Any stored item that refers to another user's registration, or to no registration, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP1, LP2, LP6, DC-2](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N2 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-015 Loadout items are reachable only through their own, owned loadout](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md)
- [BR-024 Every loadout item is in exactly one place, and bags nest for the trip](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md)
- [BR-030 Deleting a registration keeps the loadouts that use it consistent](/docs/requirements/requirements/br-030-deleting-a-registration-keeps-loadouts-consistent.md)
- [BR-036 A registration records how many the owner has](/docs/requirements/requirements/br-036-closet-count-of-a-registration.md)
- [OC-001 Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md)
