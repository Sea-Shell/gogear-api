package endpoints

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Composition tests of milestone A (ARS-001 section 3 and 4, ARD-001 "Normal
// example", BR-025, BR-029, BR-030). Every piece of state is made through the
// real routes and the real JWT middleware: the catalog by an admin, then the
// owner's registrations, container links, loadouts, bags, items, ticks,
// closes and deletes. SQL is used only to read stored state back and to
// inject a failure.
//
// Weights here are catalog weights: measured weights belong to milestone B.

type board struct {
	*apiWorld
	bob    int64
	bTok   string
	bagTop int64 // a container top category
	bagCat int64 // a category under it, so its gear can hold items
	regs   map[int64]int64
}

func newBoard(t *testing.T) *board {
	w := newAPIWorld(t)
	b := &board{apiWorld: w, regs: map[int64]int64{}}
	b.bob = w.env.SeedUser("bob", "bob@example.com", false)
	b.bTok = w.env.Token(b.bob, false)
	b.bagTop = w.topInsert("Bags", map[string]any{"top_category_is_container": true})
	b.bagCat = w.idOf(w.catInsert("Backpacks", b.bagTop, map[string]any{}), "category_id")
	return b
}

func (b *board) call(method, path, tok string, body any, want int) map[string]any {
	b.env.T.Helper()
	w := b.env.Do(method, path, tok, body)
	expectStatus(b.env.T, w, want)
	var m map[string]any
	if strings.HasPrefix(strings.TrimSpace(w.Body.String()), "{") {
		decodeJSON(b.env.T, w, &m)
	}
	return m
}

func num(m map[string]any, key string) int64 {
	f, ok := m[key].(float64)
	if !ok {
		panic(fmt.Sprintf("%s is not a number in %v", key, m))
	}
	return int64(f)
}

// gear creates catalog gear as admin. weight 0 sends no weight. A container
// gets the container category, anything else the plain seeded one.
func (b *board) gear(name string, weight int, container bool) int64 {
	b.env.T.Helper()
	body := map[string]any{"gear_name": name, "gear_manufacture_id": 1, "gear_top_category_id": 1, "gear_category_id": 1}
	if container {
		body["gear_top_category_id"], body["gear_category_id"] = b.bagTop, b.bagCat
	}
	if weight > 0 {
		body["gear_weight"] = weight
	}
	m := b.call(http.MethodPut, "/api/v1/gear/insert", b.admin, body, http.StatusOK)
	return num(m, "gear_id")
}

// register registers gear for the owner of tok and returns the registration.
func (b *board) register(tok string, owner, gear int64) int64 {
	b.env.T.Helper()
	b.call(http.MethodPut, "/api/v1/usergear/insert", tok, map[string]any{"usergear_gear_id": gear}, http.StatusOK)
	r := b.env.Do(http.MethodGet, ugList(owner, "limit=100"), tok, nil)
	expectStatus(b.env.T, r, http.StatusOK)
	items, _ := page(b.env.T, r)
	var id int64
	for _, it := range items {
		if int64(it["gear_id"].(float64)) == gear && int64(it["usergear_registration_id"].(float64)) > id {
			id = int64(it["usergear_registration_id"].(float64))
		}
	}
	if id == 0 {
		b.env.T.Fatalf("no registration of gear %d", gear)
	}
	return id
}

func (b *board) alicesReg(gear int64) int64 {
	b.env.T.Helper()
	id := b.register(b.user, b.userID, gear)
	b.regs[gear] = id
	return id
}

func (b *board) closetLink(tok string, container, content int64) {
	b.env.T.Helper()
	b.call(http.MethodPut, "/api/v1/container/insert", tok,
		map[string]any{"user_container_id": container, "user_gear_registration_id": content}, http.StatusOK)
}

func (b *board) newLoadout(tok, name string) int64 {
	b.env.T.Helper()
	return num(b.call(http.MethodPut, "/api/v1/loadout/insert", tok, map[string]any{"loadout_name": name}, http.StatusCreated), "loadout_id")
}

func (b *board) addBag(tok string, lo, reg int64) int64 {
	b.env.T.Helper()
	return num(b.call(http.MethodPut, pPath(lo, "insert"), tok,
		map[string]any{"place_kind": "bag", "usergear_registration_id": reg}, http.StatusCreated), "place_id")
}

func (b *board) addLastMinute(tok string, lo int64) int64 {
	b.env.T.Helper()
	return num(b.call(http.MethodPut, pPath(lo, "insert"), tok, map[string]any{"place_kind": "last_minute"}, http.StatusCreated), "place_id")
}

// addItem adds registration reg with the quantity and no place.
func (b *board) addItem(tok string, lo, reg, qty int64) int64 {
	b.env.T.Helper()
	return num(b.call(http.MethodPut, loPath(lo, "item/insert"), tok,
		map[string]any{"usergear_registration_id": reg, "quantity": qty}, http.StatusCreated), "loadout_item_id")
}

func (b *board) move(tok string, lo, item, place int64) {
	b.env.T.Helper()
	b.call(http.MethodPost, itemPath(lo, item, "update"), tok, map[string]any{"place_id": place}, http.StatusOK)
}

func (b *board) tick(tok string, lo, item int64, ticked bool, want int) {
	b.env.T.Helper()
	w := b.env.Do(http.MethodPost, itemPath(lo, item, "tick"), tok, map[string]any{"ticked": ticked})
	expectStatus(b.env.T, w, want)
}

func (b *board) closePlace(tok string, lo, place int64, closed bool) {
	b.env.T.Helper()
	b.call(http.MethodPost, pPlacePath(lo, place, "close"), tok, map[string]any{"closed": closed}, http.StatusOK)
}

func (b *board) full(tok string, lo int64) models.LoadoutFull {
	b.env.T.Helper()
	w := b.env.Do(http.MethodGet, loPath(lo, "full"), tok, nil)
	expectStatus(b.env.T, w, http.StatusOK)
	var f models.LoadoutFull
	decodeJSON(b.env.T, w, &f)
	return f
}

func (b *board) summary(tok string, lo int64) models.Loadout {
	b.env.T.Helper()
	w := b.env.Do(http.MethodGet, "/api/v1/loadout/list", tok, nil)
	expectStatus(b.env.T, w, http.StatusOK)
	var all []models.Loadout
	decodeJSON(b.env.T, w, &all)
	for _, l := range all {
		if *l.LoadoutID == lo {
			return l
		}
	}
	b.env.T.Fatalf("loadout %d not in the list", lo)
	return models.Loadout{}
}

func (b *board) setCatalogWeight(gear int64, weight int) {
	b.env.T.Helper()
	b.call(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", gear), b.admin, map[string]any{"gear_weight": weight}, http.StatusOK)
}

func itemOf(t *testing.T, f models.LoadoutFull, id int64) models.LoadoutItem {
	t.Helper()
	for _, it := range f.Items {
		if *it.LoadoutItemID == id {
			return it
		}
	}
	t.Fatalf("item %d is not in the loadout", id)
	return models.LoadoutItem{}
}

func placeOf(f models.LoadoutFull, id int64) *models.LoadoutPlace {
	for i := range f.Places {
		if f.Places[i].PlaceID == id {
			return &f.Places[i]
		}
	}
	return nil
}

func wornOf(t *testing.T, f models.LoadoutFull) int64 {
	t.Helper()
	for _, p := range f.Places {
		if p.PlaceKind == "worn" {
			return p.PlaceID
		}
	}
	t.Fatal("no Worn place")
	return 0
}

// stored is the complete stored state of loadouts, places, items, registrations
// and closet links, read with SQL.
func (b *board) stored() string {
	b.env.T.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT quote(loadoutId)||'|'||quote(userId)||'|'||quote(loadoutName)||'|'||quote(updatedAt) FROM loadouts ORDER BY loadoutId`,
		`SELECT quote(placeId)||'|'||quote(loadoutId)||'|'||quote(placeKind)||'|'||quote(registrationId)||'|'||quote(parentPlaceId)||'|'||quote(position)||'|'||quote(closed) FROM loadout_places ORDER BY placeId`,
		`SELECT quote(loadoutItemId)||'|'||quote(loadoutId)||'|'||quote(registrationId)||'|'||quote(placeId)||'|'||quote(detachedReason)||'|'||quote(detachedGearId)||'|'||quote(detachedGearName)||'|'||quote(detachedUnitWeight)||'|'||quote(detachedWeightSource)||'|'||quote(quantity)||'|'||quote(notes)||'|'||quote(ticked) FROM loadout_items ORDER BY loadoutItemId`,
		`SELECT quote(userGearRegistrationId)||'|'||quote(gearId)||'|'||quote(userId) FROM user_gear_registrations ORDER BY userGearRegistrationId`,
		`SELECT quote(containerRegistrationId)||'|'||quote(userContainerId)||'|'||quote(userGearRegistrationId) FROM user_container_registration ORDER BY containerRegistrationId`,
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

// ---- GET /loadout/:loadout/full ---------------------------------------------

// Places are Worn, Last minute, then bags by position and place ID; items by
// item ID. The bags are added and ordered so that neither the place ID order
// nor the insert order equals the shown order.
func TestFull_PlaceAndItemOrderAndShape(t *testing.T) {
	b := newBoard(t)
	exos := b.alicesReg(b.gear("Exos 58", 1180, true))
	lid := b.alicesReg(b.gear("Lid", 90, true))
	pad := b.alicesReg(b.gear("Pad pouch", 40, true))
	spork := b.alicesReg(b.gear("Spork", 11, false))
	cap := b.alicesReg(b.gear("Cap", 0, false)) // no catalog weight

	lo := b.newLoadout(b.user, "Hardangervidda")
	bagExos, bagLid, bagPad := b.addBag(b.user, lo, exos), b.addBag(b.user, lo, lid), b.addBag(b.user, lo, pad)
	last := b.addLastMinute(b.user, lo)
	b.call(http.MethodPost, pPath(lo, "order"), b.user, map[string]any{"place_ids": []int64{bagPad, bagExos, bagLid}}, http.StatusOK)

	i1 := b.addItem(b.user, lo, spork, 2)
	i2 := b.addItem(b.user, lo, cap, 1)
	b.move(b.user, lo, i1, bagLid)

	f := b.full(b.user, lo)
	var kinds []string
	var bags []int64
	for _, p := range f.Places {
		kinds = append(kinds, p.PlaceKind)
		if p.PlaceKind == "bag" {
			bags = append(bags, p.PlaceID)
		}
	}
	if got := strings.Join(kinds, ","); got != "worn,last_minute,bag,bag,bag" {
		t.Errorf("place kinds = %s, want worn,last_minute,bag,bag,bag", got)
	}
	if fmt.Sprint(bags) != fmt.Sprint([]int64{bagPad, bagExos, bagLid}) {
		t.Errorf("bag order = %v, want %v (by position)", bags, []int64{bagPad, bagExos, bagLid})
	}
	if placeOf(f, last) == nil || placeOf(f, last).Closed {
		t.Errorf("Last minute = %+v", placeOf(f, last))
	}
	if g := placeOf(f, bagExos); g == nil || g.GearName == nil || *g.GearName != "Exos 58" || g.UnitWeight == nil || *g.UnitWeight != 1180 ||
		g.GearIsContainer == nil || !*g.GearIsContainer || g.UsergearRegistrationID == nil || *g.UsergearRegistrationID != exos {
		t.Errorf("bag Exos 58 = %+v", g)
	}
	if len(f.Items) != 2 || *f.Items[0].LoadoutItemID != i1 || *f.Items[1].LoadoutItemID != i2 {
		t.Fatalf("items = %+v, want %d then %d", f.Items, i1, i2)
	}
	if it := f.Items[0]; it.GearName != "Spork" || it.Quantity != 2 || it.PlaceID == nil || *it.PlaceID != bagLid ||
		it.UnitWeight == nil || *it.UnitWeight != 11 || it.WeightSource != "catalog" || !it.RegistrationExists {
		t.Errorf("spork item = %+v", it)
	}
	if it := f.Items[1]; it.PlaceID != nil || it.UnitWeight != nil || it.WeightSource != "none" {
		t.Errorf("cap item (no place, no weight) = %+v", it)
	}
	// Loadout part: the same summary as the list.
	if f.Loadout.TotalWeight != 1180+90+40+22 || f.Loadout.ItemCount != 2 || f.Loadout.BagCount != 3 {
		t.Errorf("loadout part = %+v", f.Loadout)
	}
}

func TestFull_RefusalsAreOwnerScoped(t *testing.T) {
	b := newBoard(t)
	lo := b.newLoadout(b.user, "mine")
	for name, c := range map[string]struct {
		tok  string
		path string
		want int
	}{
		"another user's loadout": {b.bTok, loPath(lo, "full"), http.StatusNotFound},
		"unknown loadout":        {b.user, loPath(lo+1000, "full"), http.StatusNotFound},
		"malformed id":           {b.user, "/api/v1/loadout/x/full", http.StatusBadRequest},
		"no token":               {"", loPath(lo, "full"), http.StatusUnauthorized},
	} {
		w := b.env.Do(http.MethodGet, c.path, c.tok, nil)
		if w.Code != c.want {
			t.Errorf("%s: status %d, want %d", name, w.Code, c.want)
		}
	}
	// The other user's refusal and an unknown loadout are the same answer (BR-018).
	a := b.env.Do(http.MethodGet, loPath(lo, "full"), b.bTok, nil).Body.String()
	u := b.env.Do(http.MethodGet, loPath(lo+1000, "full"), b.bTok, nil).Body.String()
	if a != u {
		t.Errorf("foreign and unknown loadouts differ: %q vs %q", a, u)
	}
}

// The three parts of /full are read in one transaction: while it runs, no
// other write can land between them (ARS-001 section 1).
func TestFull_ReadsInOneTransaction(t *testing.T) {
	b := newBoard(t)
	spork := b.alicesReg(b.gear("Spork", 11, false))
	lo := b.newLoadout(b.user, "tx")
	item := b.addItem(b.user, lo, spork, 1)

	// A second connection to the same file, with no wait for a lock.
	var file string
	var seq int
	var name string
	if err := b.env.DB.QueryRow(`SELECT seq, name, file FROM pragma_database_list WHERE name = 'main'`).Scan(&seq, &name, &file); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite3", "file:"+file+"?_busy_timeout=20")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })

	var writeErr error
	fired := false
	// The item query is the last statement of /full; it reads notes, which the
	// loadout and place queries do not.
	onFirstRead(t, "loadout_items", "notes", func() {
		fired = true
		_, writeErr = other.Exec(`UPDATE loadout_items SET ticked = 1 WHERE loadoutItemId = ?`, item)
	})
	f := b.full(b.user, lo)
	if !fired {
		t.Fatal("hook did not run")
	}
	if writeErr == nil {
		t.Fatal("a write landed while /full was reading: its parts do not share one transaction")
	}
	if itemOf(t, f, item).Ticked {
		t.Error("item reads as ticked from a write that should have been refused")
	}
}

// ---- Weights: BR-029 AC-2, AC-3, AC-4 ----------------------------------------

// BR-029 AC-3's example with catalog weights: Exos 58 (1 180 g bag) holding a
// stove (83 g, 1) and gas (360 g, 2), a rain shell in Worn (395 g) and a spork
// in no place (11 g) is 1 180 + 83 + 720 + 11 = 1 994 g. (The requirement's
// 360 g gas and 1 180 g bag are measured weights; measured weights are
// milestone B, so here they are catalog weights.)
func TestWeights_AC3Example1994AndRecomputeAfterCatalogEdit(t *testing.T) {
	b := newBoard(t)
	gExos := b.gear("Exos 58", 1180, true)
	gStove := b.gear("Stove", 83, false)
	gGas := b.gear("Gas", 360, false)
	gShell := b.gear("Rain shell", 395, false)
	gSpork := b.gear("Spork", 11, false)
	exos, stove, gas, shell, spork := b.alicesReg(gExos), b.alicesReg(gStove), b.alicesReg(gGas), b.alicesReg(gShell), b.alicesReg(gSpork)

	lo := b.newLoadout(b.user, "L1")
	bag := b.addBag(b.user, lo, exos)
	worn := wornOf(t, b.full(b.user, lo))
	iStove, iGas, iShell, iSpork := b.addItem(b.user, lo, stove, 1), b.addItem(b.user, lo, gas, 2), b.addItem(b.user, lo, shell, 1), b.addItem(b.user, lo, spork, 1)
	b.move(b.user, lo, iStove, bag)
	b.move(b.user, lo, iGas, bag)
	b.move(b.user, lo, iShell, worn)

	f := b.full(b.user, lo)
	if f.Loadout.TotalWeight != 1994 {
		t.Fatalf("full total_weight = %d, want 1994", f.Loadout.TotalWeight)
	}
	if got := b.summary(b.user, lo).TotalWeight; got != 1994 {
		t.Errorf("list total_weight = %d, want 1994", got)
	}
	// Every weight the board needs is present for each item and bag (AC-1).
	for _, c := range []struct {
		id   int64
		unit int64
	}{{iStove, 83}, {iGas, 360}, {iShell, 395}, {iSpork, 11}} {
		it := itemOf(t, f, c.id)
		if it.UnitWeight == nil || *it.UnitWeight != c.unit || it.CatalogWeight == nil || *it.CatalogWeight != c.unit || it.WeightSource != "catalog" {
			t.Errorf("item %d weights = %+v, want %d catalog", c.id, it, c.unit)
		}
	}
	if p := placeOf(f, bag); p == nil || p.UnitWeight == nil || *p.UnitWeight != 1180 || p.WeightSource == nil || *p.WeightSource != "catalog" {
		t.Errorf("bag weights = %+v", p)
	}

	// AC-2: an admin changes the stove's catalog weight; the next read of the
	// loadout and of the summary shows the new total with no other request.
	b.setCatalogWeight(gStove, 100)
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+17 {
		t.Errorf("full total after catalog edit = %d, want %d", got, 1994+17)
	}
	if got := b.summary(b.user, lo).TotalWeight; got != 1994+17 {
		t.Errorf("list total after catalog edit = %d, want %d", got, 1994+17)
	}
	// Moving the shell out of Worn adds its weight (it is on the back again).
	b.move(b.user, lo, iShell, bag)
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+17+395 {
		t.Errorf("total with the shell out of Worn = %d, want %d", got, 1994+17+395)
	}
}

// BR-029 AC-4: the list carries the counts and the closed places of every
// loadout, so the home page needs no item read.
func TestList_SummaryCountsAndClosedPlaces(t *testing.T) {
	b := newBoard(t)
	exos := b.alicesReg(b.gear("Exos 58", 1180, true))
	lid := b.alicesReg(b.gear("Lid", 90, true))
	spork := b.alicesReg(b.gear("Spork", 11, false))
	cup := b.alicesReg(b.gear("Cup", 30, false))

	empty := b.newLoadout(b.user, "empty")
	lo := b.newLoadout(b.user, "busy")
	bagExos, bagLid := b.addBag(b.user, lo, exos), b.addBag(b.user, lo, lid)
	last := b.addLastMinute(b.user, lo)
	worn := wornOf(t, b.full(b.user, lo))
	i1, i2 := b.addItem(b.user, lo, spork, 3), b.addItem(b.user, lo, cup, 1)
	b.tick(b.user, lo, i1, true, http.StatusOK)

	// Nothing is closed yet: [] and not null, for an empty loadout as well.
	raw := b.env.Do(http.MethodGet, "/api/v1/loadout/list", b.user, nil).Body.String()
	if strings.Contains(raw, `"closed_places": null`) || strings.Count(raw, `"closed_places": []`) != 2 {
		t.Errorf("closed_places must be [] for loadouts with no closed place:\n%s", raw)
	}
	if s := b.summary(b.user, empty); s.ItemCount != 0 || s.TickedCount != 0 || s.BagCount != 0 || len(s.ClosedPlaces) != 0 {
		t.Errorf("empty loadout summary = %+v", s)
	}

	// Close Last minute, Lid and Worn: the list names them in place order.
	for _, p := range []int64{bagLid, last, worn} {
		b.closePlace(b.user, lo, p, true)
	}
	s := b.summary(b.user, lo)
	if s.ItemCount != 2 || s.TickedCount != 1 || s.BagCount != 2 {
		t.Errorf("counts = items %d ticked %d bags %d, want 2/1/2", s.ItemCount, s.TickedCount, s.BagCount)
	}
	want := []models.ClosedPlace{{PlaceID: worn, PlaceKind: "worn", Name: "Worn"}, {PlaceID: last, PlaceKind: "last_minute", Name: "Last minute"}, {PlaceID: bagLid, PlaceKind: "bag", Name: "Lid"}}
	if fmt.Sprint(s.ClosedPlaces) != fmt.Sprint(want) {
		t.Errorf("closed_places = %v, want %v", s.ClosedPlaces, want)
	}
	if s.TotalWeight != 1180+90+3*11+30 {
		t.Errorf("total_weight = %d, want %d", s.TotalWeight, 1180+90+3*11+30)
	}

	// The same summary is on get and in /full; opening a place and unticking
	// are reflected without any item read; the other loadout is unaffected.
	b.closePlace(b.user, lo, bagLid, false)
	b.tick(b.user, lo, i1, false, http.StatusOK)
	g := b.call(http.MethodGet, loPath(lo, "get"), b.user, nil, http.StatusOK)
	if num(g, "ticked_count") != 0 || num(g, "bag_count") != 2 || num(g, "item_count") != 2 || len(g["closed_places"].([]any)) != 2 {
		t.Errorf("get summary = %v", g)
	}
	fullPart, _ := json.Marshal(b.full(b.user, lo).Loadout)
	listPart, _ := json.Marshal(b.summary(b.user, lo))
	if string(fullPart) != string(listPart) {
		t.Errorf("full.loadout differs from the list entry:\n%s\n%s", fullPart, listPart)
	}
	if s := b.summary(b.user, lo); s.ItemCount != 2 {
		t.Errorf("item_count counts rows, not quantities: %d, want 2 (%d, %d, %d)", s.ItemCount, i1, i2, bagExos)
	}
}

// ---- BR-025: progress shared between sessions --------------------------------

// SC-002: on the phone the owner closes Lid and ticks three items in Exos 58;
// on the laptop Lid is closed and the three items are ticked. A tick queued on
// the phone for an item removed meanwhile is refused for that item only.
func TestProgress_TwoSessionsAndStaleTick(t *testing.T) {
	b := newBoard(t)
	exos := b.alicesReg(b.gear("Exos 58", 1180, true))
	lid := b.alicesReg(b.gear("Lid", 90, true))
	b.closetLink(b.user, exos, lid)
	var regs [5]int64
	for i := range regs {
		regs[i] = b.alicesReg(b.gear(fmt.Sprintf("Item %d", i), 10+i, false))
	}
	lo := b.newLoadout(b.user, "Hardangervidda")
	bagExos := b.addBag(b.user, lo, exos)
	bagLid := b.addBag(b.user, lo, lid)
	if p := placeOf(b.full(b.user, lo), bagLid); p == nil || p.ParentPlaceID == nil || *p.ParentPlaceID != bagExos {
		t.Fatalf("Lid starts inside Exos 58 as in the closet: %+v", p)
	}
	var items [5]int64
	for i := range items {
		items[i] = b.addItem(b.user, lo, regs[i], 1)
		b.move(b.user, lo, items[i], bagExos)
	}

	phone := b.user
	laptop := b.env.SignToken(fmt.Sprint(b.userID), b.env.Auth.JWTAudience, time.Now().Add(2*time.Hour))
	if laptop == phone {
		t.Fatal("the two sessions must hold different tokens")
	}

	// Session 1 (phone): tick three items, close Lid.
	for _, i := range items[:3] {
		b.tick(phone, lo, i, true, http.StatusOK)
	}
	b.closePlace(phone, lo, bagLid, true)

	// Session 2 (laptop) reads the same progress, in /full and in the list.
	f := b.full(laptop, lo)
	for n, i := range items {
		if got, want := itemOf(t, f, i).Ticked, n < 3; got != want {
			t.Errorf("laptop: item %d ticked = %v, want %v", n, got, want)
		}
	}
	if p := placeOf(f, bagLid); !p.Closed || placeOf(f, bagExos).Closed {
		t.Errorf("laptop: Lid closed = %v, Exos closed = %v, want true, false", p.Closed, placeOf(f, bagExos).Closed)
	}
	if s := b.summary(laptop, lo); s.TickedCount != 3 || len(s.ClosedPlaces) != 1 || s.ClosedPlaces[0].Name != "Lid" {
		t.Errorf("laptop summary = %+v", s)
	}

	// The laptop removes item 3; the phone, offline, still has ticks queued for
	// items 3 and 4. Only item 3's tick is refused (404) and nothing else changes.
	b.call(http.MethodDelete, itemPath(lo, items[3], "delete"), laptop, nil, http.StatusOK)
	before := b.stored()
	b.tick(phone, lo, items[3], true, http.StatusNotFound)
	if b.stored() != before {
		t.Error("the refused tick changed stored data")
	}
	b.tick(phone, lo, items[4], true, http.StatusOK)
	f = b.full(laptop, lo)
	if len(f.Items) != 4 || !itemOf(t, f, items[4]).Ticked {
		t.Errorf("after the queue: %d items, item 4 ticked = %v", len(f.Items), itemOf(t, f, items[4]).Ticked)
	}

	// Reopening Lid keeps every tick (BR-025 AC-3), and a tick for a loadout
	// that is gone is refused as well.
	b.closePlace(laptop, lo, bagLid, false)
	if s := b.summary(phone, lo); s.TickedCount != 4 || len(s.ClosedPlaces) != 0 {
		t.Errorf("after reopening: %+v", s)
	}
	b.call(http.MethodDelete, loPath(lo, "delete"), laptop, nil, http.StatusOK)
	b.tick(phone, lo, items[0], false, http.StatusNotFound)
}

// ---- ARD-001 "Normal example" -------------------------------------------------

// The owner makes Exos 58 and Lid bags of a loadout (Lid starts inside Exos, as
// in the closet), sorts 24 items into the bags, Worn and Last minute, ticks five
// on the phone and closes Lid, and the laptop's /full shows the same, with the
// total from the weights. (The 10 kg goal belongs to milestone B.)
func TestNormalExample_SortTickCloseAndReadOnAnotherDevice(t *testing.T) {
	b := newBoard(t)
	exos := b.alicesReg(b.gear("Exos 58", 1180, true))
	lid := b.alicesReg(b.gear("Lid", 90, true))
	b.closetLink(b.user, exos, lid)

	const n = 24
	var regs, items [n]int64
	var weights [n]int64
	for i := range regs {
		weights[i] = int64(10 + i)
		regs[i] = b.alicesReg(b.gear(fmt.Sprintf("Thing %02d", i), 10+i, false))
	}

	lo := b.newLoadout(b.user, "Hardangervidda")
	bagExos, bagLid := b.addBag(b.user, lo, exos), b.addBag(b.user, lo, lid)
	last := b.addLastMinute(b.user, lo)
	worn := wornOf(t, b.full(b.user, lo))
	if p := placeOf(b.full(b.user, lo), bagLid); p.ParentPlaceID == nil || *p.ParentPlaceID != bagExos {
		t.Fatalf("Lid must start inside Exos 58: %+v", p)
	}

	for i := range items {
		items[i] = b.addItem(b.user, lo, regs[i], 1)
	}
	// Sort: 0-9 Exos, 10-15 Lid, 16-19 Worn, 20-21 Last minute, 22-23 no place.
	placeFor := func(i int) int64 {
		switch {
		case i < 10:
			return bagExos
		case i < 16:
			return bagLid
		case i < 20:
			return worn
		case i < 22:
			return last
		}
		return 0
	}
	var onBack int64 = 1180 + 90
	for i := range items {
		if p := placeFor(i); p != 0 {
			b.move(b.user, lo, items[i], p)
		}
		if placeFor(i) != worn {
			onBack += weights[i]
		}
	}

	// On the phone: tick five items, close Lid.
	for _, i := range items[:5] {
		b.tick(b.user, lo, i, true, http.StatusOK)
	}
	b.closePlace(b.user, lo, bagLid, true)

	// On the laptop.
	laptop := b.env.SignToken(fmt.Sprint(b.userID), b.env.Auth.JWTAudience, time.Now().Add(2*time.Hour))
	f := b.full(laptop, lo)
	if len(f.Items) != n {
		t.Fatalf("laptop sees %d items, want %d", len(f.Items), n)
	}
	for i, id := range items {
		it := itemOf(t, f, id)
		if p := placeFor(i); (p == 0 && it.PlaceID != nil) || (p != 0 && (it.PlaceID == nil || *it.PlaceID != p)) {
			t.Errorf("item %d place = %v, want %d", i, it.PlaceID, p)
		}
		if it.Ticked != (i < 5) {
			t.Errorf("item %d ticked = %v", i, it.Ticked)
		}
	}
	if !placeOf(f, bagLid).Closed || placeOf(f, bagExos).Closed || placeOf(f, last).Closed || placeOf(f, worn).Closed {
		t.Error("only Lid must be closed")
	}
	if f.Loadout.TotalWeight != onBack {
		t.Errorf("total_weight = %d, want %d (bags once, Worn left out)", f.Loadout.TotalWeight, onBack)
	}
	s := b.summary(laptop, lo)
	if s.ItemCount != n || s.TickedCount != 5 || s.BagCount != 2 || s.TotalWeight != onBack || len(s.ClosedPlaces) != 1 || s.ClosedPlaces[0].PlaceID != bagLid {
		t.Errorf("home summary = %+v", s)
	}
	// A tick sent later for an item removed meanwhile is refused for that item only.
	b.call(http.MethodDelete, itemPath(lo, items[7], "delete"), laptop, nil, http.StatusOK)
	b.tick(b.user, lo, items[7], true, http.StatusNotFound)
	b.tick(b.user, lo, items[8], true, http.StatusOK)
}

// ---- BR-030: deleting a registration keeps the items -------------------------

func TestDeleteRegistration_KeepsItemsAndUndoesBagsTogether(t *testing.T) {
	b := newBoard(t)
	gExos := b.gear("Exos 58", 1180, true)
	exos := b.alicesReg(gExos)
	lid := b.alicesReg(b.gear("Lid", 90, true))
	duffel := b.alicesReg(b.gear("Duffel", 500, true))
	stove := b.alicesReg(b.gear("Stove", 83, false))
	pad := b.alicesReg(b.gear("Pad", 40, false))
	b.closetLink(b.user, exos, lid)

	// L2: Exos 58 is a bag holding the stove and the nested bag Lid, which
	// holds a ticked pad. L1: Exos 58 itself is an item (ticked, noted) in Duffel.
	l2 := b.newLoadout(b.user, "L2")
	bagExos, bagLid := b.addBag(b.user, l2, exos), b.addBag(b.user, l2, lid)
	iStove, iPad := b.addItem(b.user, l2, stove, 1), b.addItem(b.user, l2, pad, 2)
	b.move(b.user, l2, iStove, bagExos)
	b.move(b.user, l2, iPad, bagLid)
	b.tick(b.user, l2, iPad, true, http.StatusOK)
	b.tick(b.user, l2, iStove, true, http.StatusOK)
	b.closePlace(b.user, l2, bagExos, true)

	l1 := b.newLoadout(b.user, "L1")
	bagDuffel := b.addBag(b.user, l1, duffel)
	iExos := num(b.call(http.MethodPut, loPath(l1, "item/insert"), b.user,
		map[string]any{"usergear_registration_id": exos, "quantity": 2, "notes": "borrowed", "place_id": bagDuffel}, http.StatusCreated), "loadout_item_id")
	b.tick(b.user, l1, iExos, true, http.StatusOK)

	// Bob has the same gear, a loadout and a bag of his own (AC-5).
	bobExos := b.register(b.bTok, b.bob, gExos)
	bl := b.newLoadout(b.bTok, "bob")
	bobBag := b.addBag(b.bTok, bl, bobExos)
	bobItem := b.addItem(b.bTok, bl, bobExos, 1)
	b.tick(b.bTok, bl, bobItem, true, http.StatusOK)
	b.closePlace(b.bTok, bl, bobBag, true)
	bobBefore, _ := json.Marshal(b.full(b.bTok, bl))
	catalogBefore := b.env.catalogSnapshot()

	beforeL1, beforeL2 := b.full(b.user, l1), b.full(b.user, l2)
	if beforeL2.Loadout.TotalWeight != 1180+90+83+2*40 {
		t.Fatalf("setup total L2 = %d, want %d", beforeL2.Loadout.TotalWeight, 1180+90+83+2*40)
	}
	storedBefore := b.stored()

	// AC-4: a failure while deleting leaves every loadout unchanged.
	if _, err := b.env.DB.Exec(`CREATE TRIGGER fail_reg_delete_late AFTER DELETE ON user_gear_registrations BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	w := b.env.Do(http.MethodDelete, ugPath(exos, "delete"), b.user, nil)
	if w.Code < 500 {
		t.Fatalf("injected failure: status %d, want 5xx", w.Code)
	}
	if got := b.stored(); got != storedBefore {
		t.Fatalf("a failed deletion changed stored data:\nbefore:\n%s\nafter:\n%s", storedBefore, got)
	}
	if _, err := b.env.DB.Exec(`DROP TRIGGER fail_reg_delete_late`); err != nil {
		t.Fatal(err)
	}

	// The real deletion.
	expectStatus(t, b.env.Do(http.MethodDelete, ugPath(exos, "delete"), b.user, nil), http.StatusOK)
	if b.env.Count("user_gear_registrations", "userGearRegistrationId = ?", exos) != 0 {
		t.Fatal("registration not deleted")
	}

	// AC-1: L1's item stays, as a snapshot that still counts.
	f1 := b.full(b.user, l1)
	it := itemOf(t, f1, iExos)
	if it.RegistrationExists || it.UsergearRegistrationID != nil || it.DetachedReason == nil || *it.DetachedReason != "registration_deleted" {
		t.Errorf("L1 item should be detached: %+v", it)
	}
	if it.GearName != "Exos 58" || it.UnitWeight == nil || *it.UnitWeight != 1180 || it.WeightSource != "catalog" || it.CatalogWeight != nil ||
		it.Quantity != 2 || it.Notes != "borrowed" || !it.Ticked || it.PlaceID == nil || *it.PlaceID != bagDuffel {
		t.Errorf("L1 snapshot item = %+v", it)
	}
	if f1.Loadout.TotalWeight != beforeL1.Loadout.TotalWeight || f1.Loadout.TotalWeight != 500+2*1180 {
		t.Errorf("L1 total = %d, want %d unchanged", f1.Loadout.TotalWeight, beforeL1.Loadout.TotalWeight)
	}

	// AC-3: in L2 Exos 58 is no longer a bag; its item has no place, the nested
	// bag is top level, ticks stay, and nothing is lost.
	f2 := b.full(b.user, l2)
	if placeOf(f2, bagExos) != nil {
		t.Error("L2 still has the Exos 58 bag")
	}
	if p := placeOf(f2, bagLid); p == nil || p.ParentPlaceID != nil || p.Closed {
		t.Errorf("L2 Lid = %+v, want a top-level open bag", p)
	}
	if s := itemOf(t, f2, iStove); s.PlaceID != nil || !s.Ticked || s.Quantity != 1 {
		t.Errorf("L2 stove = %+v, want no place, still ticked", s)
	}
	if p := itemOf(t, f2, iPad); p.PlaceID == nil || *p.PlaceID != bagLid || !p.Ticked || p.Quantity != 2 {
		t.Errorf("L2 pad (in the nested bag) = %+v", p)
	}
	if len(f2.Items) != len(beforeL2.Items) {
		t.Errorf("L2 has %d items, want %d", len(f2.Items), len(beforeL2.Items))
	}
	// L2 keeps its total apart from the deleted bag's own weight and the stove's
	// place no longer matters (it was not in Worn).
	if f2.Loadout.TotalWeight != 90+83+2*40 {
		t.Errorf("L2 total = %d, want %d (Exos 58 is no longer a bag)", f2.Loadout.TotalWeight, 90+83+2*40)
	}

	// AC-5: nothing of Bob's and nothing in the catalog changed.
	if got, _ := json.Marshal(b.full(b.bTok, bl)); string(got) != string(bobBefore) {
		t.Errorf("bob's loadout changed:\nbefore %s\nafter  %s", bobBefore, got)
	}
	if b.env.catalogSnapshot() != catalogBefore {
		t.Error("the catalog changed")
	}

	// AC-2: the kept item is changed, moved, ticked and removed like any other.
	b.call(http.MethodPost, itemPath(l1, iExos, "update"), b.user, map[string]any{"quantity": 3, "notes": "kept", "place_id": nil}, http.StatusOK)
	b.tick(b.user, l1, iExos, false, http.StatusOK)
	it = itemOf(t, b.full(b.user, l1), iExos)
	if it.Quantity != 3 || it.Notes != "kept" || it.PlaceID != nil || it.Ticked || it.GearName != "Exos 58" {
		t.Errorf("edited snapshot item = %+v", it)
	}
	b.call(http.MethodDelete, itemPath(l1, iExos, "delete"), b.user, nil, http.StatusOK)
	if len(b.full(b.user, l1).Items) != 0 {
		t.Error("the snapshot item was not removed")
	}
}
