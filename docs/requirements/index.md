# GoGear API requirements — access control and identity

## In brief

**Goal:** Every user can see and change only what the product owner allows. Users manage only their own inventory, containers, loadouts and account. Only admins change the shared catalog and admin roles. A Google login always reaches the same account, and never someone else's.

**Example of success:** Alice and Bob are ordinary users. Alice can see Bob's name in the user directory but not his email. If Alice sends a request to edit Bob's loadout, delete his gear registration, or move one of his loadout items, she gets "not found" and nothing of Bob's changes. Alice can't make herself admin. If the owner demotes an admin, that admin's next token refresh produces an ordinary token. Anyone can open Bob's loadout through its public link while it is public. After he makes it private, the link returns "not found".

**Most important limit:** These requirements cover only the access-control and identity decisions of 2026-11-21 ([anchor](/docs/anchors/access-control-and-identity.md)). Implementation is under way: BR-006 to BR-011 (users, roles, token refresh) are implemented as of wave 1, BR-014 to BR-016 (loadouts, loadout items, public loadouts) as of wave 2, and BR-012 and BR-013 (inventory registrations, container links) as of wave 3; the rest are not yet implemented. The engineering review of revision `6fb972b` found most of them violated. Data integrity, performance, operations and API redesign findings from the same review are out of scope.

**Choices the product owner may still change:** the provisional defaults P-1 to P-7 below.

**Context, not a requirement:** At the time of writing there are two registered users, and Google OAuth is limited to test users, so there is no live exposure.

## Terms

- **Authenticated user:** the user identified by a valid service token issued by the API.
- **Admin:** a user whose stored admin status is true. Tokens reflect this status as described in BR-006.
- **Catalog:** gear, manufacturers, categories and top categories. It is shared by all users.
- **User-owned data:** gear registrations (inventory), container links, loadouts and loadout items.
- **Owner:** for a registration or loadout, the user it belongs to. For a loadout item, the owner of its loadout. For a container link, the owner of its container registration.
- **Google subject (`sub`):** Google's stable account identifier in a validated ID token.

## Behavioral requirements

| ID | Requirement | Decision | Review finding |
|---|---|---|---|
| BR-001 | [Catalog changes are admin-only](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) | D1 | SEC-10 |
| BR-002 | [Catalog contributions require admin approval — **planned, not implemented**](/docs/requirements/requirements/br-002-catalog-contributions-require-admin-approval.md) | D1 | — |
| BR-003 | [Google sign-in identifies accounts by Google subject only](/docs/requirements/requirements/br-003-google-sign-in-resolves-by-subject.md) | D2 | SEC-08 |
| BR-004 | [Legacy accounts get their Google subject filled in once, by verified email](/docs/requirements/requirements/br-004-legacy-account-subject-backfill.md) | D2 | SEC-08 |
| BR-005 | [First Google sign-in creates a new account only when no existing account claims the email](/docs/requirements/requirements/br-005-first-google-sign-in.md) | D2 | SEC-08 |
| BR-006 | [Issued tokens carry only the role the account currently has](/docs/requirements/requirements/br-006-issued-roles-reflect-current-state.md) | D5 | SEC-09, SEC-01 |
| BR-007 | [Other users' email and admin status are hidden from non-admins](/docs/requirements/requirements/br-007-user-profile-visibility.md) | D3 | SEC-02 |
| BR-008 | [Users update only their own profile](/docs/requirements/requirements/br-008-self-only-profile-update.md) | D3 | SEC-01, SEC-02 |
| BR-009 | [Users delete only their own account, and deletion removes all their data or nothing](/docs/requirements/requirements/br-009-self-only-account-deletion.md) | D3, D4 | SEC-02 |
| BR-010 | [Admin status changes are admin-only](/docs/requirements/requirements/br-010-admin-status-changes-admin-only.md) | D3 | SEC-01 |
| BR-011 | [Direct user creation is admin-only and creates non-admin accounts](/docs/requirements/requirements/br-011-direct-user-creation-admin-only.md) | D3 | SEC-01 |
| BR-012 | [Gear registrations are readable and changeable only by their owner](/docs/requirements/requirements/br-012-inventory-registrations-owner-scoped.md) | D4 | SEC-05, SEC-07 |
| BR-013 | [Container links involve only the caller's own registrations](/docs/requirements/requirements/br-013-container-links-owner-scoped.md) | D4 | SEC-06 |
| BR-014 | [Loadouts are readable and changeable only by their owner](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md) | D4 | SEC-03 |
| BR-015 | [Loadout items are reachable only through their own, owned loadout](/docs/requirements/requirements/br-015-loadout-items-scoped-to-owned-parent.md) | D4 | SEC-04 |
| BR-016 | [Public loadouts are readable by slug without signing in; private ones are not](/docs/requirements/requirements/br-016-public-loadouts-by-slug.md) | D4 | — |
| BR-017 | [Identity and privilege values in request bodies never take effect](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md) | D3, D4 | SEC-01, SEC-03, SEC-05 |
| BR-018 | [Refused requests change nothing and use a defined status](/docs/requirements/requirements/br-018-denial-outcomes.md) | D3, D4 | — |

## Obligation contracts

- OC-001 — [Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md): ownership and parent membership are checked in the same operation that reads or changes the data.

## Quality scenarios

None. The decisions contain no measurable quality target. The adversarial and concurrent cases are covered as behavior by OC-001 and the BR acceptance criteria.

## Implementation status

- **Wave 1 (implemented):** BR-006, BR-007, BR-008, BR-009, BR-010, BR-011, and the user, role and refresh parts of BR-017 and BR-018. Decisions made while implementing, within the requirements:
  - Admin-status changes use `POST /api/v1/users/{user}/admin` with `{"user_is_admin": true|false}` (admin only; 404 for an unknown user).
  - Profile update changes only the fields present in the body (username, name, email). Omitted fields and the password are never touched.
  - A non-admin search by email in the user directory is refused with 403, because it would reveal other users' emails (BR-007). Searches by username and name only match what the response shows.
  - Where a username or name contains the user's email (Google sign-in does this), it is blanked for non-admin viewers of other users.
  - Startup refuses empty or equal `jwt-audience` and `jwt-admin-audience`.
- **Wave 2 (implemented):** BR-014, BR-015, BR-016, OC-001 for loadouts and items, and the loadout parts of BR-017 and BR-018. Decisions made while implementing, within the requirements:
  - Loadout update changes only the fields present in the body (name, description, public flag, slug) and maintains `updatedAt`. An update with no fields is 400.
  - A body `loadout_id`, `loadout_item_id` or `user_id` that differs from the path or caller is 400 (P-2); a matching or absent value is accepted. A foreign or unknown target is 404 before any body conflict is reported.
  - Quantity below 1 is 400 on item insert and update. An absent quantity on insert is 1. No database CHECK constraint yet (integrity wave).
  - A blank or whitespace slug is stored as NULL. A slug already used by another loadout is 409.
  - Deleting a loadout also deletes its items in the same transaction, because foreign keys are not enforced.
  - Import, item insert, update and delete are all-or-nothing together with the total-weight recalculation; a failed recalculation fails the request.
  - List endpoints return `[]` when empty.
  - Not changed: gear existence is not checked when adding items (needs foreign-key enforcement, a later wave).
- **Wave 3 (implemented):** BR-012, BR-013, OC-001 for registrations and container links, and the registration and container parts of BR-017 and BR-018. Decisions made while implementing, within the requirements:
  - Listing another user's registrations is 403 for everyone (admins too, P-3). A non-numeric user ID is 400.
  - Registration update changes only `max_container_weight` (absent keeps it, explicit null clears it); gear and owner never change. A body `usergear_user_id` or `usergear_registration_id` that differs from the caller or path is 400 (P-2), after the 404 check.
  - Registration insert refuses a body `usergear_user_id` that is not the caller (400). Response bodies of the existing endpoints are unchanged.
  - Single-record get takes no query filters. List filters (`topCategory`, `category`, `manufacture`) are bound integers; non-numeric values are ignored. Several filters still combine with AND, as before.
  - A registration in several containers is listed once, with its lowest-numbered link into one of the owner's own containers.
  - Container list, insert and delete: `:container` in the list route is the container registration ID; in the delete route it is the link ID (`container_registration_id`). Insert needs both `user_container_id` and `user_gear_registration_id` (400 otherwise). Self-link is 400 (after the ownership check, so a foreign ID is 404).
  - Deleting a registration also removes every link that names it as container or content, in the same transaction, because foreign keys are not enforced.
  - Bad paging values are 400 instead of 500.
  - Not changed: gear existence is not checked on insert (needs foreign-key enforcement), and Q-4 rules (nesting, longer cycles, duplicate links, what may hold gear) are not enforced.
- **Not yet implemented:** BR-001 to BR-005.

## Provisional defaults (open to product-owner change)

Where the decisions were silent, the requirements author chose the least-privilege option. The product owner may change any of these. Each is marked where it applies.

- **P-1** — A user-owned record that does not exist, belongs to someone else, or is addressed through the wrong parent returns **404**. All three cases give an identical response (BR-018). This changes today's 403 for foreign loadouts and items, and the existing tests that expect 403.
- **P-2** — A conflicting owner, target or parent ID, or admin status, in a request body is **rejected with 400** rather than silently ignored (BR-017).
- **P-3** — **No admin exceptions** for user-owned data or for other users' profiles: admins cannot read or change other users' inventory, containers or loadouts, or update or delete other accounts. Admins keep only the catalog (BR-001) and admin-status changes (BR-010). This removes today's admin ability to list another user's registrations. Affects BR-008, BR-009 and BR-012 to BR-015.
- **P-4** — Self-deletion **removes all of the user's data** (registrations, container links, loadouts, items), **all-or-nothing** (BR-009).
- **P-5** — The direct user-creation operation is **admin-only** (BR-011).
- **P-6** — Google sign-in is **refused with 403** in these cases: the email is not verified and no account is bound to the token's subject; the verified email belongs to an account already bound to another subject; or several unbound accounts share the email. In these cases no account is linked or created (BR-004, BR-005).
- **P-7** — The backfill email match is **exact and case-sensitive, after trimming whitespace** (BR-004).

## Open questions (none block the current remediation)

- **Q-1** — Can an admin remove their own admin status, including the last admin? Owner: product owner. Not specified, so it is neither required nor forbidden.
- **Q-2** — When a user's Google email changes, should the stored profile email follow it? Owner: product owner. Sign-in is not affected (BR-003). Only profile data is.
- **Q-3** — Must tokens issued before a demotion stop working before they expire? Owner: product owner, with the architect for the mechanism. D5 covers refresh only. Today the risk is bounded by token lifetime (60 minutes by default).
- **Q-4** — Container integrity rules: whether a registration can hold gear, nesting, cycles, duplicates. Owner: product owner. Out of scope here (BR-013).
- **Q-5** — Which profile fields a user may edit, including their own email, which is the backfill matching key. Owner: product owner. BR-008 keeps today's editable fields.

## Bundle files

- [Change log](/docs/requirements/log.md)
