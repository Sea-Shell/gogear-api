---
type: Architecture Decision
title: ADR-002 Zero-touch release, deploy and container seeding
description: How a merge to main releases, builds, deploys and proves the deploy with no manual step (BR-022) — the nrkno v5.1.6 reusable workflow at a pinned SHA, the released/tag output contract and guards, default bump rules with no .semrelrc.yml, least-privilege permissions, a non-cancelling concurrency group, digest-checked deploy with in-run pod and migration-version evidence — and how BR-021 AC-5 seeding ships as new migration 000010.
tags: [architecture, release, deployment, ci-cd, github-actions, kubernetes, migration, sqlite]
timestamp: 2026-11-22T00:00:00Z
---

# ADR-002 — Zero-touch release, deploy and container seeding

**Status:** Accepted (architect, 2026-11-22). Implemented, pending verification: migration `000010` and the D-8 version line are in the code, and `release.yaml` follows D-1, D-2, D-6 and D-11. D-11 is implemented, pending verification: `.github/scripts/verify-deployed-pod.sh` now holds the whole post-deploy check, including D-8's wait, request timeouts and filtered diagnostics, its stub test runs in the `go` workflow, and k8s-deploy has `pull-images: false`. The deploy path is proven only by the first push-triggered run on `main` (`verify` node). **Amended 2026-11-22:** factual corrections to D-3 and D-9 and to the AC-3 and AC-5 rows from the rehearsal; the decisions are unchanged. **Amended 2026-11-22 (decision change):** D-11 replaces the D-8 `imageID` digest gate with a Deployment-template, pod-spec and owner check, reports every check before failing, and sets k8s-deploy `pull-images: false`, after run 45048563144 deployed a content-identical `v4.9.5` correctly but failed the gate. D-11 is implemented in `.github/scripts/verify-deployed-pod.sh` and `release.yaml`; it is proven only by the first push-triggered run on `main` (`verify` node). Closes ADR-001 OQ-1 and decides where ADR-001 D-5's seeding statement lives.

## In brief

**What the system will do:** On every push to `main`, `release.yaml` calls the nrkno semantic-release reusable workflow v5.1.6, pinned to its commit SHA. If the commits since the latest tag call for a version, it tags and publishes a GitHub release and reports `released == 'true'` and the new `tag`. Only then does the build job build that tag, push the image and pass on its digest, and the deploy job deploy exactly that tag and digest over Tailscale. The deploy job then waits, checks that the Deployment and its one API pod name that tag and digest and that the pod has zero restarts, and checks the migration version the pod logged against the highest migration in the tag. A merge with nothing release-worthy gives `released == 'false'`; build and deploy are skipped and the run is green. Release runs never overlap and are never cancelled while running. The seeding of category container settings from the old gear flags ships as new migration `000010`.

**Why this design:** It is the smallest change from the path that last deployed (run 28101251749): the same reusable-workflow shape, the same build and deploy steps and secrets, a new pin and new output names. The upstream defaults for bump rules reproduce this repository's release history, so no configuration file is added. The only new parts are guards and one evidence step, and each one closes a gap that BR-022 names (empty tag, wrong digest, a pod that crashes after the rollout, overlapping runs).

**Normal example:** A PR titled `fix(release): …` is squash-merged. The release job computes `v4.9.4` (the probe gives this from current `main`; a `feat` title would give `v4.10.0`), tags it, and outputs `released=true`, `tag=v4.9.4`. Build checks the tag format, builds `refs/tags/v4.9.4`, and pushes `docker.io/bateau/gogear-api:v4.9.4` with digest `sha256:abc…`. Deploy applies `…:v4.9.4@sha256:abc…`. k8s-deploy waits for the rollout. After 180 s the evidence step finds the Deployment rolled out with template image `…:v4.9.4@sha256:abc…` and one pod owned by its ReplicaSet with the same spec image, `Running`, ready, `restartCount 0`, an image ID from `bateau/gogear-api` (its digest is reported; it may name identical content by an earlier digest, D-11), and a log line `gogear-api migrations: version=10 dirty=false` that matches `000010` in the tag. The pod migrated the database itself; `000010` set the container settings from the old flags.

**Most important limits:** A failed run is not retried automatically (BR-022 AC-4). A tag can then exist without a deploy, and the next release-worthy merge releases and deploys the next version. `workflow_dispatch` stays, but a dispatched run never counts for BR-022 and cannot redeploy an existing tag. The deploy job's permissions are kept as they are, not reduced, because deploy cannot be rehearsed before the merge. Seeding is once per database on the release path; an operator who runs `000010` down and up again by hand is outside that path.

## Derived from

- [BR-022 A merge to main releases and deploys a new version with no manual step](/docs/requirements/requirements/br-022-merge-to-main-releases-and-deploys.md)
- [BR-021 The catalog category decides whether gear can be a container](/docs/requirements/requirements/br-021-container-capability-from-category.md), AC-5 as amended on 2026-11-22 (P-18)
- [Release and deployment decisions — RD1, RD2](/docs/anchors/release-and-deployment.md)
- [ADR-001 Realizing category-driven container capability](/docs/architecture/decisions/adr-001-category-driven-container-capability.md), D-5 and OQ-1
- Requirements index: provisional default P-18 and technical choice T-1 (`docs/requirements/index.md`)

## Satisfies

| Requirement | Satisfied by | Evidence that would prove it |
|---|---|---|
| BR-022 AC-1 (tag, image from the tag, deploy by digest, all or failed) | D-1, D-2, D-5 | post-merge run (`verify`): tag and GitHub release exist; image `:<tag>` digest equals the deployed digest |
| BR-022 AC-2 (push-triggered, first attempt, no human step) | D-1 trigger `push: main`; D-8 evidence inside the run; no environment approvals | run event is `push`, `run_attempt == 1`, no dispatched run for the merge |
| BR-022 AC-3 (nothing release-worthy: nothing published, green) | D-2 guard `released == 'true'`; D-3 default rules | dry run of a no-release history gives `released=false`; guard check shows build and deploy skipped; the release case is shown by a non-dry run against a bare repository and a stub API (`released=true`, a dry run cannot) |
| BR-022 AC-4 (failure visible, own image only, no partial success) | D-2 tag check, D-5 digest checks, D-6 concurrency, D-8 and D-11 post-deploy checks (template, pod spec and owner name this run's `<tag>@<digest>`) | a missing tag, empty or malformed digest, a pod of another revision or image, or a crashing pod fails the run; D-11 stub tests T-1 to T-10 |
| BR-022 AC-5 (version higher than `v4.9.3`, no moved tag) | D-1 (semrel resolves the lightweight `v4.9.x` tags, probe); D-6 serialized runs | dry run computes `v4.9.4` or `v4.10.0` depending on the squash title type |
| BR-022 AC-6 (full-SHA pinning stays, nested included) | D-1 pins and the nested pins listed there | recursive pin scan in `rehearse` and `review` |
| BR-022 AC-7 (a)–(e) (evidence) | D-8 and D-11 in-run checks, every check reported; (a)–(c) from run metadata and registry | the `verify` report |
| BR-022 AC-8 (close merges) | D-6 | two merges in quick succession: the later run deploys the higher version; no run is cancelled while running |
| BR-021 AC-5 / P-18 (seed from old flags, never overwrite, once) | D-7 migration `000010` | migration test on a v8 and a v9 database with mixed flags and admin settings |
| T-1 (release-worthy commit types) | D-3 | rehearsal with the past history: same releases as before |

Satisfied elsewhere: secrets, registry, cluster and Tailscale setup already exist (BR-022 Terms: not a manual step); owner confirms the secrets before merge (plan node `merge`). Nothing applicable is deferred.

## Decision question

How should `release.yaml` and the seeding of BR-021 AC-5 be realized so that a merge to `main` releases, deploys and proves the deploy with no manual step, under strict full-SHA pinning, and so that the first deploy leaves container-capable gear without an admin?

## Drivers and constraints

- `release.yaml` on `main` (`a8526c3`) calls `nrkno/github-workflow-semantic-release` at `b97399a` (v4.19.0), whose release step is disabled upstream, so `git-tag` is empty and nothing has deployed since 2026-06-24. Build and deploy read `needs.release.outputs.git-tag` at lines 41, 58, 72 and 99.
- v5 exposes `released`, `version`, `tag`, `major`, `minor`, `patch`; there is no `git-tag`. With nothing to release, `released=false` and `tag` is the **latest existing tag**, not empty (probe). A guard on tag emptiness would therefore redeploy the old tag.
- The repository has `sha_pinning_required: true` (research). Every `uses:` in the release path, including nested ones, must be a 40-hex SHA.
- The semrel binary accepts the lightweight `v4.9.x` tags and computes `v4.9.4` from current history; default bump rules (breaking → major, `feat` → minor, `fix` → patch, others none) reproduce every past release decision; a caller `.semrelrc.yml` is decoded on top of the defaults; the tool panics if the GitHub API is unreachable (probe).
- The PRs are squash-merged (`#NNN` suffixes on `main`). The bump comes from **all unreleased commits since the last tag**, and the highest type among them wins; a squash merge adds one commit (its title) to that set, a rebase or merge adds every commit of the branch. A title therefore decides the release only relative to what is already unreleased. On current `main`, `v4.9.3..a8526c3` already holds 3 `fix` commits, so any squash title gives at least `v4.9.4` (a `feat` title gives `v4.10.0`). A `chore` or `ci` title alone releases nothing only when no other unreleased `fix`, `feat` or breaking commit exists. The rehearsal gives `v4.10.0` for this branch because of its `feat(db)` commit, whatever the PR title. *(Wording corrected after the rehearsal and its review; the decision is unchanged.)*
- The v5 default rules release only breaking changes, `feat` and `fix`. Unlike the old preset, they do not release `perf` or `revert` commits. No such commits exist in the history, so past behaviour is unchanged; a future `perf:` or `revert:` merge would not release by itself.
- `deployment.yaml` uses `strategy: Recreate`, one replica, SQLite on a hostPath, no readiness or liveness probe. The app runs `m.Up()` at start and calls `log.Fatalf` on a migration error (`main.go` `runMigrations`), so a failed migration is a container exit and a restart, possibly after the rollout was reported complete.
- The release path uses no Node.js: nrkno v5 is a Go binary in a container, and build and deploy run no Node steps. The plan's "Node 24.x where Node is used" is therefore not applicable.
- Out of scope: release notes, floating tags, rollback (BR-022 Out of scope).

## Options considered and decisions

### D-1 Release mechanism

**Decision:** (a) the reusable workflow `nrkno/github-workflow-semantic-release/.github/workflows/workflow.yaml@a68fb147ef99f6622453fc7d2976c295e87ffb1f # v5.1.6`, with `release-enabled: true`, `lint-enabled: false`, `runs-on: ubuntu-latest`.

Pins inside it at that SHA (read from the file on 2026-11-22):

- `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7`
- `nrkno/github-action-semantic-release@5afff0e9b820879b997a3e381d09d1dc3941dc00 # v0.14.5`, a composite action that pins `sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2`, verifies the image signature with cosign, and runs `ghcr.io/nrkno/github-action-semantic-release:<action ref>`.

Why v5.1.6 and not v5.1.5: the only change (`a68fb147`, released 2026-11-20) bumps the inner action from v0.14.4 to v0.14.5, "fix(git): prune commit range enumeration". Both are SHA-clean; v5.1.6 is the current release and the one Renovate would move to next, so pinning v5.1.5 would add a bump PR for no gain. The rehearsal must run v0.14.5, the version actually pinned (D-9).

| Option | For | Against |
|---|---|---|
| **(a) reusable workflow v5.1.6 at SHA** (chosen) | Same shape as the path that last worked; the main/master gate, `fetch-depth: 0` and `fetch-tags: true` checkout come built in; Renovate already tracks this dependency | One more nested layer to scan for pins; job permissions are fixed by upstream |
| (b) in-repo job calling the nrkno action directly | Exposes the `dry-run` input in the same job; one layer fewer | Re-implements the checkout and the branch gate that upstream maintains; no requirement gains from it; the action's `dry-run` is equally usable from the rehearsal (D-9) |

(b) loses because it adds code this repository must keep correct and buys nothing BR-022 needs. The image is referenced by the action ref, not a digest; it is cosign-verified against the nrkno workflow identity. AC-6 covers actions and workflows, which are all SHA-pinned.

### D-2 Output contract and guards

**Decision:**

- **Consumed outputs:** `needs.release.outputs.released` and `needs.release.outputs.tag`. Every `git-tag` reference (lines 41, 58, 72, 99) becomes `tag`. No other release output is used.
- **Build job:** `if: needs.release.outputs.released == 'true'`. Its first step fails the job unless the tag matches `^v[0-9]+\.[0-9]+\.[0-9]+$`. The tag is passed to that step through `env:`, never interpolated into `run:`. Checkout uses `ref: refs/tags/<tag>`. After the push, a step fails the job unless `steps.build_push.outputs.digest` matches `^sha256:[0-9a-f]{64}$`.
- **Deploy job:** `needs: [release, build]` and `if: needs.release.outputs.released == 'true'`. Because the expression contains no status function, GitHub adds `success()`, so deploy runs only when release and build both succeeded. It uses `needs.build.outputs.digest` and the release tag only.
- **Why `== 'true'`:** with nothing to release, `tag` holds the latest existing tag. A guard on `tag != ''` would rebuild and redeploy it. When the release job is skipped (any ref other than `main`/`master`), `released` is empty, and the guard is false.
- **`workflow_dispatch`:** kept. A dispatched run on `main` behaves exactly like a push run: it releases only if unreleased release-worthy commits exist; otherwise `released=false` and nothing is published. It can therefore retry a run that failed *before* tagging, but it can never rebuild or redeploy an existing tag. On any other branch the upstream gate skips the release job and everything else is skipped. A dispatched run never counts as evidence for BR-022 (AC-2).

Rejected: guarding on `tag != ''` (redeploys the old tag, AC-3). Rejected: removing `workflow_dispatch` (it gives no BR-022 path either way, and keeping it costs nothing and keeps the owner's existing control).

### D-3 Bump rules: no `.semrelrc.yml`

**Decision:** add no `.semrelrc.yml`. The upstream defaults apply: breaking change → major, `feat` → minor, `fix` → patch, every other type → no release.

- T-1 asks to keep the earlier behaviour. The probe shows these defaults reproduce the past release history exactly: ranges of only `chore`/`ci` commits never released; `fix(deps)` did. Renovate's `chore(deps)` (Docker image digests, lock files) and `ci(deps)` do not release; `fix(deps)` (Go modules) does. This is what the owner had, so the choice does not change what reaches the server and needs no escalation.
- Do not copy the workflow repository's own `.semrelrc` (`chore: patch`): it would make every Renovate chore release and deploy.
- Consequence: the bump of a merge is decided by the squash-merge PR title. The `pr` node's title type sets the next version (`fix` → `v4.9.4`, `feat` → `v4.10.0`; current `main` already holds `fix` commits, so at least `v4.9.4`).

Rejected: an explicit `.semrelrc.yml` restating the defaults. It would guard against a future upstream change of defaults, but its schema is not verified here, a wrong key would silently change behaviour, and the pin already freezes the defaults until a deliberate bump. Revisit trigger in "Confirmation and reconsideration".

### D-4 Permissions

**Decision:**

| Scope | Permissions | Reason |
|---|---|---|
| workflow | `contents: read` (unchanged) | default for all jobs |
| `release` | `contents: write`, `pull-requests: read` | exactly what the upstream release job declares; a caller must grant at least that, and more is not used. Removes `issues: write`, `pull-requests: write`, `id-token: write`, `actions: write` |
| `build` | `contents: read` (explicit) | checkout only; Docker Hub uses its own secret |
| `deploy` | `contents: read`, `actions: read`, `id-token: write` (unchanged) | see below |

The deploy job's permissions stay as they are. `id-token: write` may be unneeded with Tailscale OAuth client credentials, and `actions: read` is likely used by k8s-deploy for its run annotations [both unverified]. Deploy cannot be rehearsed before the merge, and a permission error there costs a failed first release (AC-2), while dropping them gains nothing BR-022 requires. Reducing them is a follow-up after the first clean `verify`.

### D-5 Deploy only the run's own image

**Decision:**

- The `images:` input of k8s-deploy is `docker.io/bateau/gogear-api:<release tag>@<build digest>`, both from this run's `needs` (unchanged form).
- Build fails on an empty or malformed digest (D-2), so deploy can never receive `…:<tag>@`.
- The image is built from `refs/tags/<tag>`, the commit the release job tagged.
- D-6 keeps any other run from deploying in between. D-8 checks that the Deployment and its one live pod name exactly this `<tag>@<digest>` reference. *(Amended 2026-11-22: it no longer requires the pod's `imageID` to end with the digest; see D-8 and D-11.)*
- `:latest` is still pushed (unchanged); nothing deploys it.
- Provenance attestations stay on (build-push-action default). D-11 explains why turning them off would not fix the `imageID` problem.

### D-6 Concurrency for close merges

**Decision:** at workflow level:

```yaml
concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false
```

- A running release run is never cancelled, so no run stops between tag and deploy.
- GitHub keeps at most one pending run per group; a newer push replaces the pending one. The surviving run is for the newest commit, whose history includes every replaced merge, so their changes are released together (AC-8: "one release may include several merges").
- Runs execute one after another in push order. Each release job reads the tags the previous run created, so versions only grow (AC-5), and the last deploy is the highest version (AC-8).
- A replaced pending run shows as cancelled. It never started, published or deployed anything. The `verify` report must name it and the run that covered its merge.

Rejected: no concurrency group (two runs can compute from the same tag, and an older run's deploy can finish last). Rejected: `cancel-in-progress: true` (can leave a tag without a deploy, AC-4). Rejected: per-job groups (the deploy group alone does not order the releases).

### D-7 Seeding: new migration `000010`

**Decision:** a new migration `000010_seed_category_container` (up and down). `000009` stays byte-identical; its section 5 comment already says the seeding goes into a new migration after release.

- **Why not edit `000009`:** `000009` is on `main` (`a8526c3`) and is already applied to test databases. golang-migrate runs a version once, so a database at version 9 would never see an edit and would silently not be seeded. `000010` is applied on every database that reaches it through the release path: from v8 (runs `000009` then `000010`) and from v9. ADR-001 D-5 already set this rule.

**Up (`000010_seed_category_container.up.sql`), exactly one statement:**

```sql
UPDATE gear_category
SET categoryIsContainer = 1
WHERE categoryIsContainer IS NULL
  AND categoryId IN (SELECT gearCategoryId FROM gear
                     WHERE gearIsContainerLegacy = 1
                       AND gearCategoryId IS NOT NULL);
```

Rules this statement realizes (BR-021 AC-5, P-18):

- A sub-category with at least one flagged gear item and no explicit setting becomes explicitly a container (`1`).
- A sub-category with an explicit setting (`0` or `1`) made by an admin is never changed (`categoryIsContainer IS NULL`).
- Every other sub-category keeps `NULL` (inherits).
- Top categories are not touched; `topCategoryIsContainer` keeps its value (`0` unless an admin set it).
- Gear whose sub-category does not exist matches no `gear_category` row, so it contributes nothing (AC-4).
- On a fresh database there is no gear, so the statement changes nothing.
- **At most once per database:** golang-migrate records version 10 and never runs it again; the app only calls `m.Up()`. The legacy flags have no effect after that, because nothing else reads `gearIsContainerLegacy`.

**Down (`000010_seed_category_container.down.sql`):** no data change (a comment and `SELECT 1;`). A seeded `1` cannot be told apart from an admin's `1`, so undoing would risk removing admin settings. Down to 9 keeps the seeded settings; down of `000009` removes all settings as before.

**Limit:** if an operator runs `000010` down and up again by hand, the up statement runs a second time. It still changes no explicit setting, but a sub-category an admin reset to "inherit" in between would be seeded again. This is outside the release path (no workflow and no app code runs a down migration) and needs no marker table. Revisit if down migrations ever become part of a release.

**Test consequence:** `migration9_test.go` keeps asserting no carry-over **at version 9**. A new `migration10_test.go` covers v8 and v9 databases with all, some and no flags, a dangling sub-category, and an admin-set `0` and `1` on sub-categories with flagged gear.

This closes ADR-001 OQ-1: the rule is "any flagged gear → explicit container", top categories stay not container (P-18).

### D-8 In-run evidence of a healthy deploy (AC-7 (d), (e))

What k8s-deploy already gives: with `strategy: basic` it applies the manifests with the substituted image and waits for the rollout of the Deployment (`kubectl rollout status`), failing on timeout (research). With no readiness probe, "rollout complete" means only that the container started, not that the migrations passed.

**Decision:** two additions.

1. **App (owned by the `mig` node, `main.go` `runMigrations`):** after a successful `m.Up()` (including `ErrNoChange`), read `m.Version()` and write one line to stdout that does **not** depend on the configured log level:

   `gogear-api migrations: version=<N> dirty=<true|false>`

   The server's log level is not known here (`general.log_level` may be `error`), so a level-filtered log line could be missing and fail a good deploy. The existing info line stays.

2. **Deploy job, a step after k8s-deploy:** wait 180 s, then run the checks with `kubectl -n seashell`. *(Amended 2026-11-22; the full rule, its order and its test are in D-11. The `imageID` digest is now reported, not used as proof.)* In short:
   - exactly one pod with label `app=gogear-api` that is neither terminating nor in phase `Failed` or `Succeeded` (leftover evicted or completed pods are not part of the rollout);
   - the Deployment's current generation is rolled out, and its pod template, the pod's spec and the pod's owner all name this run's `<tag>@<digest>` reference (D-11 checks R-1 and I-1 to I-3);
   - its phase is `Running`, its container is ready, and `restartCount` is `0`;
   - its container `imageID` comes from `bateau/gogear-api`, and the step reports whether its digest equals the build digest (D-11 check I-4);
   - its log contains `gogear-api migrations: version=<N> dirty=false`, where `<N>` equals the highest numeric prefix of `migrations/*.up.sql` in the checked-out tag.

   Every `kubectl` call uses `--request-timeout=20s`, and the deploy job has `timeout-minutes: 25` (15 min rollout, 3 min wait, margin). Every check that can run is run and reported, and the run fails if any check failed (D-11 order). Before failing, the step prints bounded diagnostics: `get pods`, `describe` (tail), and the pod's logs (current, and previous when `restartCount` is above 0) **filtered** to error-level lines (`level` `error`, `dpanic`, `panic` or `fatal`, reduced to level, message and error), non-JSON crash lines, and the migrations line. Other log fields and info lines are never printed, because the repository is public and the app logs can carry usernames and subject IDs. The step prints the values it checked, so the run log is the AC-7 (d)/(e) evidence; the `verify` node reads it instead of asking the owner.

A migration error makes the container exit, so it shows up as `restartCount > 0` or a non-running pod within the wait. 180 s covers "a few minutes after the rollout" (BR-022 AC-7 (d)) and is a realization choice; the `verify` node may observe longer.

Rejected: a readiness probe and a `/health` field with the migration version. This changes the deployed manifest and the API, and still needs a check in the run. Rejected: `kubectl exec` with `sqlite3`. The image has no `sqlite3` binary. Rejected: relying on the rollout alone. It passes before a fatal migration.

### D-9 Rehearsal without publishing (node `rehearse`)

**Decision:** all local, nothing pushed to GitHub:

1. `actionlint` on `release.yaml`: no errors.
2. A recursive pin scan of `release.yaml`, the reusable workflow at `a68fb147…`, and the action at `5afff0e9…`, including its composite steps: every `uses:` is a 40-hex SHA.
3. A dry run (it proves only the computed `tag`: semrel sets `released = !dryRun`, so a dry run never reports `released=true`; `released=true` is shown by a local non-dry run against a bare repository and a stub GitHub API) of the **pinned** semrel action image (`ghcr.io/nrkno/github-action-semantic-release:5afff0e9b820879b997a3e381d09d1dc3941dc00`, cosign-verified as the action does) with `INPUT_SUBCOMMAND=release`, `INPUT_DRY_RUN=true`, `GITHUB_REF=refs/heads/main`, `GITHUB_EVENT_NAME=push`, a token that can only read, and a `GITHUB_OUTPUT` file in a temporary directory. Run it on two disposable local clones, whose push URL is set to an invalid path:
   - **release case:** `origin/main` plus one squash commit carrying the planned PR title. Expected: `tag=v4.9.4` for a `fix` title or `v4.10.0` for a `feat` title (`released=true` appears only in the non-dry-run variant above).
   - **no-release case:** the same history with a local lightweight tag at the simulated squash commit, plus one `chore:` commit. Expected: `released=false`, and `tag` equals that local tag (this also holds in a dry run).
4. Guard check: the `if:` expressions of build and deploy are exactly `needs.release.outputs.released == 'true'`. Under the no-release outputs they are false, and under empty outputs (release job skipped) they are false.
5. Proof of no publishing: the remote tags and releases, and the Docker Hub tags, are the same before and after.

Rejected: a run in a fork or a throwaway branch on GitHub. A dispatch on a branch skips the release job, so it proves only the empty-output case. A fork needs owner approval, and if a guard were wrong it would try to push or deploy. Use a fork only if the owner asks for it.

### D-10 Stale image pin in `deployment.yaml`

**Decision:** leave `docker.io/bateau/gogear-api:v2.0.0@sha256:ef27…` unchanged.

k8s-deploy replaces every image named `docker.io/bateau/gogear-api` in the manifests with the `images:` input before applying, so the value in Git is never deployed. It did exactly this in the last successful deploy. Changing the line touches the one step that cannot be rehearsed, and no requirement gains from it. Its cost is cosmetic: a reader may think `v2.0.0` runs. Rejected: refreshing it to the current version, which would go stale with every release and invite a Renovate PR each time.

### D-11 Proving the pod runs this run's image (amendment 2026-11-22)

**Why this was amended:** run [45048563144](https://github.com/Sea-Shell/gogear-api/actions/runs/45048563144) (`v4.9.5`) deployed correctly but failed D-8. The build was fully cached: its platform image manifest (`e9994c92…`) and config (`99ce2544…`) are byte-identical to `v4.9.4`'s. Only the provenance attestation changed, so the pushed index digest changed from `5978fe2a…` to `5b444c61…`. The new pod `gogear-api-9fcdfbfcd-fw2sz` had spec image `…:v4.9.5@sha256:5b444c61…`, was `Running` and ready with `restartCount 0`, and logged `version=10 dirty=false`. Its `imageID` was `…@sha256:5978fe2a…`, the **old** index digest. The node runtime (likely containerd under k3s) already held that content and reports a repo digest it recorded earlier. Whether it picks the first recorded digest or the lowest one is [unverified]. Either way, `imageID` names the content by *some* digest that once pointed to it on that node, not by the digest the pod asked for. Every content-identical release (for example a workflow-only `fix`) hits this.

**What proves the image:** the pod's spec reference. The kubelet pulls `…:<tag>@sha256:<d>` by digest, and the runtime checks the content against that digest, so a container that runs from a pod whose spec names `@<d>` runs the content `<d>` names. What remains is to prove that the pod checked is the one this run's deploy created. The Deployment template, its rollout state and the pod's owner prove that. None of it depends on how the runtime names images. It also needs no new RBAC: the deployer already gets Deployments (`kubectl rollout status` passed) and pods.

**Decision:** the "Verify deployed pod" logic moves into the script `.github/scripts/verify-deployed-pod.sh`. The step calls it with `env:` inputs, and the script is tested with a stub `kubectl` (below). Let `EXPECTED_IMAGE` be exactly the string passed to k8s-deploy `images:`, that is `docker.io/bateau/gogear-api:<tag>@<build digest>`. Select containers **by name** `gogear-api` in spec and status, never by index.

Pre-checks (fail fast; nothing else can be checked without them): `BUILD_DIGEST` matches `^sha256:[0-9a-f]{64}$`; `EXPECTED_IMAGE` ends with `@$BUILD_DIGEST`; the expected migration version `N` is computed from `migrations/*.up.sql` as before.

After the 180 s wait, read `deployment/gogear-api -o json` once and the `app=gogear-api` pods `-o json` once. Then evaluate and **report every** check below as `PASS`, `FAIL` or `NOT RUN (<reason>)`:

| ID | Check | Fails when |
|---|---|---|
| R-1 | Deployment rolled out at its current generation | `.status.observedGeneration != .metadata.generation`, or `.status.updatedReplicas`, `.status.readyReplicas` or `.status.replicas` is not `1` |
| I-1 | Deployment template names this run's image | template container `gogear-api` image `!= EXPECTED_IMAGE` |
| P-1 | Exactly one live pod | count of pods with no `deletionTimestamp` and phase not `Failed`/`Succeeded` `!= 1` |
| I-2 | Pod spec names this run's image | pod container `gogear-api` image `!= EXPECTED_IMAGE` |
| I-3 | Pod belongs to the Deployment's ReplicaSet for that template | the pod has no controller `ownerReference` of kind `ReplicaSet` named `gogear-api-<pod's pod-template-hash label>`, or the label is missing |
| H-1 | Pod healthy | phase `!= Running`, container not ready, or `restartCount != 0` |
| I-4 | Runtime image is from this repository | `imageID`, after removing any `<scheme>://` prefix, does not match `^(docker\.io/)?bateau/gogear-api@sha256:[0-9a-f]{64}$` (an empty `imageID` fails here) |
| M-1 | Migrations at the tag's highest version | log read fails (reported as now, with the `pods/log` hint), or no line `gogear-api migrations: version=N dirty=false` |

- **In I-4 only the repository part is a gate; the digest is reported, not used as proof.** The step prints `imageID=<value>` and either `imageID digest equals build digest` or `imageID digest <x> differs from build digest; the runtime names identical content by an earlier digest; identity is proven by I-1 to I-3`. A differing digest alone never fails the run.
- **Order:** P-1 decides whether pod checks can run. If P-1 fails, I-2, I-3, H-1, I-4 and M-1 are `NOT RUN (no single live pod)`. Otherwise **all** of them run, whatever earlier checks gave, so M-1 is reported even when an identity or health check failed. R-1 and I-1 always run. After the table, if any check is `FAIL` or `NOT RUN`, print the existing filtered diagnostics once and exit 1. Otherwise print `Verified` with the tag, the digest, the pod name and `N`.
- **Why I-1 to I-3 replace a ReplicaSet read:** tags are never reused (AC-5, D-2) and the template contains the tag, so two revisions can never carry the same `EXPECTED_IMAGE`. A pod from an older revision therefore fails I-2. I-3 proves the pod was created by the Deployment controller and not by hand. R-1 with I-1 proves the applied template is still the current one (no later apply replaced it). Reading ReplicaSets would add a permission the deployer is not known to have [unverified], and so a risk that the first run fails, for no extra guarantee.
- The diagnostics, the log filtering, the timeouts and the wait are unchanged from D-8.

**Stub test (required, runs in CI on pull requests):** `.github/scripts/verify-deployed-pod_test.sh`, using bash and `jq` only, run by a step in an existing pull-request workflow job that checks out the repository (the coder picks the job). It puts a stub `kubectl` first on `PATH`. The stub serves fixture JSON for `get deployment`, `get pods` and `describe`, and fixture text or an exit code for `logs`. The wait is set to 0 through an input (for example `VERIFY_WAIT_SECONDS`, default 180). The test asserts the exit code and the reported line of each named check. Cases:

| Case | Fixture | Expect |
|---|---|---|
| T-1 normal | everything consistent; `imageID` digest = build digest | exit 0; I-4 reports "equals" |
| T-2 content-identical release (run 45048563144) | spec and template `…:v4.9.5@sha256:5b44…`; `imageID` `docker.io/bateau/gogear-api@sha256:5978…` | exit 0; I-4 reports "differs … earlier digest" |
| T-3 wrong revision | the live pod is the previous ReplicaSet's (different `pod-template-hash`, spec `…:v4.9.4@sha256:5978…`), while the template is new | exit 1; I-2 `FAIL`; M-1 still reported |
| T-4 rollout not caught up | `observedGeneration < generation` | exit 1; R-1 `FAIL` |
| T-5 spec digest mismatch | pod spec and template `…:v4.9.5@sha256:<other>` | exit 1; I-1 and I-2 `FAIL` |
| T-6 hand-made pod | pod with no `ReplicaSet` controller owner, spec correct | exit 1; I-3 `FAIL` |
| T-7 foreign image | `imageID` `docker.io/other/x@sha256:…` | exit 1; I-4 `FAIL` |
| T-8 migration lag with identity failure | spec mismatch **and** log `version=9` | exit 1; I-2 `FAIL` and M-1 `FAIL`, both printed |
| T-9 no single pod | zero live pods (only a `Failed` one) | exit 1; P-1 `FAIL`; pod checks `NOT RUN` |
| T-10 unreadable log | `logs` exits 1 with `Forbidden` | exit 1; M-1 `FAIL` with the `pods/log` hint |

The test must also show that no fixture log field other than level, message and error reaches the output (a fixture line carries a `username`).

**k8s-deploy warnings seen in the same run:**

- `docker pull … pull access denied`: k8s-deploy pulls each image on the runner only to read a Dockerfile-path label for its annotations (`pull-images` input, default `true`, read from the action at `c7ebd0d5`). The repository is private and the deploy job has no registry login. **Decision:** set `pull-images: false`. This removes a useless pull and a misleading warning. Giving the deploy job Docker Hub credentials only for an annotation is rejected.
- `get namespace … forbidden`: when it annotates resources, k8s-deploy always reads the namespace annotation to find the last successful commit (`getLastSuccessfulRunSha`), even with `annotate-namespace: false`. **Decision:** leave it. The warning does not fail the step, and the resource annotations still record the run. Granting the deployer a cluster-scoped `get namespaces` would widen its permissions for a cosmetic gain. Turning off `annotate-resources` would lose the in-cluster record of which run deployed. Revisit only if k8s-deploy makes this an error.

**Rejected alternatives:**

| Option | Why it loses |
|---|---|
| (a) `imageID` digest ∈ {pushed index} ∪ {platform manifests in the pushed index}, from `imagetools inspect --raw` in the build job | Would still have failed run 45048563144: the reported `5978fe2a…` is the *old* index, which is in neither set. Making it pass needs the deploy job to resolve `imageID` in the registry (Docker Hub credentials in deploy) and still depends on how the runtime names content. More machinery and a new secret, with no guarantee beyond I-1 to I-3. |
| (b) `provenance: false` (and `sbom: false`) so identical content gets an identical digest | Does not fix it. The node already holds this content under `5978fe2a…`, and a new digest `e9994c92…` would be reported only if the runtime picks it, which is the unverified behaviour that failed. It also gives up provenance for every release. |
| Keep `imageID` as a gate and accept a re-run | Breaks AC-2 (first attempt, no human step) for every content-identical release. |
| Read the current ReplicaSet and compare its `pod-template-hash` | Same guarantee as I-1 to I-3 here (unique tags), but needs a permission the deployer is not known to have [unverified]. |
| Read node `status.images` | Needs cluster-scoped `get nodes`, and the list is truncated. Still runtime naming. |

## Rules the implementation must preserve

- No `git-tag` reference remains; build and deploy are guarded by `needs.release.outputs.released == 'true'`, and nothing else.
- Every `uses:` in the release path is a 40-hex SHA with a version comment.
- No `.semrelrc.yml` in the repository.
- Release permissions are exactly `contents: write`, `pull-requests: read`; build and deploy keep their secrets, steps and inputs, apart from the changes above.
- The concurrency group is workflow-wide and never cancels a running run.
- `kubernetes-manifests/deployment.yaml` is not changed (D-10).
- The pod check proves identity by `EXPECTED_IMAGE` equality in the Deployment template and the pod spec, plus R-1 and I-3 (D-11). A differing `imageID` digest never fails the run on its own, and a foreign `imageID` repository always does.
- Every D-11 check that can run is run and printed before the step fails; M-1 runs whenever exactly one live pod exists.
- Containers are selected by name `gogear-api`, never by index.
- k8s-deploy has `pull-images: false`; the deploy job gets no registry credentials and the deployer no new RBAC.
- Provenance attestations stay on.
- `000009` keeps its SQL. `000010` up is the one statement in D-7, and its down changes no data.

## Consequences

**Positive:** The release path works again with the smallest diff. The no-release, empty-tag, wrong-digest and crash-after-rollout cases all fail or skip visibly. The deploy proves itself in the run log. Seeding needs no admin action.

**Negative:** The pod check no longer has the runtime confirm the digest. It relies on pull-by-digest content checking and on tags never being reused (D-11). The release job now depends on the GitHub API being reachable (the tool panics otherwise): the run fails before any tag, and a dispatch or the next merge recovers. Every successful deploy run takes about 3 minutes longer (the wait). A replaced pending run appears as cancelled. Bump rules come from upstream defaults that only the pin freezes.

**Neutral:** The `mig` node's write scope includes one small change to `main.go` (D-8 line), in addition to the migration files.

**Reversibility:** Every workflow change is reverted by reverting `release.yaml`. `000010` cannot be undone by its down migration (by design). The settings can be changed through the admin API.

## Confirmation and reconsideration

Confirmed when:

- `rehearse` (D-9) passes all five checks, and the computed tag matches the PR title type;
- the migration tests of D-7 pass, and a fresh database migrates up to 10 and back;
- `verify`: the first push-triggered run on `main` is green on the first attempt, and its log shows the D-8 and D-11 values (R-1 and I-1 to I-3 `PASS`, the `imageID` report, `restartCount 0`, `version=10 dirty=false`);
- the D-11 stub tests T-1 to T-10 pass in CI.

Reconsider when:

- a bump of the nrkno pin changes the default bump rules or the output names (re-run the D-9 dry run on every bump; add an explicit `.semrelrc.yml` if the defaults change);
- the release job fails because the GitHub API is unreachable more than rarely;
- the first clean `verify` passes (then reduce the deploy permissions and re-verify);
- a readiness probe or a second replica is introduced (D-8 then has to select the pod differently);
- a tag is ever reused or moved, or the deploy no longer references `<tag>@<digest>` (D-11 I-2 then no longer separates revisions; add a ReplicaSet `pod-template-hash` check);
- k8s-deploy turns the namespace read into an error (D-11 warnings);
- down migrations become part of any release or recovery procedure (D-7 limit).

## Open questions

None blocking. Unverified premises, each covered by a check:

- Whether `id-token: write` and `actions: read` are needed by the deploy job. They are kept (D-4); revisit after `verify`.
- How containerd chooses the repo digest it reports as `imageID` (first recorded or lowest). D-11 does not depend on it.
- ~~That k8s-deploy writes the `images:` string into the template unchanged.~~ Closed 2026-11-22: the `describe` output of run 45048563144 shows `Image: docker.io/bateau/gogear-api:v4.9.5@sha256:5b444c611ce9…` on the pod, character for character the `images:` input of that run. I-1 and I-2 therefore compare exactly; no normalization is needed. The workflow now builds that reference once (step `image`) and passes it to both k8s-deploy and the check.
- That k8s-deploy v6 waits for the rollout of this Deployment. Research says so; D-8 does not depend on it, because it checks the pod after a fixed wait.
- That the repository secrets still exist and are valid. The owner confirms this before merge (plan node `merge`).
