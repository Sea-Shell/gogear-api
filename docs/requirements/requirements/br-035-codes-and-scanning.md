---
type: Behavioral Requirement
title: BR-035 Gear and registrations carry scannable codes, and a scan finds them
description: Catalog gear can carry a maker's product code (shared, not unique), a registration can carry the owner's own code (unique within that owner's closet, for example a printed QR sticker), and a lookup by an exact code returns the caller's own matching item and every visible gear with that product code.
tags: [catalog, inventory, barcode, qr-code, scanning, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-035 — Gear and registrations carry scannable codes, and a scan finds them

**Implementation:** not implemented. **Status:** Accepted with provisional conditions. **Priority:** P3 (milestone E; anchor decision [LP5](/docs/anchors/loadout-planning-and-packing.md): specified now, built last). **Basis:** product codes on catalog gear and the lookup (AC-1, AC-4) are sourced from capability need N9 of the accepted design ("Barcode on catalog gear and a lookup by code") and LP5. That product codes need not be unique, and the own code that is unique within one owner's closet (AC-2), are sourced from the owner's answer to Q-D as interpreted by General ([LP7](/docs/anchors/loadout-planning-and-packing.md), interpretation open to owner correction). AC-3 and AC-6 are provisional default P-33. AC-5 records the recommended client resolution; the accepted design has no flow yet for scanning one's own code (gap for the designer).

**Terms:**
- *Code*: the text read from a barcode or a QR code. The API treats both kinds the same.
- *Product code*: a code on a gear record, usually the maker's barcode. Everyone who owns that product shares it.
- *Own code*: a code the owner gives one of their registrations, for example a QR sticker they printed and put on a bag.

**Statement:**
The API shall let catalog gear carry a product code and a registration carry an own code, and shall let any authenticated user look up, by an exact code, their own matching registration and the visible gear with that product code.

**Acceptance criteria:**

- AC-1: A gear record has no product code or one. Admins set, change and remove it on approved gear ([BR-001](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) AC-8). A contributor sets, changes and removes it on their own unapproved contribution ([BR-032](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md), [BR-034](/docs/requirements/requirements/br-034-contributors-follow-up-their-contributions.md)), and an owner on their own private item ([BR-037](/docs/requirements/requirements/br-037-private-items.md)). Product codes need not be unique: several gear records may have the same one, and none is refused for that. **[Non-uniqueness: sourced from LP7 (interpretation)]**
- AC-2: A registration has no own code or one. Only its owner can set, change, remove or read it ([BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md)). Within one owner's registrations a code identifies at most one registration: setting a code that another registration of the same owner already has is refused with 409, and nothing changes. This holds when two such requests run at the same time. Different owners may use the same code. Example: A's "Exos 58" has own code `GG-A-0001`; setting `GG-A-0001` on A's "Lid" is 409; B may use `GG-A-0001` on B's own gear. **[Sourced from LP7 (interpretation): "a user should not have the same QR code multiple times. that breaks the function."]**
- AC-3: Codes match exactly after leading and trailing whitespace is removed. Matching is case-sensitive, and no other normalization is done (for example, a 12-digit UPC and the same number with a leading zero are different codes). A blank code means no code. **[Provisional default P-33 — open to product-owner change]**
- AC-4: A lookup by a code reports, and labels which is which: (a) the caller's own registration with that own code, if there is one; and (b) every gear record visible to the caller ([OC-002](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)) with that product code, each with its name, maker, category and catalog weight. It never reports another user's registration or own code, or another user's unapproved contribution or private item; such a match gives the same result as no match. A code with no match gives an empty result that the client can tell apart from a failure.
- AC-5: Recommended client resolution (not an API obligation): a match of the caller's own registration is shown first ("this is your Exos 58"), because an own code names one physical item; otherwise a single product-code match goes to the add-to-closet confirm step, several matches let the user choose, and no match offers search, adding to the catalog, or scanning again (F7).
- AC-6: When an admin merges a contribution into gear G ([BR-033](/docs/requirements/requirements/br-033-admins-decide-catalog-contributions.md) AC-4), the contribution's product code passes to G if G has none; otherwise it is dropped. Own codes are never changed by a merge. **[Provisional default P-33]**
- AC-7: An own code is owner data. It is never copied into the catalog and is removed with its registration and with the owner's account ([BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md)).

**Out of scope:** several product codes per gear record (for example colour variants with different barcodes), photo recognition, and how codes are printed or encoded.

**Verification semantics:**
As admin, give approved gear G1 and G2 the same product code P: both accepted. As A, give "Exos 58" own code Q, then give "Lid" Q: 409. As B, give B's registration Q: accepted. As A, look up Q: A's "Exos 58" only, labelled as own. Look up P: G1 and G2, with weights. Look up P with surrounding spaces: the same result; in lower case when stored in upper case: no match. As A, contribute X with product code R; as B, look up R: empty, the same as for an unknown code; as A: X. Merge X into G1 (which has P): G1 keeps P. Delete A's "Exos 58", then look up Q as A: no own match. Any lookup that reveals another user's registration, own code or unapproved contribution, or two registrations of one owner with the same own code, falsifies the requirement.

## Derived from

- [Loadout planning and packing decisions — LP5, LP7](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D1, D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N9, flow F7, scenario SC-005 (frontend repository, `docs/design/`)

## Related

- [BR-032 Users contribute new gear to the catalog, and it is in their closet at once](/docs/requirements/requirements/br-032-users-contribute-new-gear-to-the-catalog.md)
- [OC-002 Visibility of gear outside the shared catalog](/docs/requirements/obligation-contracts/oc-002-visibility-of-unapproved-contributions.md)
