# GoGear API requirements — access control and identity

## In brief

**Goal:** Every user can see and change only what the product owner allows. Users manage only their own inventory, containers, loadouts and account. Only admins change the shared catalog and admin roles. A Google login always reaches the same account, and never someone else's.

**Example of success:** Alice and Bob are ordinary users. Alice can see Bob's name in the user directory but not his email. If Alice sends a request to edit Bob's loadout, delete his gear registration, or move one of his loadout items, she gets "not found" and nothing of Bob's changes. Alice can't make herself admin. If the owner demotes an admin, that admin's next token refresh produces an ordinary token. Anyone can open Bob's loadout through its public link while it is public. After he makes it private, the link returns "not found".

**Most important limit:** Apart from BR-022 (release and deployment, below), these requirements cover only the access-control and identity decisions of 2026-11-21 ([anchor](/docs/anchors/access-control-and-identity.md)). Implementation is under way: BR-006 to BR-011 (users, roles, token refresh) are implemented as of wave 1, BR-014 to BR-016 (loadouts, loadout items, public loadouts) as of wave 2, BR-012 and BR-013 (inventory registrations, container links) as of wave 3, BR-001 (admin-only catalog) as of wave 4, and BR-003 to BR-005 (Google identity binding) as of wave 5; BR-002 is planned, not implemented. The follow-up decisions D6 to D10 of the same day add BR-019 (permanent first admin) and BR-020 (container nesting), and amend BR-004, BR-006, BR-008 to BR-010, BR-012, BR-013 and BR-018. Wave 7 implements BR-019, BR-020 and the amended parts of BR-004 (AC-9), BR-008 to BR-010 and BR-018. Decision D11 of 2026-11-22 adds BR-021 (the catalog category decides whether gear can be a container) and amends BR-020 AC-1 and AC-5 and BR-001 (AC-6); these changes are implemented in the container-capability change after wave 7. Wave 6 adds the database integrity that backs these rules (foreign keys, container-link and quantity constraints) and a required Go CI gate. The engineering review of revision `6fb972b` found most of them violated. Performance and API redesign findings from the same review are out of scope.

**Release and deployment (BR-022, added 2026-11-22):** a separate topic in the same bundle, from [anchor decisions RD1 and RD2](/docs/anchors/release-and-deployment.md). Goal: a merge to `main` with a release-worthy change creates a new version, builds and pushes its image and deploys it to the owner's Kubernetes server in one workflow run, with nothing for anyone to do after the merge. Example: a `fix:` merged to `main` (if the release rules count it) becomes a version above `v4.9.3`, the new pod runs and its database is at the latest migration; a merge with nothing release-worthy creates no tag, image or deploy, and the run is still green. Limit: one-time setup that already exists (secrets, server, cluster) is not a manual step, but a re-run, approval or any per-release action is. BR-022 is implemented, pending verification: the workflow and the app's migration-version line are in place, and the deploy path is proven only by the first push-triggered run on `main` after merge. Because of it, BR-021 AC-5 seeds the container settings from the old gear flags (P-14 superseded by P-18).

**Choices the product owner may still change:** the provisional defaults P-1 to P-10, P-12, P-13 and P-15 to P-18 below (P-11 and P-14 are superseded), the D11 container rules adopted on General's proposal, and the edited-email rule of BR-004 AC-8 and AC-9, which was adopted on General's recommendation without an explicit owner choice.

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
- **Top category, sub-category:** the two levels of catalog classification (`gear_top_category`, `gear_category`). Every sub-category belongs to one top category; every gear item names one of each.
- **Container-capable gear:** gear whose sub-category is a container category, directly or inherited from its top category (BR-021). Only registrations of such gear can receive new contents (BR-020).
- **Email:** profile data only. It identifies an account only in the one-time backfill match of BR-004 (anchor decision D7).

## Behavioral requirements

| ID | Requirement | Decision | Review finding |
|---|---|---|---|
| BR-001 | [Catalog changes are admin-only](/docs/requirements/requirements/br-001-catalog-changes-admin-only.md) — AC-6 (container settings, D11) implemented | D1, D11 | SEC-10 |
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
| BR-019 | [The first admin is a permanent admin](/docs/requirements/requirements/br-019-permanent-first-admin.md) — **implemented (wave 7)** | D6 | — |
| BR-020 | [Containers nest to any depth, with one direct container per registration and no cycles](/docs/requirements/requirements/br-020-container-nesting-consistency.md) — **implemented (wave 7); AC-1 and AC-5 as amended by D11 implemented (BR-021 change)** | D9, D11 | SEC-06 |
| BR-021 | [The catalog category decides whether gear can be a container](/docs/requirements/requirements/br-021-container-capability-from-category.md) — **implemented (BR-021 change); AC-5 as amended on 2026-11-22 (seeding, P-18) implemented in migration 000010** | D11, D1, RD2 | — |
| BR-022 | [A merge to main releases and deploys a new version with no manual step](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md) — **implemented, pending verification (deploy path proven only by the first post-merge run)** | RD1 | — |

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
  - Still open at the end of wave 6: repair of existing orphans, `totalWeight` after catalog weight edits, and a Swagger drift check in CI.
- **Wave 7 (implemented):** BR-019, BR-020, BR-004 AC-9, and the amended parts of BR-008 to BR-010 and BR-018 (answers to Q-1 to Q-5; provisional defaults P-8 to P-12 accepted for implementation). Decisions made while implementing, within the requirements:
  - **Permanent admin (BR-019):** migration 000007 adds `permanent_admin`, a single-row table with no foreign key. It is filled with the lowest-ID admin when the migration runs, and by triggers with the first account that becomes admin if there was none. The designation never moves, and an account removed outside the API is not replaced. `POST /users/{user}/admin` and `DELETE /users/{user}/delete` check it in the same transaction as the change: revoking is 403 for every caller, granting is a no-op 200, deleting is 403. Other admins have no protection (P-10).
  - **Clearing the edited-email mark (BR-004 AC-9):** `POST /api/v1/users/{user}/email-edited/clear`, admin only. It logs the acting admin and the target.
  - **Profile update (BR-008):** the existing statement already met AC-4 to AC-6; wave 7 only adds tests.
  - **Containers (BR-020):** any registration may hold any other (the catalog flag gates nothing; it stays the `container` list filter). *Superseded on 2026-11-22 by D11: only registrations of container-capable gear may receive contents (BR-020 AC-1, BR-021); implemented in the container-capability change below.* A second container or a cycle at any depth is 409, a self-link stays 400 and is checked first, and ownership (404) comes before both. The checks run in the insert transaction, and the database repeats them (unique index on the content column, cycle trigger), so concurrent requests cannot break them.
  - **Migration 000008 data cleanup (BR-020 AC-8):** removes self-links, links with a missing container or content registration, links across owners, every container of a registration except the oldest link, and one link per ring (the link that puts the ring's lowest-numbered member into its container). Registrations are never deleted. The down migration restores the 000005 indexes but not the removed links.
  - **Existing tests changed:** tests that seeded two-way links or a registration in two containers now seed states the database refuses (`integrity_constraints_test.go`, `usergear_authz_test.go`); `fkstartup_test.go` no longer expects the orphan container link that 000008 removes.
- **Container capability from the catalog (implemented, after wave 7):** BR-021, the D11 amendments of BR-020 (AC-1, AC-5) and BR-001 (AC-6); provisional defaults P-13 to P-17 implemented as written. Realization is fixed in [ADR-001](/docs/architecture/decisions/adr-001-category-driven-container-capability.md). Decisions made while implementing, within the requirements:
  - **API (ADR-001 D-2):** top categories carry `top_category_is_container` (bool; absent or `null` is false on create and unchanged on update). Sub-categories carry `category_container` (`container`, `not_container` or `inherit`; `inherit` removes the explicit setting; absent or `null` is `inherit` on create and unchanged on update; any other value is 400) and the read-only effective `category_is_container`. Category list items also carry `top_category_is_container`. Writes are admin-only (403, 401 without a token), reads are open to every authenticated user.
  - **Status (AC-3):** `gear_is_container` stays in every gear and registration response as a read-only value derived from the sub-category; a request that sends it is not refused, the field is ignored, and an update whose only field is `gear_is_container` is 400 (no updatable fields).
  - **Links (BR-020 AC-1, P-17):** 409 with a message about the caller's own registration, after the 404 and the self-link 400, before the other 409s; database triggers repeat the rule on link insert and update only, so existing links stay (P-15).
  - **Migration 000009:** all settings start as not a container (P-14); the old per-gear flag is kept as `gear.gearIsContainerLegacy`; no link changes; it cannot be applied twice and its down migration discards the settings. *Admins had to mark container categories by hand after 000009 (P-14); P-14 was superseded on 2026-11-22 by the amended BR-021 AC-5 (seeding, P-18), implemented in migration 000010 (below).*
  - **Tests:** `pkg/api/container_capability_test.go`, `pkg/api/category_container_api_test.go` (E2E-001), `migration9_test.go`.
- **Seeding and release (implemented 2026-11-22, pending verification of the deploy path):** BR-021 AC-5 as amended (P-18) is migration `000010_seed_category_container` (ADR-002 D-7), tested in `migration10_test.go` and `pkg/api/migration10_api_test.go`, so a deploy needs no admin step. BR-022 is realised by `release.yaml` (nrkno v5.1.6 at `a68fb147`, `released == 'true'` guards, concurrency group, "Verify deployed pod" step) and the app's `gogear-api migrations: version=<N> dirty=<bool>` stdout line. BR-022 stays unproven until the first push-triggered run on `main` is green on its first attempt (`verify` node).
- **Not yet implemented:** BR-002 (planned); repair of pre-existing orphan rows other than container links (registrations, loadouts and loadout items that point at missing parents are still only reported at startup).

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
- **P-11 (superseded 2026-11-22 by D11)** — ~~Any registration can act as a container, whatever the catalog `gear_is_container` flag says.~~ The owner clarified that "a container can store anything" meant what a container may hold, not what may be a container. The reading was a misunderstanding. Now only registrations of container-capable gear can receive contents (BR-020 AC-1, BR-021). Kept here so the history stays traceable; do not implement.
- **P-12** — A registration has **at most one direct container**. Linking it into a second container, or creating a cycle longer than a self-link, is **refused with 409**. A self-link stays 400 (BR-020 AC-3, AC-4).
- **P-13** — Gear whose sub-category does not exist (only possible for rows stored before foreign keys were enforced) is **not container-capable** (BR-021 AC-4).
- **P-14 (superseded 2026-11-22 by RD2 and P-18)** — ~~**Starting values:** new and existing top categories are not container categories, and sub-categories have no explicit setting, unless an admin sets them. When BR-021 takes effect, no gear is container-capable until an admin marks categories; old per-gear flag values are not carried over (BR-021 AC-5). An alternative the owner may prefer: seed the settings from the old gear flags.~~ It made admins mark categories by hand after deploy, a manual step that the owner's rule "no manual steps when deploying a new version" (anchor RD1, RD2) rules out. Kept here so the history stays traceable; do not implement.
- **P-15** — When a catalog change makes gear no longer container-capable, the change **is allowed, and existing container links stay**. Such a registration keeps its contents but receives no new ones. The same holds for links that exist when BR-021 takes effect (BR-021 AC-6). Chosen over refusing the change (any user's link would block catalog work, and admins cannot remove other users' links, P-3) and over removing links (it would change user data without the user's request).
- **P-16** — The list filter `container=true|false` on the gear list and the registration list **keeps working**, now from the derived container status (BR-021 AC-7).
- **P-17** — Linking into a registration whose gear is not container-capable is **refused with 409**, after the ownership 404 and the self-link 400, and before the 409s for a second container or a cycle (BR-020 AC-1, AC-5).
- **P-18** — **Seeding from the old gear flags** (replaces P-14): when BR-021 takes effect on a database, a sub-category with **at least one** flagged gear item becomes explicitly a container category; every other sub-category keeps no explicit setting; top categories stay not container categories unless an admin sets them. Seeding never overwrites an admin's explicit setting and runs at most once per database (BR-021 AC-5). That seeding happens at all is derived from RD2; this exact rule was adopted on General's proposal (the ADR-001 OQ-1 candidate) and is open to product-owner change.

## Open questions

No product question is open. One technical choice is open:

- **T-1 (open, architect; not blocking the requirements)** — Which commit types are release-worthy for BR-022, and which version part each bumps. The architect decides and keeps the earlier release behavior of this repository where it is known. If none can be established and the choice changes what reaches the server (for example whether dependency-update chores deploy), the architect raises it to the owner through General.

The product owner answered Q-1 to Q-5 on 2026-11-21 (anchor decisions D6 to D10), and refined Q-4 on 2026-11-22 (D11):

- **Q-1 (answered, D6)** — Can an admin remove admin status, including their own or the last admin's? The first admin is permanent and can never lose admin status (BR-019). Other admins can be demoted, also by themselves (P-10). Because the permanent admin always stays, at least one admin always exists.
- **Q-2 (answered, D7)** — Should the stored profile email follow a changed Google email? There is no requirement either way. Email is profile data only and never identifies a user, except in the one-time backfill match (BR-004 AC-10, BR-008 AC-6).
- **Q-3 (answered, D10; accepted, not provisional)** — Must GoGear service JWTs issued at sign-in or refresh stop working before they expire after a demotion or account deletion? (Google tokens were never the subject.) No. They keep their role until they expire, 60 minutes by default. No revocation and no per-request role check is required (BR-006 AC-5, BR-009 AC-7).
- **Q-4 (answered, D9; refined by D11)** — Container rules. Anything can be stored in a container, at unlimited depth (BR-020, P-12). Only registrations of gear in a container category can hold items; admins set this on categories (D11, BR-021, P-13 to P-17). P-11 is superseded.
- **Q-5 (answered, D8)** — Which profile fields may a user edit? All of them (username, name, email) except the Google subject, admin status and user ID (BR-008 AC-4, AC-5). The edited-email rule that protects the backfill is BR-004 AC-8 and AC-9.

## Bundle files

- [Change log](/docs/requirements/log.md)
