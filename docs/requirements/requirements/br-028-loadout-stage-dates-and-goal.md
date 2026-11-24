---
type: Behavioral Requirement
title: BR-028 A loadout has a stage, optional trip dates and an optional weight goal
description: Each loadout is in stage planning, packing or done (any change allowed, set only by the owner), records when it was last marked done, and may have start and end dates and an on-your-back weight goal in grams.
tags: [loadouts, stage, trip-dates, weight-goal, loadout-planning]
timestamp: 2026-11-22T00:00:00Z
---

# BR-028 — A loadout has a stage, optional trip dates and an optional weight goal

**Implementation:** AC-1 to AC-6 are implemented in the code of milestone B on branch `loadout-loop-milestone-b` (2026-11-24; migration `000012`, tests in `pkg/api/loadout_stage_test.go`; contract in the Swagger file and [ARS-001](/docs/architecture/spec/ars-001-loadout-loop-schema-and-api.md) milestone B): `PUT /loadout/insert` takes the dates and the goal, `POST /loadout/:loadout/update` also takes `stage`, and loadout list, get and full report `stage`, `done_at`, `start_date`, `end_date` and `weight_goal`. The public loadout carries none of them. The part of AC-1 that concerns a copy waits for milestone C. **Not merged to `main`, not released, not deployed, and not accepted.** **Status:** Accepted. **Priority:** P1 (milestone B). **Basis:** the statement and AC-1, AC-3 and AC-4 are sourced from capability need N6 of the accepted design ("Loadout stage (planning, packing, done), optional trip dates, optional weight goal"; owner's yes, [LP1](/docs/anchors/loadout-planning-and-packing.md)) and its home flow F1 ("packed 12 Jul"), board footer ("Start packing" switches to Packing) and Pack flow ("Mark trip as packed"). AC-2 (any change allowed, no automatic change) and the value rules are provisional default P-26.

**Statement:**
The API shall keep, for each loadout, its stage, when it was last marked done, and its optional trip dates and weight goal, and change them only when the owner asks.

**Acceptance criteria:**

- AC-1: A loadout's stage is exactly one of planning, packing and done. A new loadout is in planning (a copy too, [BR-031](/docs/requirements/requirements/br-031-copy-a-loadout.md)). Any other value is refused with 400.
- AC-2: The owner can change the stage from any stage to any other at any time, whatever the loadout's places and progress. The API never changes the stage on its own, for example because of ticks or closed bags. **[Provisional default P-26 — open to product-owner change]**
- AC-3: While a loadout is done, reads report when it was last set to done. A loadout that is not done reports no such time.
- AC-4: A loadout may have a start date and an end date, each optional, each a calendar date with no time of day or time zone. The owner can set, change and clear each. An end date before the start date is refused with 400. A weight goal is optional: a whole number of grams, at least 1, meaning the target for "on your back" ([BR-029](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md) AC-3). The owner can set, change and clear it; any other value is refused with 400. **[Provisional default P-26]**
- AC-5: Being over the goal is never refused and never changes anything ([BR-024](/docs/requirements/requirements/br-024-every-loadout-item-has-one-place.md) AC-7).
- AC-6: Only the owner can read or change these values through authenticated operations ([BR-014](/docs/requirements/requirements/br-014-loadouts-owner-scoped.md)). The values in a request body that names a different loadout never redirect the change ([BR-017](/docs/requirements/requirements/br-017-client-identity-fields-never-take-effect.md)).

**Verification semantics:**
Create a loadout: stage planning, no dates, no goal, no done time. Change planning → done → planning → packing → done: each succeeds, and the done time is present only while done. Tick everything and close every place: the stage is unchanged. Set an end date before the start date, a goal of 0 and stage "packed": 400, nothing changes. Set and clear dates and goal: reads reflect each step. As another user, change the stage: 404. Any stage change that the owner did not request falsifies AC-2.

## Derived from

- [Loadout planning and packing decisions — LP1](/docs/anchors/loadout-planning-and-packing.md)
- [Access control and identity decisions — D4](/docs/anchors/access-control-and-identity.md)
- Accepted frontend design "Loadout planning and packing", capability need N6, flows F1, F4, F5 (frontend repository, `docs/design/design-spec/loadout-planning-and-packing.md`)

## Related

- [BR-029 Loadouts report every weight input and a current on-your-back total](/docs/requirements/requirements/br-029-loadout-weights-and-summary-reported.md)
