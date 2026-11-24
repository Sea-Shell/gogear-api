package endpoints

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Milestone B2: the stage, trip dates and weight goal of a loadout (BR-028),
// the consumable role of a loadout item (BR-027) and the stage part of BR-025
// AC-8. Everything is made through the real routes and the real JWT
// middleware, on the `board` fixture of loadout_full_test.go; SQL only reads
// stored state back.
//
// BR-027 AC-4 (a copy keeps the role) belongs to milestone C, which adds copy.

// tripState is the stored stage, done time, dates and goal of every loadout and
// the consumable flag of every item, as SQL text. It is the "nothing changed"
// reference of the refusal tests.
func (b *board) tripState() string {
	b.env.T.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT quote(loadoutId)||'|'||quote(stage)||'|'||quote(doneAt)||'|'||quote(startDate)||'|'||quote(endDate)||'|'||quote(weightGoal)||'|'||quote(updatedAt) FROM loadouts ORDER BY loadoutId`,
		`SELECT quote(loadoutItemId)||'|'||quote(consumable)||'|'||quote(placeId)||'|'||quote(ticked)||'|'||quote(quantity) FROM loadout_items ORDER BY loadoutItemId`,
	} {
		rows, err := b.env.DB.Query(q)
		if err != nil {
			b.env.T.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				b.env.T.Fatal(err)
			}
			sb.WriteString(s + "\n")
		}
		rows.Close()
		sb.WriteString("--\n")
	}
	return sb.String()
}

func (b *board) loadoutOf(tok string, lo int64) models.Loadout {
	b.env.T.Helper()
	w := b.env.Do(http.MethodGet, loPath(lo, "get"), tok, nil)
	expectStatus(b.env.T, w, http.StatusOK)
	var l models.Loadout
	decodeJSON(b.env.T, w, &l)
	return l
}

// setStage updates the stage and requires 200.
func (b *board) setStage(tok string, lo int64, stage string) {
	b.env.T.Helper()
	b.call(http.MethodPost, loPath(lo, "update"), tok, map[string]any{"stage": stage}, http.StatusOK)
}

func str(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}

func wantTrip(t *testing.T, what string, l models.Loadout, stage, start, end string, goal int64) {
	t.Helper()
	g := "<null>"
	if l.WeightGoal != nil {
		g = fmt.Sprint(*l.WeightGoal)
	}
	wantGoal := "<null>"
	if goal != 0 {
		wantGoal = fmt.Sprint(goal)
	}
	if l.Stage != stage || str(l.StartDate) != orNull(start) || str(l.EndDate) != orNull(end) || g != wantGoal {
		t.Errorf("%s: stage %q start %s end %s goal %s; want %q %s %s %s",
			what, l.Stage, str(l.StartDate), str(l.EndDate), g, stage, orNull(start), orNull(end), wantGoal)
	}
}

func orNull(s string) string {
	if s == "" {
		return "<null>"
	}
	return s
}

// ---- BR-028 -------------------------------------------------------------------

// AC-1, AC-3: a new loadout is in planning with no dates, goal or done time,
// and every read of it says so, with the keys present and null.
func TestLoadoutStage_NewLoadoutReportsDefaultsEverywhere(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Trip")

	raw := b.call(http.MethodGet, loPath(lo, "get"), b.user, nil, http.StatusOK)
	if raw["stage"] != "planning" {
		t.Errorf("stage = %v, want planning", raw["stage"])
	}
	for _, k := range []string{"done_at", "start_date", "end_date", "weight_goal"} {
		v, present := raw[k]
		if !present || v != nil {
			t.Errorf("%s = %v (present %v), want present and null", k, v, present)
		}
	}
	for what, l := range map[string]models.Loadout{
		"get": b.loadoutOf(b.user, lo), "list": b.summary(b.user, lo), "full": b.full(b.user, lo).Loadout,
	} {
		wantTrip(t, what, l, "planning", "", "", 0)
		if l.DoneAt != nil {
			t.Errorf("%s: done_at = %s, want null", what, *l.DoneAt)
		}
	}
}

// Verification semantics: planning -> done -> planning -> packing -> done, each
// succeeds, and the done time is present only while done. Done again keeps the
// time; a later done after leaving done is a new time.
func TestLoadoutStage_AnyChangeAllowedAndDoneTimeFollowsDone(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Trip")

	b.setStage(b.user, lo, "done")
	first := b.loadoutOf(b.user, lo)
	if first.Stage != "done" || first.DoneAt == nil {
		t.Fatalf("after done: stage %q done_at %v", first.Stage, first.DoneAt)
	}
	doneTime, err := time.Parse("2006-01-02T15:04:05.000Z", *first.DoneAt)
	if err != nil {
		t.Fatalf("done_at %q is not in the updated_at format: %v", *first.DoneAt, err)
	}
	if d := time.Since(doneTime).Abs(); d > time.Minute {
		t.Errorf("done_at %s is %s from now", *first.DoneAt, d)
	}

	// Done again: the time is when it was last set to done, so it stays.
	time.Sleep(15 * time.Millisecond)
	b.setStage(b.user, lo, "done")
	if again := b.loadoutOf(b.user, lo); again.DoneAt == nil || *again.DoneAt != *first.DoneAt {
		t.Errorf("done again changed done_at: %s -> %s", *first.DoneAt, str(again.DoneAt))
	}
	// An update that does not name the stage keeps both stage and time.
	b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{"loadout_name": "Renamed"}, http.StatusOK)
	if kept := b.loadoutOf(b.user, lo); kept.Stage != "done" || kept.DoneAt == nil || *kept.DoneAt != *first.DoneAt {
		t.Errorf("a rename changed stage %q done_at %s", kept.Stage, str(kept.DoneAt))
	}

	for _, stage := range []string{"planning", "packing", "planning", "done"} {
		b.setStage(b.user, lo, stage)
		l := b.loadoutOf(b.user, lo)
		if l.Stage != stage {
			t.Fatalf("stage = %q, want %q", l.Stage, stage)
		}
		if (stage == "done") != (l.DoneAt != nil) {
			t.Errorf("stage %q reports done_at %v", stage, l.DoneAt)
		}
		// The list and the board say the same.
		for what, got := range map[string]models.Loadout{"list": b.summary(b.user, lo), "full": b.full(b.user, lo).Loadout} {
			if got.Stage != l.Stage || str(got.DoneAt) != str(l.DoneAt) {
				t.Errorf("%s: stage %q done_at %s, get says %q %s", what, got.Stage, str(got.DoneAt), l.Stage, str(l.DoneAt))
			}
		}
	}
	last := b.loadoutOf(b.user, lo)
	if *last.DoneAt == *first.DoneAt {
		t.Errorf("done after leaving done kept the old time %s", *first.DoneAt)
	}
	// A stage with no time stored: leaving done clears the stored column too.
	b.setStage(b.user, lo, "packing")
	var stored sqlNullInt
	if err := b.env.DB.QueryRow(`SELECT COUNT(*) FROM loadouts WHERE loadoutId = ? AND doneAt IS NOT NULL`, lo).Scan(&stored); err != nil || stored.V != 0 {
		t.Errorf("doneAt still stored after leaving done: %d (err %v)", stored.V, err)
	}
}

// Verification semantics: tick everything and close every place, and move,
// re-quantify and re-role items: the stage and done time never change (also
// BR-025 AC-8: ticking and closing are allowed whatever the stage).
func TestLoadoutStage_ProgressAndEditsNeverChangeStage(t *testing.T) {
	b := newBoard(t)
	pack := b.alicesReg(b.gear("Pack", 900, true))
	spork := b.alicesReg(b.gear("Spork", 11, false))
	for _, stage := range []string{"planning", "packing", "done"} {
		lo := b.newLoadout(b.user, "Trip "+stage)
		bag := b.addBag(b.user, lo, pack)
		last := b.addLastMinute(b.user, lo)
		i1, i2 := b.addItem(b.user, lo, spork, 1), b.addItem(b.user, lo, spork, 2)
		if stage != "planning" {
			b.setStage(b.user, lo, stage)
		}
		before := b.loadoutOf(b.user, lo)

		b.tick(b.user, lo, i1, true, http.StatusOK)
		b.tick(b.user, lo, i2, true, http.StatusOK)
		for _, p := range []int64{bag, last, wornOf(t, b.full(b.user, lo))} {
			b.closePlace(b.user, lo, p, true)
		}
		b.move(b.user, lo, i1, bag)
		b.call(http.MethodPost, itemPath(lo, i2, "update"), b.user, map[string]any{"quantity": 5, "consumable": true}, http.StatusOK)
		b.tick(b.user, lo, i1, false, http.StatusOK)
		b.closePlace(b.user, lo, bag, false)
		b.call(http.MethodDelete, itemPath(lo, i1, "delete"), b.user, nil, http.StatusOK)

		after := b.loadoutOf(b.user, lo)
		if after.Stage != stage || str(after.DoneAt) != str(before.DoneAt) {
			t.Errorf("stage %s: progress and edits changed stage %q done_at %s -> %q %s",
				stage, before.Stage, str(before.DoneAt), after.Stage, str(after.DoneAt))
		}
	}
}

// Verification semantics: an end date before the start date, a goal of 0 and
// stage "packed" are 400 and nothing changes. The same holds for every other
// malformed value of the four fields.
func TestLoadoutStage_RefusalsAre400AndChangeNothing(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Trip")
	b.call(http.MethodPost, loPath(lo, "update"), b.user,
		map[string]any{"stage": "done", "start_date": "2026-07-10", "end_date": "2026-07-14", "weight_goal": 5000}, http.StatusOK)
	before := b.tripState()

	bodies := []string{
		`{"stage":"packed"}`, `{"stage":""}`, `{"stage":"Done"}`, `{"stage":" done"}`, `{"stage":1}`, `{"stage":true}`, `{"stage":["done"]}`,
		`{"start_date":"2026-07-15"}`,                         // after the stored end date
		`{"end_date":"2026-07-09"}`,                           // before the stored start date
		`{"start_date":"2026-08-01","end_date":"2026-07-31"}`, // both named, end before start
		`{"start_date":"2026-07-15","end_date":"2026-07-14"}`,
		`{"weight_goal":0}`, `{"weight_goal":-1}`, `{"weight_goal":1.5}`, `{"weight_goal":2e3}`, `{"weight_goal":"3000"}`,
		`{"weight_goal":true}`, `{"weight_goal":9223372036854775808}`, `{"weight_goal":[1]}`,
		`{"start_date":"2026-13-01"}`, `{"start_date":"2026-02-30"}`, `{"start_date":"2023-02-29"}`, `{"start_date":"2026-1-5"}`,
		`{"start_date":"2026/07/10"}`, `{"start_date":"20260710"}`, `{"start_date":"2026-07-10T00:00:00Z"}`, `{"start_date":"2026-07-10 "}`,
		`{"start_date":""}`, `{"start_date":20260710}`, `{"start_date":true}`, `{"end_date":"tomorrow"}`, `{"end_date":"2026-07-32"}`,
		// A good field next to a bad one: nothing is applied.
		`{"loadout_name":"Other","stage":"packed"}`, `{"loadout_name":"Other","weight_goal":0}`,
		`{"stage":"packing","start_date":"nope"}`,
	}
	for _, body := range bodies {
		w := b.env.Do(http.MethodPost, loPath(lo, "update"), b.user, body)
		expectStatus(t, w, http.StatusBadRequest)
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("%s: body %s names no error", body, w.Body.String())
		}
		if after := b.tripState(); after != before {
			t.Fatalf("%s changed stored state:\nbefore\n%s\nafter\n%s", body, before, after)
		}
	}
	if got := b.loadoutOf(b.user, lo); got.LoadoutName != "Trip" {
		t.Errorf("name = %q, a refused request changed it", got.LoadoutName)
	}
}

// AC-4: set, change and clear each date and the goal; reads reflect each step.
// Absent keeps the value, null removes it, and the end-before-start rule is
// judged on the values after the update.
func TestLoadoutDates_SetChangeClearAndOrderJudgedAfterUpdate(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Trip")
	upd := func(body string, want int) {
		t.Helper()
		expectStatus(t, b.env.Do(http.MethodPost, loPath(lo, "update"), b.user, body), want)
	}
	read := func() models.Loadout { return b.loadoutOf(b.user, lo) }

	upd(`{"start_date":"2026-07-10"}`, http.StatusOK)
	wantTrip(t, "start only", read(), "planning", "2026-07-10", "", 0)
	upd(`{"end_date":"2026-07-14"}`, http.StatusOK)
	wantTrip(t, "end added", read(), "planning", "2026-07-10", "2026-07-14", 0)
	upd(`{"weight_goal":6500}`, http.StatusOK)
	wantTrip(t, "goal added", read(), "planning", "2026-07-10", "2026-07-14", 6500)

	// Absent keeps; a rename touches none of them.
	upd(`{"loadout_name":"Renamed"}`, http.StatusOK)
	wantTrip(t, "rename", read(), "planning", "2026-07-10", "2026-07-14", 6500)

	// Same day is allowed; a window moved as a whole is allowed even though the
	// new start is after the old end: the rule reads the values after the update.
	upd(`{"start_date":"2026-07-14"}`, http.StatusOK)
	wantTrip(t, "start = end", read(), "planning", "2026-07-14", "2026-07-14", 6500)
	upd(`{"start_date":"2026-09-01","end_date":"2026-09-05"}`, http.StatusOK)
	wantTrip(t, "moved window", read(), "planning", "2026-09-01", "2026-09-05", 6500)

	// Clearing the end date lets the start date move past it, in one request or two.
	upd(`{"end_date":null}`, http.StatusOK)
	wantTrip(t, "end cleared", read(), "planning", "2026-09-01", "", 6500)
	upd(`{"start_date":"2026-12-01"}`, http.StatusOK)
	upd(`{"end_date":"2026-11-30"}`, http.StatusBadRequest)
	upd(`{"start_date":null,"end_date":"2026-11-30"}`, http.StatusOK)
	wantTrip(t, "start cleared, end set", read(), "planning", "", "2026-11-30", 6500)

	upd(`{"weight_goal":null}`, http.StatusOK)
	wantTrip(t, "goal cleared", read(), "planning", "", "2026-11-30", 0)
	upd(`{"end_date":null}`, http.StatusOK)
	wantTrip(t, "all cleared", read(), "planning", "", "", 0)

	// null on the stage is "not named", like every other field that is not clearable.
	upd(`{"stage":null}`, http.StatusBadRequest) // nothing left to update
	upd(`{"stage":null,"weight_goal":1}`, http.StatusOK)
	wantTrip(t, "null stage ignored", read(), "planning", "", "", 1)

	for what, l := range map[string]models.Loadout{"list": b.summary(b.user, lo), "full": b.full(b.user, lo).Loadout} {
		wantTrip(t, what, l, "planning", "", "", 1)
	}
}

// Dates the application accepts must be dates the column CHECK accepts, at the
// ends of the calendar and on a leap day, so no accepted request is a 500.
func TestLoadoutDates_EdgeDatesAreStored(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Trip")
	for _, d := range []string{"0001-01-01", "0000-01-01", "2024-02-29", "2000-02-29", "9999-12-31", "2026-01-01"} {
		b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{"start_date": d, "end_date": nil}, http.StatusOK)
		if got := b.loadoutOf(b.user, lo); str(got.StartDate) != d {
			t.Errorf("start_date %s read back as %s", d, str(got.StartDate))
		}
	}
	b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{"weight_goal": int64(9223372036854775807)}, http.StatusOK)
	if got := b.loadoutOf(b.user, lo); got.WeightGoal == nil || *got.WeightGoal != 9223372036854775807 {
		t.Errorf("goal = %v, want the largest whole number", got.WeightGoal)
	}
}

// AC-5: being over the goal is never refused and never changes anything.
func TestLoadoutGoal_OverTheGoalIsNeverRefusedOrActedOn(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Anvil", 3000, false))
	lo := b.newLoadout(b.user, "Trip")
	b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{"weight_goal": 1000, "stage": "packing"}, http.StatusOK)

	item := b.addItem(b.user, lo, reg, 2) // 6 000 g against a 1 000 g goal
	b.tick(b.user, lo, item, true, http.StatusOK)
	b.call(http.MethodPost, itemPath(lo, item, "update"), b.user, map[string]any{"quantity": 3}, http.StatusOK)

	l := b.loadoutOf(b.user, lo)
	wantTrip(t, "over goal", l, "packing", "", "", 1000)
	if l.TotalWeight != 9000 {
		t.Errorf("total = %d, want 9000", l.TotalWeight)
	}
}

// New loadouts: dates and goal are accepted at creation; the stage can only be
// planning; null is the same as absent; refusals create nothing.
func TestLoadoutInsert_TripFieldsAndStage(t *testing.T) {
	b := newBoard(t)

	made := b.call(http.MethodPut, "/api/v1/loadout/insert", b.user,
		map[string]any{"loadout_name": "A", "start_date": "2026-07-10", "end_date": "2026-07-14", "weight_goal": 5000}, http.StatusCreated)
	if made["stage"] != "planning" || made["start_date"] != "2026-07-10" || made["end_date"] != "2026-07-14" || made["weight_goal"] != float64(5000) || made["done_at"] != nil {
		t.Errorf("insert response = %v", made)
	}
	if got := b.loadoutOf(b.user, num(made, "loadout_id")); str(got.StartDate) != "2026-07-10" || got.Stage != "planning" {
		t.Errorf("stored = stage %q start %s", got.Stage, str(got.StartDate))
	}

	// The only stage a new loadout may name is planning (null and absent mean
	// planning too); any other value is a 400 and creates nothing.
	for _, stage := range []string{`"planning"`, `null`} {
		m := b.call(http.MethodPut, "/api/v1/loadout/insert", b.user, `{"loadout_name":"S","stage":`+stage+`}`, http.StatusCreated)
		if m["stage"] != "planning" || m["done_at"] != nil {
			t.Errorf("stage %s on insert: response stage %v done_at %v", stage, m["stage"], m["done_at"])
		}
	}
	created := b.env.Count("loadouts", "1=1")
	for _, stage := range []string{`"done"`, `"packing"`, `"packed"`, `""`, `7`, `true`} {
		expectStatus(t, b.env.Do(http.MethodPut, "/api/v1/loadout/insert", b.user, `{"loadout_name":"S","stage":`+stage+`}`), http.StatusBadRequest)
	}
	if got := b.env.Count("loadouts", "1=1"); got != created {
		t.Errorf("refused stage on insert created %d loadouts", got-created)
	}

	nulls := b.call(http.MethodPut, "/api/v1/loadout/insert", b.user,
		`{"loadout_name":"N","start_date":null,"end_date":null,"weight_goal":null}`, http.StatusCreated)
	if nulls["start_date"] != nil || nulls["end_date"] != nil || nulls["weight_goal"] != nil {
		t.Errorf("null fields on insert = %v", nulls)
	}

	count := func() int { return b.env.Count("loadouts", "1=1") }
	before := count()
	for _, body := range []string{
		`{"loadout_name":"X","start_date":"2026-07-15","end_date":"2026-07-14"}`,
		`{"loadout_name":"X","weight_goal":0}`, `{"loadout_name":"X","weight_goal":"5"}`, `{"loadout_name":"X","weight_goal":1.5}`,
		`{"loadout_name":"X","start_date":"2026-02-30"}`, `{"loadout_name":"X","end_date":"2026-7-4"}`, `{"loadout_name":"X","start_date":5}`,
	} {
		expectStatus(t, b.env.Do(http.MethodPut, "/api/v1/loadout/insert", b.user, body), http.StatusBadRequest)
	}
	if count() != before {
		t.Errorf("refused inserts created %d loadouts", count()-before)
	}
}

// AC-6: only the owner can read or change these values, and a body that names
// another loadout or user never redirects the change.
func TestLoadoutStage_OwnerOnlyAndBodyIdentityNeverRedirects(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Alice")
	bobs := b.newLoadout(b.bTok, "Bob")
	b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{"weight_goal": 4000, "start_date": "2026-07-10"}, http.StatusOK)
	before := b.tripState()

	// Another user, an admin and an unknown loadout all get the same 404, and a
	// bad body does not turn it into 400 (404 first).
	var bodies []string
	for _, tok := range []string{b.bTok, b.admin} {
		for _, body := range []string{`{"stage":"done"}`, `{"stage":"packed"}`, `{"weight_goal":0}`, `{"end_date":null}`} {
			w := b.env.Do(http.MethodPost, loPath(lo, "update"), tok, body)
			expectStatus(t, w, http.StatusNotFound)
			bodies = append(bodies, w.Body.String())
		}
	}
	unknown := b.env.Do(http.MethodPost, loPath(lo+1000, "update"), b.user, `{"stage":"done"}`)
	expectStatus(t, unknown, http.StatusNotFound)
	for _, got := range bodies {
		if got != unknown.Body.String() {
			t.Errorf("foreign 404 body %q differs from the unknown one %q", got, unknown.Body.String())
		}
	}
	for _, tok := range []string{b.bTok, b.admin} {
		expectStatus(t, b.env.Do(http.MethodGet, loPath(lo, "get"), tok, nil), http.StatusNotFound)
		expectStatus(t, b.env.Do(http.MethodGet, loPath(lo, "full"), tok, nil), http.StatusNotFound)
	}
	if b.tripState() != before {
		t.Fatal("a refused request by another user changed stored state")
	}
	// Bob's list does not carry Alice's loadout or its values.
	for _, l := range b.listOf(b.bTok) {
		if *l.LoadoutID == lo {
			t.Errorf("bob's list shows alice's loadout %d", lo)
		}
	}

	// Identity in the body never redirects: a different loadout_id or user_id is 400.
	for _, body := range []map[string]any{
		{"loadout_id": bobs, "stage": "done"},
		{"user_id": b.bob, "stage": "done"},
	} {
		b.call(http.MethodPost, loPath(lo, "update"), b.user, body, http.StatusBadRequest)
	}
	if b.tripState() != before {
		t.Fatal("a body naming another loadout or user changed stored state")
	}
	b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{"loadout_id": lo, "user_id": b.userID, "stage": "packing"}, http.StatusOK)
	if got := b.loadoutOf(b.user, lo).Stage; got != "packing" {
		t.Errorf("stage = %q, want packing", got)
	}
	if got := b.loadoutOf(b.bTok, bobs).Stage; got != "planning" {
		t.Errorf("bob's stage = %q, alice's update leaked", got)
	}
}

// AC-6 for the public view: stage, done time, dates and goal are not part of
// what an anonymous reader gets.
func TestLoadoutStage_PublicViewDoesNotShowTripFields(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Public trip")
	b.call(http.MethodPost, loPath(lo, "update"), b.user,
		map[string]any{"loadout_is_public": true, "loadout_slug": "public-trip", "stage": "done", "start_date": "2026-07-10", "weight_goal": 4000}, http.StatusOK)

	w := b.env.Do(http.MethodGet, "/api/v1/public/loadout/public-trip", "", nil)
	expectStatus(t, w, http.StatusOK)
	for _, k := range []string{`"stage"`, `"done_at"`, `"start_date"`, `"end_date"`, `"weight_goal"`} {
		if strings.Contains(w.Body.String(), k) {
			t.Errorf("public loadout shows %s: %s", k, w.Body.String())
		}
	}
}

// The deployed frontend updates a loadout with a body that knows nothing of the
// new fields. That must not change the stage, dates or goal.
func TestLoadoutStage_DeployedFrontendUpdateShapeKeepsThem(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "Trip")
	b.call(http.MethodPost, loPath(lo, "update"), b.user,
		map[string]any{"stage": "done", "start_date": "2026-07-10", "end_date": "2026-07-14", "weight_goal": 4000}, http.StatusOK)
	done := b.loadoutOf(b.user, lo)

	b.call(http.MethodPost, loPath(lo, "update"), b.user, map[string]any{
		"loadout_id": lo, "loadout_name": "Renamed", "loadout_description": "d2", "loadout_is_public": true, "loadout_slug": "same-slug",
	}, http.StatusOK)
	got := b.loadoutOf(b.user, lo)
	wantTrip(t, "after the old update shape", got, "done", "2026-07-10", "2026-07-14", 4000)
	if str(got.DoneAt) != str(done.DoneAt) {
		t.Errorf("done_at %s -> %s", str(done.DoneAt), str(got.DoneAt))
	}
}

func (b *board) listOf(tok string) []models.Loadout {
	b.env.T.Helper()
	w := b.env.Do(http.MethodGet, "/api/v1/loadout/list", tok, nil)
	expectStatus(b.env.T, w, http.StatusOK)
	var all []models.Loadout
	decodeJSON(b.env.T, w, &all)
	return all
}

// ---- BR-027 -------------------------------------------------------------------

func (b *board) setConsumable(tok string, lo, item int64, value any, want int) {
	b.env.T.Helper()
	expectStatus(b.env.T, b.env.Do(http.MethodPost, itemPath(lo, item, "update"), tok, map[string]any{"consumable": value}), want)
}

func (b *board) consumableOf(lo, item int64) bool {
	b.env.T.Helper()
	return itemOf(b.env.T, b.full(b.user, lo), item).Consumable
}

// Verification semantics: two items of the same registration in L1 and one in
// L2. Mark one L1 item: only that item reports the role; the other two, and
// their places, ticks and weights, are unchanged. Switch it off again.
func TestConsumable_PerItemNotPerRegistrationOrLoadout(t *testing.T) {
	b := newBoard(t)
	bottle := b.alicesReg(b.gear("Water bottle 1 L", 120, false))
	pack := b.alicesReg(b.gear("Pack", 900, true))
	l1, l2 := b.newLoadout(b.user, "L1"), b.newLoadout(b.user, "L2")
	bag := b.addBag(b.user, l1, pack)

	a, c := b.addItem(b.user, l1, bottle, 1), b.addItem(b.user, l1, bottle, 3)
	d := b.addItem(b.user, l2, bottle, 1)
	b.move(b.user, l1, a, bag)
	b.tick(b.user, l1, a, true, http.StatusOK)

	// AC-1: a new item is not a consumable, and every read says so.
	for _, it := range []struct{ lo, id int64 }{{l1, a}, {l1, c}, {l2, d}} {
		if b.consumableOf(it.lo, it.id) {
			t.Fatalf("new item %d is a consumable", it.id)
		}
	}
	raw := b.env.Do(http.MethodGet, loPath(l1, "item/list"), b.user, nil)
	if !strings.Contains(raw.Body.String(), `"consumable":false`) {
		t.Errorf("item list does not carry consumable: %s", raw.Body.String())
	}

	// Everything the board reports except the role, as JSON (pointers compare by value).
	snapshot := func(lo int64) string {
		f := b.full(b.user, lo)
		for i := range f.Items {
			f.Items[i].Consumable = false
		}
		f.Loadout.UpdatedAt = ""
		out, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before1, before2 := snapshot(l1), snapshot(l2)
	b.setConsumable(b.user, l1, c, true, http.StatusOK)

	f1 := b.full(b.user, l1)
	if !itemOf(t, f1, c).Consumable || itemOf(t, f1, a).Consumable || b.consumableOf(l2, d) {
		t.Errorf("roles after marking %d: a=%v c=%v l2=%v, want only c", c, itemOf(t, f1, a).Consumable, itemOf(t, f1, c).Consumable, b.consumableOf(l2, d))
	}
	if after := snapshot(l1); after != before1 {
		t.Errorf("marking an item changed something else in L1 (place, tick, quantity, weight or total):\nbefore %s\nafter  %s", before1, after)
	}
	if after := snapshot(l2); after != before2 {
		t.Errorf("marking an L1 item changed L2")
	}

	// The same registration is a consumable in L2 and not in L1 (the water-bottle example).
	b.setConsumable(b.user, l2, d, true, http.StatusOK)
	b.setConsumable(b.user, l1, c, false, http.StatusOK)
	if b.consumableOf(l1, c) || !b.consumableOf(l2, d) {
		t.Errorf("L1 item = %v, L2 item = %v; want false and true", b.consumableOf(l1, c), b.consumableOf(l2, d))
	}
	if after := snapshot(l1); after != before1 {
		t.Errorf("switching the role off did not restore L1")
	}
	// Setting the value it has succeeds and changes nothing.
	b.setConsumable(b.user, l1, c, false, http.StatusOK)
	b.setConsumable(b.user, l2, d, true, http.StatusOK)
	if b.consumableOf(l1, c) || !b.consumableOf(l2, d) {
		t.Errorf("repeating a role changed it")
	}
}

// AC-1: insert carries the role, default off; false, true and null (absent).
// AC-3 and BR-025 AC-4: the role changes no place, tick, quantity or weight,
// and moving, ticking or re-quantifying never changes the role.
func TestConsumable_InsertDefaultsAndRoleSurvivesOtherEdits(t *testing.T) {
	b := newBoard(t)
	fuel := b.alicesReg(b.gear("Gas 230 g", 360, false))
	pack := b.alicesReg(b.gear("Pack", 900, true))
	lo := b.newLoadout(b.user, "Trip")
	bag := b.addBag(b.user, lo, pack)

	created := func(body any) models.LoadoutItem {
		w := b.env.Do(http.MethodPut, loPath(lo, "item/insert"), b.user, body)
		expectStatus(t, w, http.StatusCreated)
		var it models.LoadoutItem
		decodeJSON(t, w, &it)
		return it
	}
	for _, c := range []struct {
		body any
		want bool
	}{
		{map[string]any{"usergear_registration_id": fuel}, false},
		{map[string]any{"usergear_registration_id": fuel, "consumable": false}, false},
		{`{"usergear_registration_id":` + fmt.Sprint(fuel) + `,"consumable":null}`, false},
		{map[string]any{"usergear_registration_id": fuel, "consumable": true}, true},
	} {
		if it := created(c.body); it.Consumable != c.want {
			t.Errorf("insert %v: consumable = %v, want %v", c.body, it.Consumable, c.want)
		}
	}
	// The response of the insert and the read agree.
	it := created(map[string]any{"usergear_registration_id": fuel, "consumable": true, "quantity": 2})
	if !b.consumableOf(lo, *it.LoadoutItemID) {
		t.Error("the role of an inserted item is not read back")
	}

	// Role first, then everything else: the role stays.
	id := *it.LoadoutItemID
	b.move(b.user, lo, id, bag)
	b.tick(b.user, lo, id, true, http.StatusOK)
	b.call(http.MethodPost, itemPath(lo, id, "update"), b.user, map[string]any{"quantity": 4, "notes": "n"}, http.StatusOK)
	b.closePlace(b.user, lo, bag, true)
	b.closePlace(b.user, lo, bag, false)
	got := itemOf(t, b.full(b.user, lo), id)
	if !got.Consumable || got.PlaceID == nil || *got.PlaceID != bag || !got.Ticked || got.Quantity != 4 {
		t.Errorf("after other edits: consumable %v place %v ticked %v quantity %d", got.Consumable, got.PlaceID, got.Ticked, got.Quantity)
	}

	// Role only: the update response is the item, and place, tick, quantity and totals do not move.
	totalBefore := b.loadoutOf(b.user, lo).TotalWeight
	w := b.env.Do(http.MethodPost, itemPath(lo, id, "update"), b.user, map[string]any{"consumable": false})
	expectStatus(t, w, http.StatusOK)
	var resp models.LoadoutItem
	decodeJSON(t, w, &resp)
	if resp.Consumable || resp.PlaceID == nil || *resp.PlaceID != bag || !resp.Ticked || resp.Quantity != 4 {
		t.Errorf("role-only update response = consumable %v place %v ticked %v quantity %d", resp.Consumable, resp.PlaceID, resp.Ticked, resp.Quantity)
	}
	if after := b.loadoutOf(b.user, lo).TotalWeight; after != totalBefore {
		t.Errorf("total %d -> %d after a role change", totalBefore, after)
	}
	if p := placeOf(b.full(b.user, lo), bag); p == nil || p.Closed {
		t.Errorf("bag after role change = %+v", p)
	}
	// null is "not named", so a body with nothing else has nothing to update.
	b.setConsumable(b.user, lo, id, nil, http.StatusBadRequest)
	if b.consumableOf(lo, id) {
		t.Error("a null consumable switched the role")
	}
}

// BR-027 AC-1, AC-5: a value that is not true or false is 400 and changes
// nothing; the refusal order is 404, then 400, then 409.
func TestConsumable_RefusalsAndOrder(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Gas", 360, false))
	lo := b.newLoadout(b.user, "Trip")
	item := b.addItem(b.user, lo, reg, 1)
	before := b.tripState()

	for _, v := range []string{`"yes"`, `"true"`, `1`, `0`, `[]`, `{}`, `"false"`} {
		w := b.env.Do(http.MethodPost, itemPath(lo, item, "update"), b.user, `{"consumable":`+v+`}`)
		expectStatus(t, w, http.StatusBadRequest)
		w = b.env.Do(http.MethodPut, loPath(lo, "item/insert"), b.user, `{"usergear_registration_id":`+fmt.Sprint(reg)+`,"consumable":`+v+`}`)
		expectStatus(t, w, http.StatusBadRequest)
		// A good field next to the bad one is not applied.
		w = b.env.Do(http.MethodPost, itemPath(lo, item, "update"), b.user, `{"quantity":9,"consumable":`+v+`}`)
		expectStatus(t, w, http.StatusBadRequest)
	}
	if b.tripState() != before {
		t.Fatal("a refused consumable request changed stored state")
	}

	// 404 before 400: an unknown item, a foreign item, an unknown registration
	// and an unknown place beat a bad value.
	for _, c := range []struct{ path, tok, body string }{
		{itemPath(lo, item+1000, "update"), b.user, `{"consumable":"yes"}`},
		{itemPath(lo, item, "update"), b.bTok, `{"consumable":"yes"}`},
		{itemPath(lo, item, "update"), b.user, `{"consumable":"yes","place_id":99999}`},
		{loPath(lo+1000, "item/insert"), b.user, `{"usergear_registration_id":1,"consumable":"yes"}`},
		{loPath(lo, "item/insert"), b.user, `{"usergear_registration_id":99999,"consumable":"yes"}`},
	} {
		method := http.MethodPost
		if strings.HasSuffix(c.path, "/insert") {
			method = http.MethodPut
		}
		expectStatus(t, b.env.Do(method, c.path, c.tok, c.body), http.StatusNotFound)
	}
}

// 400 before 409 and a 409 changes nothing, role included: a bag whose gear
// cannot hold items refuses the move, and a bad role beats that refusal.
func TestConsumable_BadValueBeats409AndRefusedMoveKeepsRole(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)

	code, _, body := f.update(item, map[string]any{"place_id": f.noCapBag, "consumable": "yes"})
	if code != http.StatusBadRequest {
		t.Errorf("bad role into a non-capable bag = %d %s, want 400", code, body)
	}
	code, _, body = f.update(item, map[string]any{"place_id": f.noCapBag, "consumable": true})
	if code != http.StatusConflict {
		t.Errorf("move into a non-capable bag = %d %s, want 409", code, body)
	}
	for _, it := range f.items() {
		if *it.LoadoutItemID == item && (it.Consumable || it.PlaceID != nil) {
			t.Errorf("a refused update was partly applied: consumable %v place %v", it.Consumable, it.PlaceID)
		}
	}
	code, _, body = f.insert(map[string]any{"usergear_registration_id": f.tent, "place_id": f.noCapBag, "consumable": true})
	if code != http.StatusConflict {
		t.Errorf("insert into a non-capable bag = %d %s, want 409", code, body)
	}
	if n := len(f.items()); n != 1 {
		t.Errorf("a refused insert left %d items, want 1", n)
	}
}

// AC-5: only the owner can read or change the role.
func TestConsumable_OwnerOnly(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Gas", 360, false))
	lo := b.newLoadout(b.user, "Trip")
	item := b.addItem(b.user, lo, reg, 1)
	b.setConsumable(b.user, lo, item, true, http.StatusOK)
	bobs := b.newLoadout(b.bTok, "Bob's")
	before := b.tripState()

	for _, tok := range []string{b.bTok, b.admin} {
		b.setConsumable(tok, lo, item, false, http.StatusNotFound)
		expectStatus(t, b.env.Do(http.MethodGet, loPath(lo, "item/list"), tok, nil), http.StatusNotFound)
		expectStatus(t, b.env.Do(http.MethodGet, loPath(lo, "full"), tok, nil), http.StatusNotFound)
		expectStatus(t, b.env.Do(http.MethodPut, loPath(lo, "item/insert"), tok,
			map[string]any{"usergear_registration_id": reg, "consumable": true}), http.StatusNotFound)
	}
	// Naming alice's item through bob's own loadout does not reach it either.
	b.setConsumable(b.bTok, bobs, item, false, http.StatusNotFound)
	if b.tripState() != before || !b.consumableOf(lo, item) {
		t.Fatal("another user's request changed the role")
	}
}

// BR-030 with BR-027: the role belongs to the loadout item, so it survives the
// deletion of the item's registration (the item is kept as a snapshot).
func TestConsumable_SurvivesRegistrationDeleteAndLegacyItems(t *testing.T) {
	b := newBoard(t)
	gear := b.gear("Gas", 360, false)
	reg := b.alicesReg(gear)
	lo := b.newLoadout(b.user, "Trip")
	item := b.addItem(b.user, lo, reg, 1)
	b.setConsumable(b.user, lo, item, true, http.StatusOK)

	b.call(http.MethodDelete, ugPath(reg, "delete"), b.user, nil, http.StatusOK)
	got := itemOf(t, b.full(b.user, lo), item)
	if got.RegistrationExists || !got.Consumable {
		t.Errorf("after the registration was deleted: exists %v consumable %v, want false and true", got.RegistrationExists, got.Consumable)
	}
	b.setConsumable(b.user, lo, item, false, http.StatusOK)
	if b.consumableOf(lo, item) {
		t.Error("the role of a detached item cannot be switched off")
	}

	// The deprecated gear_id insert path takes the role too.
	w := b.env.Do(http.MethodPut, loPath(lo, "item/insert"), b.user, map[string]any{"gear_id": b.gear("Cup", 40, false), "consumable": true})
	expectStatus(t, w, http.StatusCreated)
	var legacy models.LoadoutItem
	decodeJSON(t, w, &legacy)
	if !legacy.Consumable {
		t.Error("a legacy gear_id item ignored consumable")
	}
}
