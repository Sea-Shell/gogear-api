---
type: Obligation Contract
title: OC-001 Owner-scoped data operations
description: Shared guarantee that every read or change of user-owned data checks the authenticated owner, and parent membership, as part of the same data operation, so no check-then-act gap or mismatched identifier can reach another user's data.
tags: [ownership, authorization, data-integrity, concurrency, security-remediation]
timestamp: 2026-11-21T00:00:00Z
---

# OC-001 — Owner-scoped data operations

**Status:** Accepted. **Priority:** P0 (review SEC-03 to SEC-06, API-02). **Basis:** anchor decision D4 says ownership must hold "at the data-mutation boundary", including parent/child membership. The other guarantees are derived from that. This contract states what the parties must agree on. It does not choose code structure, query form or storage mechanism.

**Why a contract:** Many independently written operations ([BR-009](/docs/requirements/requirements/br-009-self-only-account-deletion.md), [BR-012](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md) to [BR-015](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md)) rely on one shared meaning of "owned". The defects in the review come from one party checking one identifier while another party changes a different one.

**Participants (by responsibility):**
- *Authenticated identity*: establishes which user is making the request, from a verified service token only.
- *Request interpretation*: determines the target record, and its parent where the path names one, from the request path ([BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md)).
- *Data operation*: performs the read or change against stored data.

**Preconditions:**
- The request has passed authentication.
- The target is user-owned data: a gear registration, a container link, a loadout or a loadout item.

**Guarantees:**
- G-1: The owner checked is always the authenticated user. It never comes from the request body, a query parameter, or a value read earlier in the request that might have changed since.
- G-2: The data operation itself guarantees ownership. It affects or returns only records whose ownership resolves to the authenticated user, and, where a parent is named, only records that belong to that parent. The record that was checked and the record that was changed are always the same record, verified in the same indivisible operation. A separate earlier check is allowed, but it is never what the guarantee depends on.
- G-3: Ownership resolves through the chain of membership. A loadout item's owner is its loadout's owner. A container link's owner is its container registration's owner, and creating a link also requires the content registration to be owned ([BR-013](/docs/requirements/requirements/br-013-container-links-owner-scoped.md)).
- G-4: A changing operation succeeds only if it affected exactly the intended records. If it affected none, the outcome is "not available" ([BR-018](/docs/requirements/requirements/br-018-denial-outcomes.md)), never success.
- G-5: Dependent effects of one operation succeed or fail together with it. Examples: a loadout's total weight after an item change, the container links removed with a registration, and the data removed with an account. A refused or failed operation leaves no partial effect on any user's data.

**Semantic input/output:**
- Input: the authenticated user, the operation, the target identifier and the parent identifier, if any.
- Output: exactly one of: the result, limited to owned records; "not available" (the record does not exist, is not owned, or is not a member of the parent); or failure with no effect.

**Failure semantics:**
- "Not available" is the same outcome whether the record does not exist, is owned by someone else, or belongs to a different parent.
- A concurrent change, such as the parent being deleted, between any earlier check and the change gives "not available" or no effect. It never gives a change to a record the caller does not own.
- A storage failure partway through gives no effect (G-5), and the response is not a success.

**Invariant:**
- No operation by user U creates, returns, changes or deletes user-owned data whose ownership chain does not resolve to U. There are no admin exceptions unless a requirement explicitly states one, and none does at present.

**Verification semantics:**
Exercise the real request handling against real stored data, not only an isolated helper. For each operation in BR-012 to BR-015: (1) send foreign targets, mismatched parent/child pairs and conflicting body identifiers, and check that stored data is unchanged; (2) where the storage engine allows, delete or reassign the target between any preliminary check and the change, and check that no foreign record is touched; (3) inject a failure after the first effect, and check that no partial effect remains. Any foreign-record effect or partial effect falsifies the contract.

## Derived from

- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)

## Related

- [BR-018 Refused requests change nothing and use a defined status](/docs/requirements/requirements/br-018-denial-outcomes.md)
- [BR-017 Identity and privilege values in request bodies never take effect](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md)
