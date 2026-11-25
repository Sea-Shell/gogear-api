package endpoints

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Copy tests of ARS-001 milestone C (BR-031 AC-1 to AC-8, BR-027 AC-4, BR-014
// AC-2, ARD-001 composition check). All state is made through the real
// routes and JWT middleware, as in loadout_full_test.go; SQL is used only to
// read stored state back, to inject a failure and, for the concurrency test,
// to write the source the way a second session would.

// copyWorld is the BR-031 verification loadout L1: three bags (Lid inside Exos
// 58), Last minute, ten items in various places (two of one registration, one
// of them a consumable), notes, a goal, a description, ticks, two closed bags
// and a closed Worn, stage done, dates, public with a slug. Two of the
// registrations used in L1 are then deleted.
type copyWorld struct {
	*board
	lo                                      int64
	worn, last, bagExos, bagLid, bagPouch   int64
	regExos, regLid, regPouch               int64
	items                                   map[string]int64
	thermosName, zliteName                  string
	copyOnlyCount, leftOutCount, totalItems int
}

func newCopyWorld(t *testing.T) *copyWorld {
	b := newBoard(t)
	w := &copyWorld{board: b, items: map[string]int64{}, thermosName: "Thermos 0.75 L", zliteName: "Z Lite Sol"}
	w.regExos = b.alicesReg(b.gear("Exos 58", 1180, true))
	w.regLid = b.alicesReg(b.gear("Lid", 90, true))
	w.regPouch = b.alicesReg(b.gear("Pouch", 40, true))
	stove := b.alicesReg(b.gear("Stove", 83, false))
	spork := b.alicesReg(b.gear("Spork", 11, false))
	water := b.alicesReg(b.gear("Water bottle 1 L", 150, false))
	tent := b.alicesReg(b.gear("Tent", 900, false))
	jacket := b.alicesReg(b.gear("Jacket", 300, false))
	pad := b.alicesReg(b.gear("Sit pad", 40, false))
	capReg := b.alicesReg(b.gear("Cap", 0, false))
	thermos := b.alicesReg(b.gear(w.thermosName, 400, false))
	zlite := b.alicesReg(b.gear(w.zliteName, 410, false))

	w.lo = b.newLoadout(b.user, "Hardangervidda")
	w.bagExos, w.bagLid, w.bagPouch = b.addBag(b.user, w.lo, w.regExos), b.addBag(b.user, w.lo, w.regLid), b.addBag(b.user, w.lo, w.regPouch)
	b.call(http.MethodPost, pPlacePath(w.lo, w.bagLid, "update"), b.user, map[string]any{"parent_place_id": w.bagExos}, http.StatusOK)
	// Order: Pouch, Exos 58, Lid, so neither place ID order nor insert order is the shown order.
	b.call(http.MethodPost, pPath(w.lo, "order"), b.user, map[string]any{"place_ids": []int64{w.bagPouch, w.bagExos, w.bagLid}}, http.StatusOK)
	w.last = b.addLastMinute(b.user, w.lo)
	w.worn = wornOf(t, b.full(b.user, w.lo))

	add := func(name string, reg, qty int64, notes string, place int64) {
		body := map[string]any{"usergear_registration_id": reg, "quantity": qty, "notes": notes}
		if place != 0 {
			body["place_id"] = place
		}
		w.items[name] = num(b.call(http.MethodPut, loPath(w.lo, "item/insert"), b.user, body, http.StatusCreated), "loadout_item_id")
	}
	add("stove", stove, 1, "isobutane", w.bagExos)
	add("spork", spork, 2, "", w.bagLid)
	add("water full", water, 1, "filled at the lake", w.bagLid)
	add("water spare", water, 2, "spare", w.bagPouch) // same registration as "water full"
	add("tent", tent, 1, "footprint?", 0)
	add("jacket", jacket, 1, "", w.worn)
	add("pad", pad, 1, "", w.last)
	add("cap", capReg, 1, "", w.bagExos)
	add("thermos", thermos, 1, "", w.bagPouch)
	add("zlite", zlite, 1, "", 0)
	b.call(http.MethodPost, itemPath(w.lo, w.items["water full"], "update"), b.user, map[string]any{"consumable": true}, http.StatusOK)

	for _, name := range []string{"stove", "spork", "jacket", "thermos"} {
		b.tick(b.user, w.lo, w.items[name], true, http.StatusOK)
	}
	b.closePlace(b.user, w.lo, w.bagExos, true)
	b.closePlace(b.user, w.lo, w.bagPouch, true)
	b.closePlace(b.user, w.lo, w.worn, true)
	b.call(http.MethodPost, loPath(w.lo, "update"), b.user, map[string]any{
		"loadout_description": "Autumn trip", "weight_goal": 9000, "stage": "done",
		"start_date": "2026-09-10", "end_date": "2026-09-14",
		"loadout_is_public": true, "loadout_slug": "hardangervidda-2026",
	}, http.StatusOK)

	// Two registrations used in L1 are no longer in the closet (SC-004).
	for _, reg := range []int64{thermos, zlite} {
		expectStatus(t, b.env.Do(http.MethodDelete, ugPath(reg, "delete"), b.user, nil), http.StatusOK)
	}
	w.totalItems, w.leftOutCount, w.copyOnlyCount = 10, 2, 8
	return w
}

func (w *copyWorld) copy(tok string, lo int64, body any, want int) models.LoadoutCopyResult {
	w.env.T.Helper()
	r := w.env.Do(http.MethodPost, loPath(lo, "copy"), tok, body)
	expectStatus(w.env.T, r, want)
	var res models.LoadoutCopyResult
	if want == http.StatusCreated {
		decodeJSON(w.env.T, r, &res)
	}
	return res
}

func copyBody(name string, places, notes bool) map[string]any {
	return map[string]any{"loadout_name": name, "copy_places": places, "copy_notes_and_goal": notes}
}

// rowsOf renders every row of a query as text, for before/after comparison.
func (b *board) rowsOf(query string, args ...any) string {
	b.env.T.Helper()
	rows, err := b.env.DB.Query(query, args...)
	if err != nil {
		b.env.T.Fatal(err)
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			b.env.T.Fatal(err)
		}
		sb.WriteString(s + "\n")
	}
	return sb.String()
}

// storedLoadout is every stored column of one loadout, its places and items.
func (b *board) storedLoadout(lo int64) string {
	b.env.T.Helper()
	return b.rowsOf(`SELECT quote(loadoutId)||quote(userId)||quote(loadoutName)||quote(loadoutDescription)||quote(loadoutIsPublic)||quote(loadoutSlug)||quote(stage)||quote(doneAt)||quote(startDate)||quote(endDate)||quote(weightGoal)||quote(createdAt)||quote(updatedAt) FROM loadouts WHERE loadoutId = ?`, lo) +
		b.rowsOf(`SELECT quote(placeId)||'|'||quote(placeKind)||'|'||quote(registrationId)||'|'||quote(parentPlaceId)||'|'||quote(position)||'|'||quote(closed) FROM loadout_places WHERE loadoutId = ? ORDER BY placeId`, lo) +
		b.rowsOf(`SELECT quote(loadoutItemId)||'|'||quote(registrationId)||'|'||quote(placeId)||'|'||quote(quantity)||'|'||quote(notes)||'|'||quote(ticked)||'|'||quote(consumable)||'|'||quote(detachedReason)||'|'||quote(detachedGearName) FROM loadout_items WHERE loadoutId = ? ORDER BY loadoutItemId`, lo)
}

type shapeOpts struct {
	asCopy   bool // leave out detached items; render no tick and no closed place
	noPlaces bool // no item has a place
	noNotes  bool
}

func placeLabel(f models.LoadoutFull, id *int64) string {
	if id == nil {
		return "none"
	}
	p := placeOf(f, *id)
	if p == nil {
		return fmt.Sprintf("MISSING(%d)", *id)
	}
	if p.PlaceKind == "bag" {
		return fmt.Sprintf("bag(%d)", *p.UsergearRegistrationID)
	}
	return p.PlaceKind
}

// shape describes a board without any database ID: its places (bags by
// registration, nesting and position) and its items, in order.
func shape(f models.LoadoutFull, o shapeOpts) (places, items []string) {
	for _, p := range f.Places {
		closed := p.Closed && !o.asCopy
		switch p.PlaceKind {
		case "bag":
			places = append(places, fmt.Sprintf("bag reg=%d parent=%s pos=%d closed=%v", *p.UsergearRegistrationID, placeLabel(f, p.ParentPlaceID), *p.Position, closed))
		default:
			places = append(places, fmt.Sprintf("%s closed=%v", p.PlaceKind, closed))
		}
	}
	for _, it := range f.Items {
		if o.asCopy && it.UsergearRegistrationID == nil {
			continue
		}
		reg, place, notes := "gone", placeLabel(f, it.PlaceID), it.Notes
		if it.UsergearRegistrationID != nil {
			reg = fmt.Sprint(*it.UsergearRegistrationID)
		}
		if o.noPlaces {
			place = "none"
		}
		if o.noNotes {
			notes = ""
		}
		items = append(items, fmt.Sprintf("item reg=%s qty=%d notes=%q consumable=%v ticked=%v place=%s", reg, it.Quantity, notes, it.Consumable, it.Ticked && !o.asCopy, place))
	}
	return places, items
}

func requireShape(t *testing.T, got, want []string, what string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got  %s\n want %s", what, strings.Join(got, "\n      "), strings.Join(want, "\n      "))
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (w *copyWorld) loadoutCount() int {
	return w.env.Count("loadouts", "")
}

// ---- BR-031 AC-2 to AC-6, BR-027 AC-4: the verification loadout --------------

func TestCopy_BothOptionsOn(t *testing.T) {
	w := newCopyWorld(t)
	src := w.full(w.user, w.lo)
	srcStored, srcJSON := w.storedLoadout(w.lo), jsonOf(t, src)
	if len(src.Items) != w.totalItems || w.env.Count("loadout_items", "loadoutId = ? AND registrationId IS NULL", w.lo) != w.leftOutCount {
		t.Fatalf("setup: %d items, want %d with %d detached", len(src.Items), w.totalItems, w.leftOutCount)
	}

	res := w.copy(w.user, w.lo, copyBody("Autumn copy", true, true), http.StatusCreated)

	// The new loadout (AC-1, AC-4, AC-5, AC-8).
	l := res.Loadout
	if l.LoadoutID == nil || *l.LoadoutID == w.lo || l.UserID != w.userID || l.LoadoutName != "Autumn copy" {
		t.Errorf("new loadout identity = %+v", l)
	}
	if l.LoadoutDescription != "Autumn trip" || l.WeightGoal == nil || *l.WeightGoal != 9000 {
		t.Errorf("description %q and goal %v, want the source's", l.LoadoutDescription, l.WeightGoal)
	}
	if l.Stage != "planning" || l.DoneAt != nil || l.StartDate != nil || l.EndDate != nil || l.LoadoutIsPublic || l.LoadoutSlug != "" {
		t.Errorf("stage/dates/public/slug = %q %v %v %v %v %q, want planning, none, private, no slug", l.Stage, l.DoneAt, l.StartDate, l.EndDate, l.LoadoutIsPublic, l.LoadoutSlug)
	}
	if l.ItemCount != int64(w.copyOnlyCount) || l.TickedCount != 0 || l.BagCount != 3 || len(l.ClosedPlaces) != 0 {
		t.Errorf("counts = items %d ticked %d bags %d closed %v", l.ItemCount, l.TickedCount, l.BagCount, l.ClosedPlaces)
	}

	// AC-2: the two items whose registration is gone are left out and named.
	if len(res.LeftOut) != 2 ||
		res.LeftOut[0] != (models.LoadoutLeftOutItem{LoadoutItemID: w.items["thermos"], GearName: w.thermosName}) ||
		res.LeftOut[1] != (models.LoadoutLeftOutItem{LoadoutItemID: w.items["zlite"], GearName: w.zliteName}) {
		t.Errorf("left_out = %+v", res.LeftOut)
	}

	// AC-2, AC-3, BR-027 AC-4: the same bags, nesting, order, places, quantities,
	// notes and roles; no tick and nothing closed.
	cp := w.full(w.user, *l.LoadoutID)
	if len(cp.Items) != 8 {
		t.Fatalf("copy has %d items, want 8", len(cp.Items))
	}
	wantPlaces, wantItems := shape(src, shapeOpts{asCopy: true})
	gotPlaces, gotItems := shape(cp, shapeOpts{})
	requireShape(t, gotPlaces, wantPlaces, "places")
	requireShape(t, gotItems, wantItems, "items")
	// The expectation above is derived from the source; these are written out.
	requireShape(t, gotPlaces, []string{
		"worn closed=false", "last_minute closed=false",
		fmt.Sprintf("bag reg=%d parent=none pos=1 closed=false", w.regPouch),
		fmt.Sprintf("bag reg=%d parent=none pos=2 closed=false", w.regExos),
		fmt.Sprintf("bag reg=%d parent=bag(%d) pos=3 closed=false", w.regLid, w.regExos),
	}, "places, written out")
	var consumables, spares int
	for _, it := range cp.Items {
		if it.Consumable {
			consumables++
			if it.GearName != "Water bottle 1 L" || it.Quantity != 1 {
				t.Errorf("consumable = %+v", it)
			}
		}
		if it.GearName == "Water bottle 1 L" && !it.Consumable {
			spares++ // the other item of the same registration did not take the role
		}
		if it.Ticked || it.DetachedReason != nil || !it.RegistrationExists {
			t.Errorf("copied item %+v is ticked or detached", it)
		}
	}
	if consumables != 1 || spares != 1 {
		t.Errorf("consumable items = %d, non-consumable items of the same registration = %d, want 1 and 1", consumables, spares)
	}
	srcIDs := map[int64]bool{}
	for _, it := range src.Items {
		srcIDs[*it.LoadoutItemID] = true
	}
	for _, it := range cp.Items {
		if srcIDs[*it.LoadoutItemID] {
			t.Errorf("copied item reuses the source item ID %d", *it.LoadoutItemID)
		}
	}
	if placeOf(cp, wornOf(t, cp)).Closed {
		t.Error("the copy's Worn is closed")
	}
	if w.env.Count("loadout_places", "loadoutId = ? AND placeKind = 'worn'", *l.LoadoutID) != 1 {
		t.Error("the copy does not have exactly one Worn")
	}

	// AC-6: the source is unchanged, in storage and as read.
	if got := w.storedLoadout(w.lo); got != srcStored {
		t.Errorf("source changed in storage:\nbefore:\n%s\nafter:\n%s", srcStored, got)
	}
	if got := jsonOf(t, w.full(w.user, w.lo)); got != srcJSON {
		t.Errorf("source changed as read:\nbefore %s\nafter  %s", srcJSON, got)
	}
}

func TestCopy_BothOptionsOff(t *testing.T) {
	w := newCopyWorld(t)
	src := w.full(w.user, w.lo)
	srcStored := w.storedLoadout(w.lo)

	res := w.copy(w.user, w.lo, copyBody("Bare copy", false, false), http.StatusCreated)

	l := res.Loadout
	if l.LoadoutDescription != "" || l.WeightGoal != nil || l.LoadoutName != "Bare copy" {
		t.Errorf("description %q goal %v name %q, want none", l.LoadoutDescription, l.WeightGoal, l.LoadoutName)
	}
	if l.ItemCount != 8 || l.BagCount != 0 || l.TickedCount != 0 || len(l.ClosedPlaces) != 0 || len(res.LeftOut) != 2 {
		t.Errorf("counts = items %d bags %d ticked %d closed %v, left out %d", l.ItemCount, l.BagCount, l.TickedCount, l.ClosedPlaces, len(res.LeftOut))
	}
	cp := w.full(w.user, *l.LoadoutID)
	// No bags, no Last minute; only the Worn every loadout has (AC-3).
	_, wantItems := shape(src, shapeOpts{asCopy: true, noPlaces: true, noNotes: true})
	gotPlaces, gotItems := shape(cp, shapeOpts{})
	requireShape(t, gotPlaces, []string{"worn closed=false"}, "places")
	requireShape(t, gotItems, wantItems, "items")
	for _, it := range cp.Items {
		// Every copied item has no place, including the one that was in Worn.
		if it.PlaceID != nil || it.Notes != "" {
			t.Errorf("item %+v has a place or notes", it)
		}
		if it.GearName == "Jacket" && it.PlaceID != nil {
			t.Error("the jacket that was in Worn is not in a place in the copy")
		}
	}
	// Quantities and the role come even with both options off (AC-2).
	var qty int64
	for _, it := range cp.Items {
		qty += it.Quantity
	}
	if qty != 1+2+1+2+1+1+1+1 {
		t.Errorf("quantities add up to %d", qty)
	}
	if got := w.storedLoadout(w.lo); got != srcStored {
		t.Error("source changed in storage")
	}
}

// The two options act independently of each other.
func TestCopy_OptionsAreIndependent(t *testing.T) {
	w := newCopyWorld(t)
	src := w.full(w.user, w.lo)

	places := w.copy(w.user, w.lo, copyBody("places only", true, false), http.StatusCreated)
	cp := w.full(w.user, *places.Loadout.LoadoutID)
	gp, gi := shape(cp, shapeOpts{})
	wp, wi := shape(src, shapeOpts{asCopy: true, noNotes: true})
	requireShape(t, gp, wp, "places (places only)")
	requireShape(t, gi, wi, "items (places only)")
	if places.Loadout.LoadoutDescription != "" || places.Loadout.WeightGoal != nil {
		t.Errorf("places only copied description %q / goal %v", places.Loadout.LoadoutDescription, places.Loadout.WeightGoal)
	}

	notes := w.copy(w.user, w.lo, copyBody("notes only", false, true), http.StatusCreated)
	cp = w.full(w.user, *notes.Loadout.LoadoutID)
	gp, gi = shape(cp, shapeOpts{})
	_, wi = shape(src, shapeOpts{asCopy: true, noPlaces: true})
	requireShape(t, gp, []string{"worn closed=false"}, "places (notes only)")
	requireShape(t, gi, wi, "items (notes only)")
	if notes.Loadout.LoadoutDescription != "Autumn trip" || notes.Loadout.WeightGoal == nil || *notes.Loadout.WeightGoal != 9000 || notes.Loadout.BagCount != 0 {
		t.Errorf("notes only = %+v", notes.Loadout)
	}
}

func TestCopy_NothingToLeaveOutGivesAnEmptyArray(t *testing.T) {
	w := newBoard(t)
	empty := w.newLoadout(w.user, "empty")
	r := w.env.Do(http.MethodPost, loPath(empty, "copy"), w.user, copyBody("empty copy", true, true))
	expectStatus(t, r, http.StatusCreated)
	var raw map[string]any
	decodeJSON(t, r, &raw)
	if left, ok := raw["left_out"].([]any); !ok || len(left) != 0 {
		t.Errorf("left_out = %#v, want []", raw["left_out"])
	}
	lo := raw["loadout"].(map[string]any)
	if num(lo, "item_count") != 0 || num(lo, "bag_count") != 0 || num(lo, "ticked_count") != 0 {
		t.Errorf("empty copy = %v", lo)
	}
}

// A legacy item has no registration (ADR-004 D-4), so it is left out, and it is
// named by the live name of its gear.
func TestCopy_LegacyItemIsLeftOutByItsLiveName(t *testing.T) {
	w := newCopyWorld(t)
	mug := w.gear("Legacy mug", 120, false) // never registered by alice
	lo := w.newLoadout(w.user, "legacy")
	spork := w.alicesReg(w.gear("Spork 2", 11, false))
	kept := w.addItem(w.user, lo, spork, 1)
	legacy := num(w.call(http.MethodPut, loPath(lo, "item/insert"), w.user, map[string]any{"gear_id": mug, "quantity": 1}, http.StatusCreated), "loadout_item_id")
	if it := itemOf(t, w.full(w.user, lo), legacy); it.DetachedReason == nil || *it.DetachedReason != "legacy" {
		t.Fatalf("setup: item %+v is not a legacy item", it)
	}

	res := w.copy(w.user, lo, copyBody("c", true, true), http.StatusCreated)
	if len(res.LeftOut) != 1 || res.LeftOut[0] != (models.LoadoutLeftOutItem{LoadoutItemID: legacy, GearName: "Legacy mug"}) {
		t.Errorf("left_out = %+v", res.LeftOut)
	}
	if cp := w.full(w.user, *res.Loadout.LoadoutID); len(cp.Items) != 1 || cp.Items[0].GearName != "Spork 2" || *cp.Items[0].LoadoutItemID == kept {
		t.Errorf("copy items = %+v", cp.Items)
	}
}

// BR-024 AC-6 and BR-031 AC-3: a bag whose gear can no longer hold items is
// copied like the others, keeps what it holds, and takes nothing new.
func TestCopy_BagThatLostCapabilityIsCopiedAsABag(t *testing.T) {
	w := newCopyWorld(t)
	w.call(http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", w.bagCat), w.admin, map[string]any{"category_container": "not_container"}, http.StatusOK)

	res := w.copy(w.user, w.lo, copyBody("after the category change", true, true), http.StatusCreated)
	cp := w.full(w.user, *res.Loadout.LoadoutID)
	wp, wi := shape(w.full(w.user, w.lo), shapeOpts{asCopy: true})
	gp, gi := shape(cp, shapeOpts{})
	requireShape(t, gp, wp, "places")
	requireShape(t, gi, wi, "items")

	var bag int64
	for _, p := range cp.Places {
		if p.PlaceKind == "bag" && *p.UsergearRegistrationID == w.regPouch {
			bag = p.PlaceID
			if p.GearIsContainer == nil || *p.GearIsContainer {
				t.Errorf("the bag still reads as a container: %+v", p)
			}
		}
	}
	for _, it := range cp.Items {
		if it.GearName == "Tent" {
			w.call(http.MethodPost, itemPath(*res.Loadout.LoadoutID, *it.LoadoutItemID, "update"), w.user, map[string]any{"place_id": bag}, http.StatusConflict)
			return
		}
	}
	t.Fatal("the copy has no Tent item")
}

// ---- BR-031 AC-1, BR-014 AC-2: only the owner copies -------------------------

func TestCopy_OnlyTheOwnerCanCopy(t *testing.T) {
	w := newCopyWorld(t)
	bobsPublic := w.newLoadout(w.bTok, "bob public")
	w.call(http.MethodPost, loPath(bobsPublic, "update"), w.bTok, map[string]any{"loadout_is_public": true, "loadout_slug": "bobs"}, http.StatusOK)
	bobsPrivate := w.newLoadout(w.bTok, "bob private")
	w.call(http.MethodPost, loPath(w.lo, "update"), w.user, map[string]any{"loadout_is_public": true}, http.StatusOK)
	before := w.stored()

	body := copyBody("stolen", true, true)
	unknown := w.env.Do(http.MethodPost, loPath(w.lo+1000, "copy"), w.user, body)
	expectStatus(t, unknown, http.StatusNotFound)
	for name, c := range map[string]struct {
		tok string
		lo  int64
	}{
		"another user's public loadout":  {w.user, bobsPublic},
		"another user's private loadout": {w.user, bobsPrivate},
		"alice's loadout, as bob":        {w.bTok, w.lo},
	} {
		r := w.env.Do(http.MethodPost, loPath(c.lo, "copy"), c.tok, body)
		if r.Code != http.StatusNotFound || r.Body.String() != unknown.Body.String() {
			t.Errorf("%s: %d %s, want the unknown-loadout 404 %s", name, r.Code, r.Body, unknown.Body)
		}
		// 404 before 400: an invalid body does not change the answer (ARS-001 s. 1).
		if r := w.env.Do(http.MethodPost, loPath(c.lo, "copy"), c.tok, `{"loadout_name": 5}`); r.Code != http.StatusNotFound {
			t.Errorf("%s with a bad body: %d, want 404", name, r.Code)
		}
	}
	// Admins get no exception (BR-014 AC-6).
	expectStatus(t, w.env.Do(http.MethodPost, loPath(w.lo, "copy"), w.admin, body), http.StatusNotFound)
	expectStatus(t, w.env.Do(http.MethodPost, loPath(w.lo, "copy"), "", body), http.StatusUnauthorized)
	expectStatus(t, w.env.Do(http.MethodPost, "/api/v1/loadout/x/copy", w.user, body), http.StatusBadRequest)
	if got := w.stored(); got != before {
		t.Fatalf("refused copies changed stored data:\nbefore:\n%s\nafter:\n%s", before, got)
	}

	// The copy is owned by the caller, whoever else holds a public loadout.
	res := w.copy(w.user, w.lo, body, http.StatusCreated)
	if res.Loadout.UserID != w.userID || w.env.Count("loadouts", "userId = ? AND loadoutName = 'stolen'", w.userID) != 1 {
		t.Errorf("copy owner = %d", res.Loadout.UserID)
	}
	if res.Loadout.LoadoutIsPublic || res.Loadout.LoadoutSlug != "" {
		t.Error("the copy of a public loadout is public or has a slug")
	}
	expectStatus(t, w.env.Do(http.MethodGet, loPath(*res.Loadout.LoadoutID, "get"), w.bTok, nil), http.StatusNotFound)
}

// ---- BR-031 AC-8: every option is stated -------------------------------------

func TestCopy_EveryOptionMustBeStated(t *testing.T) {
	w := newCopyWorld(t)
	before := w.stored()
	for name, body := range map[string]any{
		"no name":                  map[string]any{"copy_places": true, "copy_notes_and_goal": true},
		"null name":                map[string]any{"loadout_name": nil, "copy_places": true, "copy_notes_and_goal": true},
		"number name":              map[string]any{"loadout_name": 5, "copy_places": true, "copy_notes_and_goal": true},
		"no copy_places":           map[string]any{"loadout_name": "x", "copy_notes_and_goal": true},
		"null copy_places":         map[string]any{"loadout_name": "x", "copy_places": nil, "copy_notes_and_goal": true},
		"string copy_places":       map[string]any{"loadout_name": "x", "copy_places": "true", "copy_notes_and_goal": true},
		"number copy_places":       map[string]any{"loadout_name": "x", "copy_places": 1, "copy_notes_and_goal": true},
		"no copy_notes_and_goal":   map[string]any{"loadout_name": "x", "copy_places": true},
		"null copy_notes_and_goal": map[string]any{"loadout_name": "x", "copy_places": true, "copy_notes_and_goal": nil},
		"object copy_notes":        map[string]any{"loadout_name": "x", "copy_places": true, "copy_notes_and_goal": map[string]any{}},
		"empty object":             map[string]any{},
		"empty body":               "",
		"not JSON":                 "{",
		"a JSON array":             "[]",
		"loadout_id of another":    map[string]any{"loadout_name": "x", "copy_places": true, "copy_notes_and_goal": true, "loadout_id": w.lo + 1},
		"user_id of another":       map[string]any{"loadout_name": "x", "copy_places": true, "copy_notes_and_goal": true, "user_id": w.bob},
	} {
		r := w.env.Do(http.MethodPost, loPath(w.lo, "copy"), w.user, body)
		if r.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400; body %s", name, r.Code, r.Body)
		}
	}
	if got := w.stored(); got != before {
		t.Fatalf("a refused copy changed stored data:\nbefore:\n%s\nafter:\n%s", before, got)
	}

	// Stating false is stating it; identity fields that agree are harmless.
	w.copy(w.user, w.lo, map[string]any{"loadout_name": "x", "copy_places": false, "copy_notes_and_goal": false, "loadout_id": w.lo, "user_id": w.userID}, http.StatusCreated)
	// The name follows the rules of loadout insert, which accepts any string.
	w.call(http.MethodPut, "/api/v1/loadout/insert", w.user, map[string]any{"loadout_name": ""}, http.StatusCreated)
	if got := w.copy(w.user, w.lo, copyBody("", true, true), http.StatusCreated); got.Loadout.LoadoutName != "" {
		t.Errorf("name = %q", got.Loadout.LoadoutName)
	}
}

// ---- BR-031 AC-7: all or nothing ---------------------------------------------

func TestCopy_AFailureAnywhereLeavesNothing(t *testing.T) {
	for name, trigger := range map[string]string{
		"after the first copied item":  `CREATE TRIGGER fail_copy BEFORE INSERT ON loadout_items WHEN NEW.loadoutId > %d AND (SELECT COUNT(*) FROM loadout_items WHERE loadoutId = NEW.loadoutId) >= 1 BEGIN SELECT RAISE(ABORT, 'injected failure'); END`,
		"on the last copied item":      `CREATE TRIGGER fail_copy AFTER INSERT ON loadout_items WHEN NEW.loadoutId > %d AND (SELECT COUNT(*) FROM loadout_items WHERE loadoutId = NEW.loadoutId) >= 8 BEGIN SELECT RAISE(ABORT, 'injected failure'); END`,
		"on the first bag":             `CREATE TRIGGER fail_copy BEFORE INSERT ON loadout_places WHEN NEW.loadoutId > %d AND NEW.placeKind = 'bag' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`,
		"while setting a bag's parent": `CREATE TRIGGER fail_copy BEFORE UPDATE OF parentPlaceId ON loadout_places WHEN NEW.loadoutId > %d BEGIN SELECT RAISE(ABORT, 'injected failure'); END`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newCopyWorld(t)
			before, loadouts := w.stored(), w.loadoutCount()
			if _, err := w.env.DB.Exec(fmt.Sprintf(trigger, w.lo)); err != nil {
				t.Fatal(err)
			}
			r := w.env.Do(http.MethodPost, loPath(w.lo, "copy"), w.user, copyBody("doomed", true, true))
			if r.Code != http.StatusInternalServerError {
				t.Fatalf("status %d, want 500; body %s", r.Code, r.Body)
			}
			if strings.Contains(r.Body.String(), "injected") {
				t.Errorf("the response leaks the database message: %s", r.Body)
			}
			if got := w.stored(); got != before || w.loadoutCount() != loadouts {
				t.Fatalf("a failed copy left data behind:\nbefore:\n%s\nafter:\n%s", before, got)
			}
			// The same request succeeds once the failure is gone, so the failure above was the injected one.
			if _, err := w.env.DB.Exec(`DROP TRIGGER fail_copy`); err != nil {
				t.Fatal(err)
			}
			w.copy(w.user, w.lo, copyBody("fine", true, true), http.StatusCreated)
		})
	}
}

// ---- ARD-001 composition check -----------------------------------------------

// Delete a registration that is a bag containing a bag with ticked items (and
// is also an item), then copy: the detached item is left out, the nesting that
// is left is correct, and nothing is ticked or closed.
func TestCopy_AfterDeletingABagRegistration(t *testing.T) {
	b := newBoard(t)
	exos := b.alicesReg(b.gear("Exos 58", 1180, true))
	lid := b.alicesReg(b.gear("Lid", 90, true))
	duffel := b.alicesReg(b.gear("Duffel", 500, true))
	stove := b.alicesReg(b.gear("Stove", 83, false))
	pad := b.alicesReg(b.gear("Pad", 40, false))
	b.closetLink(b.user, exos, lid) // Lid starts inside Exos 58

	lo := b.newLoadout(b.user, "L2")
	bagExos, bagLid, bagDuffel := b.addBag(b.user, lo, exos), b.addBag(b.user, lo, lid), b.addBag(b.user, lo, duffel)
	if p := placeOf(b.full(b.user, lo), bagLid); p == nil || p.ParentPlaceID == nil || *p.ParentPlaceID != bagExos {
		t.Fatalf("setup: Lid should start inside Exos 58: %+v", p)
	}
	iStove, iPad := b.addItem(b.user, lo, stove, 1), b.addItem(b.user, lo, pad, 2)
	b.move(b.user, lo, iStove, bagExos)
	b.move(b.user, lo, iPad, bagLid)
	iExos := num(b.call(http.MethodPut, loPath(lo, "item/insert"), b.user,
		map[string]any{"usergear_registration_id": exos, "notes": "borrowed", "place_id": bagDuffel}, http.StatusCreated), "loadout_item_id")
	for _, i := range []int64{iStove, iPad, iExos} {
		b.tick(b.user, lo, i, true, http.StatusOK)
	}
	b.closePlace(b.user, lo, bagExos, true)
	b.closePlace(b.user, lo, bagLid, true)
	expectStatus(t, b.env.Do(http.MethodDelete, ugPath(exos, "delete"), b.user, nil), http.StatusOK)
	srcStored := b.storedLoadout(lo)

	res := (&copyWorld{board: b}).copy(b.user, lo, copyBody("L2 copy", true, true), http.StatusCreated)

	if len(res.LeftOut) != 1 || res.LeftOut[0] != (models.LoadoutLeftOutItem{LoadoutItemID: iExos, GearName: "Exos 58"}) {
		t.Errorf("left_out = %+v, want the detached Exos 58 item by its snapshot name", res.LeftOut)
	}
	cp := b.full(b.user, *res.Loadout.LoadoutID)
	gp, gi := shape(cp, shapeOpts{})
	requireShape(t, gp, []string{
		"worn closed=false",
		fmt.Sprintf("bag reg=%d parent=none pos=2 closed=false", lid), // lifted to top level by the delete
		fmt.Sprintf("bag reg=%d parent=none pos=3 closed=false", duffel),
	}, "places")
	requireShape(t, gi, []string{
		fmt.Sprintf("item reg=%d qty=1 notes=\"\" consumable=false ticked=false place=none", stove),
		fmt.Sprintf("item reg=%d qty=2 notes=\"\" consumable=false ticked=false place=bag(%d)", pad, lid),
	}, "items")
	if l := res.Loadout; l.TickedCount != 0 || len(l.ClosedPlaces) != 0 || l.ItemCount != 2 || l.BagCount != 2 {
		t.Errorf("copy summary = %+v", l)
	}
	if got := b.storedLoadout(lo); got != srcStored {
		t.Errorf("source changed:\nbefore:\n%s\nafter:\n%s", srcStored, got)
	}
}

// ---- concurrency (BR-031 AC-7) -----------------------------------------------

// A second session changes a loadout, its description and an item, in single
// transactions, while copies are made. Every copy shows one of the states,
// never a mix of two. Several copies at once all succeed and are complete.
func TestCopy_ConcurrentCopiesSeeOneMoment(t *testing.T) {
	b := newBoard(t)
	pack := b.alicesReg(b.gear("Pack", 1000, true))
	spork := b.alicesReg(b.gear("Spork", 11, false))
	lo := b.newLoadout(b.user, "v1")
	bag := b.addBag(b.user, lo, pack)
	item := b.addItem(b.user, lo, spork, 1)
	b.move(b.user, lo, item, bag)
	// Start in state 1: description "v1", quantity 1, notes "n1".
	for _, q := range []string{
		fmt.Sprintf(`UPDATE loadouts SET loadoutDescription = 'v1' WHERE loadoutId = %d`, lo),
		fmt.Sprintf(`UPDATE loadout_items SET quantity = 1, notes = 'n1' WHERE loadoutItemId = %d`, item),
	} {
		if _, err := b.env.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	const writes, copiers, perCopier = 40, 6, 4
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // the other session: description, quantity and notes always change together
		defer wg.Done()
		for i := 0; i < writes; i++ {
			n := 1 + i%2
			tx, err := b.env.DB.Begin()
			if err != nil {
				t.Errorf("writer: %v", err)
				return
			}
			_, e1 := tx.Exec(`UPDATE loadouts SET loadoutDescription = ? WHERE loadoutId = ?`, fmt.Sprintf("v%d", n), lo)
			_, e2 := tx.Exec(`UPDATE loadout_items SET quantity = ?, notes = ? WHERE loadoutItemId = ?`, n, fmt.Sprintf("n%d", n), item)
			if e1 != nil || e2 != nil {
				_ = tx.Rollback()
				t.Errorf("writer: %v %v", e1, e2)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Errorf("writer: %v", err)
				return
			}
		}
	}()
	var mu sync.Mutex
	var copies []int64
	for c := 0; c < copiers; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perCopier; i++ {
				r := b.env.Do(http.MethodPost, loPath(lo, "copy"), b.user, copyBody(fmt.Sprintf("c%d-%d", c, i), true, true))
				if r.Code != http.StatusCreated {
					t.Errorf("copy status %d: %s", r.Code, r.Body)
					return
				}
				var res models.LoadoutCopyResult
				decodeJSON(t, r, &res)
				mu.Lock()
				copies = append(copies, *res.Loadout.LoadoutID)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(copies) != copiers*perCopier {
		t.Fatalf("%d copies made, want %d", len(copies), copiers*perCopier)
	}
	seen := map[int64]bool{}
	for _, id := range copies {
		if seen[id] {
			t.Fatalf("copy %d was returned twice", id)
		}
		seen[id] = true
		f := b.full(b.user, id)
		if len(f.Items) != 1 || f.Loadout.BagCount != 1 || f.Items[0].PlaceID == nil {
			t.Fatalf("copy %d is incomplete: %+v", id, f)
		}
		it := f.Items[0]
		wantQty := int64(1)
		if f.Loadout.LoadoutDescription == "v2" {
			wantQty = 2
		}
		if it.Quantity != wantQty || it.Notes != fmt.Sprintf("n%d", wantQty) {
			t.Errorf("copy %d mixes two moments: description %q, quantity %d, notes %q", id, f.Loadout.LoadoutDescription, it.Quantity, it.Notes)
		}
	}
	if b.env.Count("loadouts", "userId = ?", b.userID) != 1+copiers*perCopier {
		t.Error("a copy is missing or doubled")
	}
}
