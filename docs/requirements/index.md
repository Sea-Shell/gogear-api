# GoGear API requirements — access control and identity

## In brief

**Goal:** Every user can see and change only what the product owner allows. Users manage only their own inventory, containers, loadouts and account. Only admins change the shared catalog and admin roles. A Google login always reaches the same account, and never someone else's.

**Example of success:** Alice and Bob are ordinary users. Alice can see Bob's name in the user directory but not his email. If Alice sends a request to edit Bob's loadout, delete his gear registration, or move one of his loadout items, she gets "not found" and nothing of Bob's changes. Alice can't make herself admin. If the owner demotes an admin, that admin's next token refresh produces an ordinary token. Anyone can open Bob's loadout through its public link while it is public. After he makes it private, the link returns "not found".

**Most important limit:** These requirements cover only the access-control and identity decisions of 2026-11-21 ([anchor](/docs/anchors/access-control-and-identity.md)). Implementation is under way: BR-006 to BR-011 (users, roles, token refresh) are implemented as of wave 1, BR-014 to BR-016 (loadouts, loadout items, public loadouts) as of wave 2, BR-012 and BR-013 (inventory registrations, container links) as of wave 3, BR-001 (admin-only catalog) as of wave 4, and BR-003 to BR-005 (Google identity binding) as of wave 5; BR-002 is planned, not implemented. The follow-up decisions D6 to D10 of the same day add BR-019 (permanent first admin, not implemented) and BR-020 (container nesting, partly implemented), and amend BR-004, BR-006, BR-008 to BR-010, BR-012, BR-013 and BR-018. Each requirement marks its amended parts "not implemented" where that applies. Wave 6 adds the database integrity that backs these rules (foreign keys, container-link and quantity constraints) and a required Go CI gate. The engineering review of revision `6fb972b` found most of them violated. Performance and API redesign findings from the same review are out of scope.

**Choices the product owner may still change:** the provisional defaults P-1 to P-12 below, and the edited-email rule of BR-004 AC-8 and AC-9, which was adopted on General's recommendation without an explicit owner choice.

**Context, not a requirement:** At the time of writing there are two registered users, and Google OAuth is limited to test users, so there is no live exposure. All stored data is generated test data, not real user data, so preserving existing data is not a product obligation (anchor context note). This adds no requirement.

## Terms

- **Authenticated user:** the user identified by a valid service token issued by the API.
- **Admin:** a user whose stored admin status is true. Tokens reflect this status as described in BR-006.
- **Catalog:** gear, manufacturers, categories and top categories. It is shared by all users.
- **User-owned data:** gear registrations (inventory), container links, loadouts and loadout items.
- **Owner:** for a registration or loadout, the user it belongs to. For a loadout item, the owner of its loadout. For a container link, the owner of its container registration.
- **Google subject (`sub`):** Google's stable account identifier in a validated ID token.
- **Service token:** the GoGear JWT the API issues at sign-in and refresh. It is not a Google token.
- **Permanent admin:** the one account designated as the first admin in BR-019. It can never lose admin status or be deleted through the API.
- **Email:** profile data only. It identifies an account only in the one-time backfill match of BR-004 (anchor decision D7).

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
| BR-019 | [The first admin is a permanent admin](/docs/requirements/requirements/br-019-permanent-first-admin.md) — **not implemented** | D6 | — |
| BR-020 | [Containers nest to any depth, with one direct container per registration and no cycles](/docs/requirements/requirements/br-020-container-nesting-consistency.md) — **partly implemented** | D9 | SEC-06 |

## Obligation contracts

- OC-001 — [Owner-scoped data operations](/docs/requirements/obligation-contracts/oc-001-owner-scoped-data-operations.md): ownership and parent membership are checked in the same operation that reads or changes the data.

## Quality scenarios

None. The decisions contain no measurable quality target. The adversarial and concurrent cases are covered as behavior by OC-001 and the BR acceptance criteria.

## Implementation status

- **Wave 1 (implemented):** BR-006, BR-007, BR-008, BR-009, BR-010, BR-011, and the user, role and refresh parts of BR-017 and BR-018. Decisions made while implementing, within the requirements:
  - Admin-status changes use `POST /api/v1/users/{user}/admin` with `{"user_is_admin": true|false}` (admin only; 404 for an unknown user).
  - Profile update changes only the fields present in the body (username, name, email); a JSON `null` is treated as absent. Omitted fields and the password are never touched.
  - A non-admin search by email in the user directory is refused with 403, because it would reveal other users' emails (BR-007). Searches by username and name only match what the response shows.
  - Where a username or name contains `@` (Google sign-in stores the email there), it is blanked for non-admin viewers of other users, and a non-admin search cannot match it. The rule uses `@` only, in the response and in the SQL search, so the two cannot disagree.
  - Startup refuses empty or equal `jwt-audience` and `jwt-admin-audience`.
- **Wave 2 (implemented):** BR-014, BR-015, BR-016, OC-001 for loadouts and items, and the loadout parts of BR-017 and BR-018. Decisions made while implementing, within the requirements:
  - Loadout update changes only the fields present in the body (name, description, public flag, slug); a JSON `null` is treated as absent. The update maintains `updatedAt`. An update with no fields is 400.
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
  - Not changed: gear existence is not checked on insert (needs foreign-key enforcement), and the container rules of former open question Q-4 (nesting, longer cycles, duplicate links, what may hold gear) are not enforced. Q-4 is now answered by BR-020.
- **Wave 4 (implemented):** BR-001, and the catalog parts of BR-017 and BR-018. BR-002 stays planned. Decisions made while implementing, within the requirements:
  - One route middleware (`RequireAdmin`) guards every catalog create, update and delete; the routes are registered by `RegisterCatalogRoutes`, shared by `main` and the tests, so a handler cannot forget the check. Reads are unchanged.
  - Catalog updates are explicit partial `UPDATE`s: only fields present in the body change (a JSON `null` is treated as absent), and an empty body is 400. A body ID that differs from the path ID is 400 (P-2). An unknown ID is 404, checked before the body-ID conflict and the empty-body checks.
  - Catalog delete reads only the record name, answers 404 for an unknown ID, and reports the ID rather than a pointer. Its message text changed to `success! <Type> with ID <n> and name <name> was deleted`.
  - Request and config logging (review SEC-11, not a requirement): debug config output reports secrets only as set/unset. The request log is written after the handler with the final status, latency, route template and the number of query parameters, and never query names or values. A request whose handler panics is logged as a 500, the panic handler does not write the query, and gin runs in release mode unless `GIN_MODE` is set.
  - Not changed: catalog foreign keys are not enforced, so deleting a category or manufacturer still leaves gear pointing at it (integrity wave).
- **Wave 5 (implemented):** BR-003, BR-004, BR-005 (review SEC-08). Decisions made while implementing. All are within the requirements except the planted-email rule, which went beyond the requirement text (an earlier version of this list wrongly called it "within the requirements"). It is now accepted as BR-004 AC-8:
  - Storage is a nullable `users.googleSubject` with a unique index (migration 000004), plus `users.userEmailEdited` (default 0). The migration applies to existing databases and leaves every existing account unbound, so the next Google sign-in of each existing user backfills it.
  - **Planted email:** a profile update that changes the email sets `userEmailEdited`, and such an account is never eligible for backfill. Otherwise a user could set their email to someone else's and capture that person's first Google sign-in. An excluded account that holds the email still blocks creation of a new account for it (403), because BR-005 allows creation only when no account uses the email. The edit flag is not cleared; there is no way yet to restore eligibility. BR-004 AC-9 now requires an admin-only way to clear it (not implemented).
  - A token without a subject is 401 (treated as an invalid token). The P-6 refusals are 403. A token without an email that matches no bound subject is 400, as before.
  - Sign-in by a bound subject needs no email and no `email_verified`; the stored account's name, email and admin status are never updated from the token.
  - A sign-in that creates an account stores the Google email as username, name fallback and email, as before.
  - Not changed: the Swagger annotations on the callback, and Google-subject handling outside sign-in (no endpoint unbinds or rebinds).
- **Wave 6 (implemented):** database integrity behind BR-001 and BR-009 to BR-015 (review COR-03, COR-05, OPS-01); no new product behavior. Decisions made while implementing:
  - Foreign keys are enforced on every connection (`_foreign_keys=on` in the DSN). Startup runs `PRAGMA foreign_key_check` and logs one warning per table and parent with the number of orphan rows. Nothing is deleted or repaired, and orphans do not stop startup; enforcement only refuses new invalid writes. The two existing users may already have orphans.
  - Status policy for invalid references: gear that is not in the catalog is **404** on registration insert, loadout item insert and import (import stays all-or-nothing). Any other invalid reference in a write is **400**. Deleting a catalog row that other rows still reference (gear in registrations or loadout items, categories or manufacturers used by gear) is **409** with a message; the row stays. This closes the "gear existence is not checked" and "catalog foreign keys are not enforced" notes of waves 2 to 4.
  - Cascades: container links go with their registration and loadout items with their loadout (`ON DELETE CASCADE`); users and catalog rows are never cascaded. The explicit deletes of waves 1 to 3 are kept, children before parents.
  - Migration 000005 (triggers and indexes, no table rebuild, because a rebuild cannot run with foreign keys enforced inside the migration transaction): `loadout_items.quantity` must be a positive integer; a registration cannot be linked to itself; a container link is unique per (container, content); blank loadout slugs become NULL; indexes on `user_gear_registrations(userId)`, `(gearId)` and the container-link content column. A duplicate link insert is **409**.
  - Legacy data: quantities of 0 or below are **clamped to 1**, with the original values kept in `audit_000005_clamped_loadout_item_quantity` and the totals of the affected loadouts recomputed. Only identical duplicate links are deleted. Existing self-links and orphans are left unchanged for review. The down migration removes the constraints and indexes but does not undo data changes.
  - Database settings: file databases use the WAL journal (`_journal_mode=WAL`) so concurrent readers and writers do not hit "database is locked". Startup refuses an empty `database.file`.
  - CI: `.github/workflows/go.yaml` runs `go mod verify`, `go build`, `go vet`, `go test -race -count=1` and `govulncheck` on pull requests and pushes to main. Making it a required check is a GitHub branch-protection setting, not part of this repository.
  - Still open at the end of wave 6: the container rules of former Q-4 (now BR-020: one direct container, longer cycles, removing existing self-links and other violating links), repair of existing orphans, `totalWeight` after catalog weight edits, and a Swagger drift check in CI.
- **Not yet implemented:** BR-002 (planned); BR-019 (permanent first admin, with its refusals in BR-009 AC-6, BR-010 AC-2 and BR-018); BR-004 AC-9 (admin-only clearing of the edited-email mark); BR-020 AC-3 to AC-8 (one direct container, longer cycles, concurrency, cleanup of violating stored links); repair of pre-existing orphan rows.

## Provisional defaults (open to product-owner change)

Where the decisions were silent, the requirements author chose the least-privilege option. The product owner may change any of these. Each is marked where it applies.

- **P-1** — A user-owned record that does not exist, belongs to someone else, or is addressed through the wrong parent returns **404**. All three cases give an identical response (BR-018). This changes today's 403 for foreign loadouts and items, and the existing tests that expect 403.
- **P-2** — A conflicting owner, target or parent ID, or admin status, in a request body is **rejected with 400** rather than silently ignored (BR-017).
- **P-3** — **No admin exceptions** for user-owned data or for other users' profiles: admins cannot read or change other users' inventory, containers or loadouts, or update or delete other accounts. Admins keep only the catalog (BR-001) and admin-status changes (BR-010). This removes today's admin ability to list another user's registrations. Affects BR-008, BR-009 and BR-012 to BR-015.
- **P-4** — Self-deletion **removes all of the user's data** (registrations, container links, loadouts, items), **all-or-nothing** (BR-009).
- **P-5** — The direct user-creation operation is **admin-only** (BR-011).
- **P-6** — Google sign-in is **refused with 403** in these cases: the email is not verified and no account is bound to the token's subject; the verified email belongs to an account already bound to another subject; or several unbound accounts share the email. In these cases no account is linked or created (BR-004, BR-005).
- **P-7** — The backfill email match is **exact and case-sensitive, after trimming whitespace** (BR-004).
- **P-8** — The **permanent admin** is the admin with the **lowest user ID when BR-019 takes effect**, or, if there is no admin then, the first account to become admin afterwards. The designation never moves (BR-019 AC-1). The product owner should confirm that this selects their own account.
- **P-9** — Revoking the permanent admin's status, or the permanent admin **deleting their own account**, is **refused with 403** for every caller and changes nothing. Granting admin status to the permanent admin succeeds and changes nothing (BR-019 AC-4, AC-5).
- **P-10** — Admins other than the permanent admin get **no protection**: any admin may revoke them, and they may revoke **their own** status (BR-019 AC-7).
- **P-11** — **Any registration can act as a container**, whatever the catalog `gear_is_container` flag says (BR-020 AC-1). "A container can store anything" could also mean "only container gear holds items, but it holds any gear". This reading was chosen because the owner also said that a gear item in a container can itself hold items.
- **P-12** — A registration has **at most one direct container**. Linking it into a second container, or creating a cycle longer than a self-link, is **refused with 409**. A self-link stays 400 (BR-020 AC-3, AC-4).

## Open questions

None are open. The product owner answered all five on 2026-11-21 (anchor decisions D6 to D10):

- **Q-1 (answered, D6)** — Can an admin remove admin status, including their own or the last admin's? The first admin is permanent and can never lose admin status (BR-019). Other admins can be demoted, also by themselves (P-10). Because the permanent admin always stays, at least one admin always exists.
- **Q-2 (answered, D7)** — Should the stored profile email follow a changed Google email? There is no requirement either way. Email is profile data only and never identifies a user, except in the one-time backfill match (BR-004 AC-10, BR-008 AC-6).
- **Q-3 (answered, D10; accepted, not provisional)** — Must GoGear service JWTs issued at sign-in or refresh stop working before they expire after a demotion or account deletion? (Google tokens were never the subject.) No. They keep their role until they expire, 60 minutes by default. No revocation and no per-request role check is required (BR-006 AC-5, BR-009 AC-7).
- **Q-4 (answered, D9)** — Container rules. Anything can be stored in a container, at unlimited depth (BR-020, with provisional defaults P-11 and P-12).
- **Q-5 (answered, D8)** — Which profile fields may a user edit? All of them (username, name, email) except the Google subject, admin status and user ID (BR-008 AC-4, AC-5). The edited-email rule that protects the backfill is BR-004 AC-8 and AC-9.

## Bundle files

- [Change log](/docs/requirements/log.md)
