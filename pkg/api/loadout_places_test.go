package endpoints

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Place endpoints of ARS-001 milestone A (BR-024, BR-025, BR-030 AC-3, BR-018
// AC-5, BR-015 AC-7). Everything goes through the real route table, JWT
// middleware and migrated database; stored state is read back with SQL.

type placeFixture struct {
	env        *authzEnv
	alice, bob int64
	aTok, bTok string
	lo, loBob  int64
	loOther    int64 // a second loadout of alice
	worn       int64
	// alice's registrations
	exos, lid, pad, tent int64
	exosGear             int64
	exosCategory         int64
	// bob's registration of a container and his place
	bobPack  int64
	bobBag   int64 // a bag of loBob
	otherBag int64 // a bag of loOther (alice's other loadout)
}

func newPlaceFixture(t *testing.T) *placeFixture {
	env := newAuthzEnv(t)
	f := &placeFixture{env: env}
	f.alice = env.SeedUser("alice", "alice@example.com", false)
	f.bob = env.SeedUser("bob", "bob@example.com", false)
	f.aTok, f.bTok = env.Token(f.alice, false), env.Token(f.bob, false)
	top := env.seedTopCategory("Packs")
	f.exosGear = env.seedGearX(true, false, 0, top)
	lidGear := env.seedGearX(true, false, 0, top)
	padGear := env.seedGearX(true, false, 0, top)
	tentGear := env.seedGearW(1000)
	if err := env.DB.QueryRow(`SELECT gearCategoryId FROM gear WHERE gearId = ?`, f.exosGear).Scan(&f.exosCategory); err != nil {
		t.Fatal(err)
	}
	f.exos = env.SeedRegistration(f.alice, f.exosGear)
	f.lid = env.SeedRegistration(f.alice, lidGear)
	f.pad = env.SeedRegistration(f.alice, padGear)
	f.tent = env.SeedRegistration(f.alice, tentGear)
	f.bobPack = env.SeedRegistration(f.bob, f.exosGear)
	f.lo = env.seedLO(f.alice, false, "alice-lo")
	f.loOther = env.seedLO(f.alice, false, "alice-other")
	f.loBob = env.seedLO(f.bob, false, "bob-lo")
	f.worn = f.placeOfKind(f.lo, "worn")
	f.otherBag = f.seedBag(f.loOther, env.SeedRegistration(f.alice, f.exosGear), 1)
	f.bobBag = f.seedBag(f.loBob, f.bobPack, 1)
	return f
}

func (f *placeFixture) placeOfKind(loadout int64, kind string) int64 {
	f.env.T.Helper()
	var id int64
	if err := f.env.DB.QueryRow(`SELECT placeId FROM loadout_places WHERE loadoutId = ? AND placeKind = ?`, loadout, kind).Scan(&id); err != nil {
		f.env.T.Fatal(err)
	}
	return id
}

func (f *placeFixture) seedBag(loadout, registration, position int64) int64 {
	f.env.T.Helper()
	res, err := f.env.DB.Exec(`INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (?, 'bag', ?, ?)`,
		loadout, registration, position)
	if err != nil {
		f.env.T.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func pPath(loadout int64, op string) string {
	return fmt.Sprintf("/api/v1/loadout/%d/place/%s", loadout, op)
}
func pPlacePath(loadout, place int64, op string) string {
	return fmt.Sprintf("/api/v1/loadout/%d/place/%d/%s", loadout, place, op)
}

// do sends a request as alice and returns the status and raw body.
func (f *placeFixture) do(method, path string, body any) (int, string) {
	f.env.T.Helper()
	w := f.env.Do(method, path, f.aTok, body)
	return w.Code, w.Body.String()
}

// addBag adds a bag through the API and returns its place ID.
func (f *placeFixture) addBag(registration int64, extra map[string]any) int64 {
	f.env.T.Helper()
	body := map[string]any{"place_kind": "bag", "usergear_registration_id": registration}
	for k, v := range extra {
		body[k] = v
	}
	w := f.env.Do(http.MethodPut, pPath(f.lo, "insert"), f.aTok, body)
	expectStatus(f.env.T, w, http.StatusCreated)
	var p models.LoadoutPlace
	decodeJSON(f.env.T, w, &p)
	return p.PlaceID
}

func (f *placeFixture) places(loadout int64) []models.LoadoutPlace {
	f.env.T.Helper()
	ps, err := LoadoutPlaces(context.Background(), f.env.DB, loadout)
	if err != nil {
		f.env.T.Fatal(err)
	}
	return ps
}

func (f *placeFixture) place(loadout, id int64) *models.LoadoutPlace {
	f.env.T.Helper()
	for _, p := range f.places(loadout) {
		if p.PlaceID == id {
			return &p
		}
	}
	return nil
}

func (f *placeFixture) parentOf(id int64) *int64 {
	f.env.T.Helper()
	p := f.place(f.lo, id)
	if p == nil {
		f.env.T.Fatalf("place %d is gone", id)
	}
	return p.ParentPlaceID
}

func (f *placeFixture) bagOrder(loadout int64) []int64 {
	f.env.T.Helper()
	var ids []int64
	for _, p := range f.places(loadout) {
		if p.PlaceKind == "bag" {
			ids = append(ids, p.PlaceID)
		}
	}
	return ids
}

// stored is the complete stored state of every place and item.
func (f *placeFixture) stored() string {
	f.env.T.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT quote(placeId)||'|'||quote(loadoutId)||'|'||quote(placeKind)||'|'||quote(registrationId)||'|'||quote(parentPlaceId)||'|'||quote(position)||'|'||quote(closed) FROM loadout_places ORDER BY placeId`,
		`SELECT quote(loadoutItemId)||'|'||quote(loadoutId)||'|'||quote(registrationId)||'|'||quote(placeId)||'|'||quote(ticked)||'|'||quote(quantity) FROM loadout_items ORDER BY loadoutItemId`,
		`SELECT quote(containerRegistrationId)||'|'||quote(userContainerId)||'|'||quote(userGearRegistrationId) FROM user_container_registration ORDER BY containerRegistrationId`,
	} {
		rows, err := f.env.DB.Query(q)
		if err != nil {
			f.env.T.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				f.env.T.Fatal(err)
			}
			sb.WriteString(s + "\n")
		}
		rows.Close()
		sb.WriteString("--\n")
	}
	return sb.String()
}

func (f *placeFixture) refused(method, path string, body any, want int) string {
	f.env.T.Helper()
	before := f.stored()
	code, resp := f.do(method, path, body)
	if code != want {
		f.env.T.Fatalf("%s %s %v: status %d, want %d (%s)", method, path, body, code, want, resp)
	}
	if after := f.stored(); after != before {
		f.env.T.Fatalf("%s %s %v changed stored state:\nbefore:\n%s\nafter:\n%s", method, path, body, before, after)
	}
	return resp
}

// ---- Worn and Last minute (BR-024 AC-1) ------------------------------------

func TestPlace_NewLoadoutHasWornAndNoLastMinute(t *testing.T) {
	f := newPlaceFixture(t)
	ps := f.places(f.lo)
	if len(ps) != 1 || ps[0].PlaceKind != "worn" || ps[0].Closed {
		t.Fatalf("places of a new loadout = %+v, want one open Worn", ps)
	}
	if ps[0].UsergearRegistrationID != nil || ps[0].GearName != nil || ps[0].ParentPlaceID != nil || ps[0].Position != nil {
		t.Errorf("Worn has bag-only fields: %+v", ps[0])
	}
	// A loadout created through the API gets Worn too (trigger).
	w := f.env.Do(http.MethodPut, "/api/v1/loadout/insert", f.aTok, map[string]any{"loadout_name": "new"})
	expectStatus(t, w, http.StatusCreated)
	var l models.Loadout
	decodeJSON(t, w, &l)
	if got := f.places(*l.LoadoutID); len(got) != 1 || got[0].PlaceKind != "worn" {
		t.Errorf("API-created loadout places = %+v, want Worn only", got)
	}
}

func TestPlace_LastMinuteOneOnlyAndRemovable(t *testing.T) {
	f := newPlaceFixture(t)
	code, body := f.do(http.MethodPut, pPath(f.lo, "insert"), map[string]any{"place_kind": "last_minute"})
	if code != http.StatusCreated {
		t.Fatalf("add Last minute: %d %s", code, body)
	}
	var lm models.LoadoutPlace
	if err := json.Unmarshal([]byte(body), &lm); err != nil {
		t.Fatal(err)
	}
	if lm.PlaceKind != "last_minute" || lm.Closed || lm.LoadoutID != f.lo || lm.UsergearRegistrationID != nil {
		t.Errorf("Last minute = %+v", lm)
	}
	f.refused(http.MethodPut, pPath(f.lo, "insert"), map[string]any{"place_kind": "last_minute"}, http.StatusConflict)

	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, lm.PlaceID, "delete"), nil); code != http.StatusOK {
		t.Fatalf("remove Last minute: %d %s", code, body)
	}
	if code, body := f.do(http.MethodPut, pPath(f.lo, "insert"), map[string]any{"place_kind": "last_minute"}); code != http.StatusCreated {
		t.Fatalf("add Last minute again: %d %s", code, body)
	}
}

func TestPlace_WornCannotBeRemoved(t *testing.T) {
	f := newPlaceFixture(t)
	f.refused(http.MethodDelete, pPlacePath(f.lo, f.worn, "delete"), nil, http.StatusBadRequest)
	if f.place(f.lo, f.worn) == nil {
		t.Fatal("Worn is gone")
	}
}

// ---- Adding bags (BR-024 AC-2, AC-6, AC-9) ---------------------------------

func TestPlaceInsert_BagReturnsContractFieldsAndGoesLast(t *testing.T) {
	f := newPlaceFixture(t)
	if _, err := f.env.DB.Exec(`UPDATE user_gear_registrations SET maxContainerWeight = 3000 WHERE userGearRegistrationId = ?`, f.exos); err != nil {
		t.Fatal(err)
	}
	w := f.env.Do(http.MethodPut, pPath(f.lo, "insert"), f.aTok,
		map[string]any{"place_kind": "bag", "usergear_registration_id": f.exos})
	expectStatus(t, w, http.StatusCreated)
	var p models.LoadoutPlace
	decodeJSON(t, w, &p)
	if p.LoadoutID != f.lo || p.PlaceKind != "bag" || p.Closed ||
		p.UsergearRegistrationID == nil || *p.UsergearRegistrationID != f.exos ||
		p.GearID == nil || *p.GearID != f.exosGear || p.GearName == nil ||
		p.GearIsContainer == nil || !*p.GearIsContainer ||
		p.MaxContainerWeight == nil || *p.MaxContainerWeight != 3000 ||
		p.CatalogWeight == nil || *p.CatalogWeight != 100 ||
		p.UnitWeight == nil || *p.UnitWeight != 100 ||
		p.WeightSource == nil || *p.WeightSource != "catalog" ||
		p.ParentPlaceID != nil || p.Position == nil || *p.Position != 1 {
		t.Errorf("bag = %+v", p)
	}
	lid := f.addBag(f.lid, nil)
	pad := f.addBag(f.pad, nil)
	if got := f.bagOrder(f.lo); len(got) != 3 || got[0] != p.PlaceID || got[1] != lid || got[2] != pad {
		t.Errorf("bag order = %v, want [%d %d %d]", got, p.PlaceID, lid, pad)
	}
	// The bag's own weight counts on your back (BR-029 AC-3).
	if got := f.env.weight(f.lo); got != 300 {
		t.Errorf("on-back total = %d, want 300", got)
	}
	// The place list is Worn first, then bags.
	if ps := f.places(f.lo); ps[0].PlaceKind != "worn" || len(ps) != 4 {
		t.Errorf("places = %+v", ps)
	}
}

func TestPlaceInsert_BagRefusals(t *testing.T) {
	f := newPlaceFixture(t)
	f.addBag(f.exos, nil)

	ins := func(body any, want int) string {
		return f.refused(http.MethodPut, pPath(f.lo, "insert"), body, want)
	}
	bag := func(reg int64) map[string]any {
		return map[string]any{"place_kind": "bag", "usergear_registration_id": reg}
	}
	// 404: unknown and foreign registration, identical bodies.
	foreign := ins(bag(f.bobPack), http.StatusNotFound)
	unknown := ins(bag(999999), http.StatusNotFound)
	if foreign != unknown {
		t.Errorf("foreign and unknown registration bodies differ: %q vs %q", foreign, unknown)
	}
	// 409: not container-capable, already a bag.
	if body := ins(bag(f.tent), http.StatusConflict); !strings.Contains(body, "cannot hold items") {
		t.Errorf("non-capable body = %s, want it to say the gear cannot hold items", body)
	}
	ins(bag(f.exos), http.StatusConflict)
	// 400: kind, missing registration, malformed.
	ins(map[string]any{"place_kind": "worn"}, http.StatusBadRequest)
	ins(map[string]any{"place_kind": "shelf", "usergear_registration_id": f.lid}, http.StatusBadRequest)
	ins(map[string]any{"usergear_registration_id": f.lid}, http.StatusBadRequest)
	ins(map[string]any{"place_kind": "bag"}, http.StatusBadRequest)
	ins(map[string]any{"place_kind": "bag", "usergear_registration_id": f.lid, "parent_place_id": "x"}, http.StatusBadRequest)
	ins("{not json", http.StatusBadRequest)
}

func TestPlaceInsert_AnotherLoadoutOfTheSameOwnerMayUseTheRegistration(t *testing.T) {
	f := newPlaceFixture(t)
	f.addBag(f.exos, nil)
	// A registration is a bag of at most one place per loadout, not once overall.
	if _, err := f.env.DB.Exec(`INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (?, 'bag', ?, 1)`, f.loOther, f.exos); err != nil {
		t.Fatal(err)
	}
}

func TestPlaceInsert_ParentRules(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lm := f.addLastMinute()
	ins := func(parent any, want int) string {
		return f.refused(http.MethodPut, pPath(f.lo, "insert"),
			map[string]any{"place_kind": "bag", "usergear_registration_id": f.lid, "parent_place_id": parent}, want)
	}
	// 400: Worn and Last minute are not bags (BR-024 AC-6).
	ins(f.worn, http.StatusBadRequest)
	ins(lm, http.StatusBadRequest)
	// 404: another loadout's place, another user's, unknown: identical bodies.
	a := ins(f.otherBag, http.StatusNotFound)
	b := ins(f.bobBag, http.StatusNotFound)
	c := ins(999999, http.StatusNotFound)
	if a != b || b != c {
		t.Errorf("parent 404 bodies differ: %q %q %q", a, b, c)
	}
	// 404 before 400 before 409: a Last minute parent with an unknown registration is 404.
	f.refused(http.MethodPut, pPath(f.lo, "insert"),
		map[string]any{"place_kind": "bag", "usergear_registration_id": 999999, "parent_place_id": f.worn}, http.StatusNotFound)
	// 409 comes after 400: a non-capable registration with a Worn parent is 400.
	f.refused(http.MethodPut, pPath(f.lo, "insert"),
		map[string]any{"place_kind": "bag", "usergear_registration_id": f.tent, "parent_place_id": f.worn}, http.StatusBadRequest)

	// A valid explicit parent nests the bag.
	lid := f.addBag(f.lid, map[string]any{"parent_place_id": exos})
	if p := f.parentOf(lid); p == nil || *p != exos {
		t.Errorf("parent of Lid = %v, want %d", p, exos)
	}

	// 409: a parent whose gear lost capability (BR-024 AC-6).
	f.env.setCategoryContainer(f.exosCategory, boolPtr(false))
	f.refused(http.MethodPut, pPath(f.lo, "insert"),
		map[string]any{"place_kind": "bag", "usergear_registration_id": f.pad, "parent_place_id": exos}, http.StatusConflict)
}

func (f *placeFixture) addLastMinute() int64 {
	f.env.T.Helper()
	w := f.env.Do(http.MethodPut, pPath(f.lo, "insert"), f.aTok, map[string]any{"place_kind": "last_minute"})
	expectStatus(f.env.T, w, http.StatusCreated)
	var p models.LoadoutPlace
	decodeJSON(f.env.T, w, &p)
	return p.PlaceID
}

// ---- Closet default parent (BR-024 AC-4, ARS-001 F5) -----------------------

func TestPlaceInsert_DefaultParentIsTheClosetContainerWhenItIsABag(t *testing.T) {
	f := newPlaceFixture(t)
	f.env.SeedContainerLink(f.exos, f.lid) // in the closet, Lid is inside Exos

	// Exos is not a bag yet: Lid starts top level.
	lidAlone := f.addBag(f.lid, nil)
	if p := f.parentOf(lidAlone); p != nil {
		t.Fatalf("parent of Lid before Exos is a bag = %d, want top level", *p)
	}
	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, lidAlone, "delete"), nil); code != http.StatusOK {
		t.Fatal(code, body)
	}

	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil) // absent parent_place_id: closet default
	if p := f.parentOf(lid); p == nil || *p != exos {
		t.Fatalf("parent of Lid = %v, want the Exos bag %d", p, exos)
	}

	// Explicit null: top level, whatever the closet says.
	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, lid, "delete"), nil); code != http.StatusOK {
		t.Fatal(code, body)
	}
	lid = f.addBag(f.lid, map[string]any{"parent_place_id": nil})
	if p := f.parentOf(lid); p != nil {
		t.Errorf("explicit null parent = %d, want top level", *p)
	}

	// The closet link is as it was (no link written, changed or removed).
	if n := f.env.Count("user_container_registration", "userContainerId = ? AND userGearRegistrationId = ?", f.exos, f.lid); n != 1 {
		t.Errorf("closet link Exos > Lid count = %d, want 1", n)
	}
	if n := f.env.Count("user_container_registration", "1 = 1"); n != 1 {
		t.Errorf("closet link rows = %d, want 1", n)
	}
}

func TestPlaceInsert_DefaultParentInAnotherLoadoutDoesNotCount(t *testing.T) {
	f := newPlaceFixture(t)
	f.env.SeedContainerLink(f.exos, f.lid)
	// Exos is a bag of the other loadout only.
	f.seedBag(f.loOther, f.exos, 2)
	lid := f.addBag(f.lid, nil)
	if p := f.parentOf(lid); p != nil {
		t.Errorf("parent = %d, want top level", *p)
	}
}

func TestPlaceInsert_DefaultParentThatLostCapabilityIsSkippedNotRefused(t *testing.T) {
	f := newPlaceFixture(t)
	f.env.SeedContainerLink(f.exos, f.lid)
	exos := f.addBag(f.exos, nil)
	f.env.setCategoryContainer(f.exosCategory, boolPtr(false)) // Exos can no longer hold items

	lid := f.addBag(f.lid, nil) // 201, top level
	if p := f.parentOf(lid); p != nil {
		t.Errorf("parent = %d, want top level (default parent skipped)", *p)
	}
	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, lid, "delete"), nil); code != http.StatusOK {
		t.Fatal(code, body)
	}
	// Naming it explicitly is still 409.
	f.refused(http.MethodPut, pPath(f.lo, "insert"),
		map[string]any{"place_kind": "bag", "usergear_registration_id": f.lid, "parent_place_id": exos}, http.StatusConflict)
}

// ---- Changing the parent (BR-024 AC-3, AC-4, AC-6, AC-7) -------------------

func TestPlaceUpdate_NestingCycleSelfAndTargets(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	pad := f.addBag(f.pad, nil)
	up := func(place int64, parent any, want int) string {
		return f.refused(http.MethodPost, pPlacePath(f.lo, place, "update"), map[string]any{"parent_place_id": parent}, want)
	}

	// Lid into Exos.
	code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": exos})
	if code != http.StatusOK {
		t.Fatalf("Lid into Exos: %d %s", code, body)
	}
	var p models.LoadoutPlace
	if err := json.Unmarshal([]byte(body), &p); err != nil || p.ParentPlaceID == nil || *p.ParentPlaceID != exos {
		t.Fatalf("response = %s", body)
	}
	// Exos into Lid: cycle (409); Exos into itself: 400.
	up(exos, lid, http.StatusConflict)
	up(exos, exos, http.StatusBadRequest)
	// A deeper cycle: Pad in Lid in Exos; Exos into Pad.
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, pad, "update"), map[string]any{"parent_place_id": lid}); code != http.StatusOK {
		t.Fatal(code, body)
	}
	up(exos, pad, http.StatusConflict)
	// Parent that is Worn: 400. Target that is Worn: 400.
	up(lid, f.worn, http.StatusBadRequest)
	up(f.worn, exos, http.StatusBadRequest)
	// 404: parent of another loadout / another user / unknown, identical; unknown, foreign place.
	a := up(lid, f.otherBag, http.StatusNotFound)
	b := up(lid, f.bobBag, http.StatusNotFound)
	c := up(lid, 999999, http.StatusNotFound)
	if a != b || b != c {
		t.Errorf("parent 404 bodies differ: %q %q %q", a, b, c)
	}
	d := up(f.otherBag, exos, http.StatusNotFound)
	e := up(999999, exos, http.StatusNotFound)
	if d != e || d != a {
		t.Errorf("place 404 bodies differ: %q %q %q", d, e, a)
	}
	// 400: parent_place_id missing or not a number.
	f.refused(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{}, http.StatusBadRequest)
	f.refused(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": "x"}, http.StatusBadRequest)
	// Refusal order: the 404 of the place beats the 400 of a bad body.
	f.refused(http.MethodPost, pPlacePath(f.lo, 999999, "update"), map[string]any{}, http.StatusNotFound)
	// Refusal order: 400 (self) beats 409.
	up(lid, lid, http.StatusBadRequest)

	// null: top level. Lid leaves Exos; Pad stays in Lid.
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": nil}); code != http.StatusOK {
		t.Fatal(code, body)
	}
	if f.parentOf(lid) != nil {
		t.Errorf("Lid should be top level")
	}
	if p := f.parentOf(pad); p == nil || *p != lid {
		t.Errorf("Pad should still be in Lid")
	}
	// Now Exos into Lid is fine: Lid is no longer inside Exos.
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, exos, "update"), map[string]any{"parent_place_id": lid}); code != http.StatusOK {
		t.Fatal(code, body)
	}
}

func TestPlaceUpdate_NeverWritesClosetLinks(t *testing.T) {
	f := newPlaceFixture(t)
	f.env.SeedContainerLink(f.exos, f.pad)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	closet := func() string {
		return fmt.Sprint(f.env.Count("user_container_registration", "1 = 1"),
			f.env.Count("user_container_registration", "userContainerId = ? AND userGearRegistrationId = ?", f.exos, f.pad))
	}
	before := closet()
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": exos}); code != http.StatusOK {
		t.Fatal(code, body)
	}
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": nil}); code != http.StatusOK {
		t.Fatal(code, body)
	}
	if closet() != before {
		t.Errorf("closet links changed: %s -> %s", before, closet())
	}
	// And the other way round: a closet change leaves the loadout alone.
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": exos}); code != http.StatusOK {
		t.Fatal(code, body)
	}
	f.env.SeedContainerLink(f.lid, f.tent)
	if p := f.parentOf(lid); p == nil || *p != exos {
		t.Errorf("closet change moved a bag")
	}
}

func TestPlaceUpdate_ParentThatLostCapabilityReceivesNothingNew(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, map[string]any{"parent_place_id": exos})
	pad := f.addBag(f.pad, nil)
	f.env.setCategoryContainer(f.exosCategory, boolPtr(false))

	f.refused(http.MethodPost, pPlacePath(f.lo, pad, "update"), map[string]any{"parent_place_id": exos}, http.StatusConflict)
	// What it already holds stays, and leaving it is allowed.
	if p := f.parentOf(lid); p == nil || *p != exos {
		t.Fatal("Lid left Exos by itself")
	}
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": exos}); code != http.StatusOK {
		t.Errorf("leaving the parent as it is: %d %s", code, body)
	}
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": nil}); code != http.StatusOK {
		t.Errorf("leaving a bag that lost capability: %d %s", code, body)
	}
}

func TestPlace_CapacityAndGoalNeverRefuse(t *testing.T) {
	f := newPlaceFixture(t)
	if _, err := f.env.DB.Exec(`UPDATE user_gear_registrations SET maxContainerWeight = 1 WHERE userGearRegistrationId = ?`, f.exos); err != nil {
		t.Fatal(err)
	}
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	// 100 g of Lid and 1000 g of items in a 1 g bag.
	w := f.env.Do(http.MethodPut, loPath(f.lo, "item/insert"), f.aTok,
		map[string]any{"usergear_registration_id": f.tent, "place_id": exos, "quantity": 5})
	expectStatus(t, w, http.StatusCreated)
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, lid, "update"), map[string]any{"parent_place_id": exos}); code != http.StatusOK {
		t.Fatalf("over capacity: %d %s", code, body)
	}
}

// ---- Order (BR-024 AC-9) ----------------------------------------------------

func TestPlaceOrder_SetsOrderAndRemovalKeepsTheOthers(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	pad := f.addBag(f.pad, nil)
	lm := f.addLastMinute()

	w := f.env.Do(http.MethodPost, pPath(f.lo, "order"), f.aTok, map[string]any{"place_ids": []int64{pad, exos, lid}})
	expectStatus(t, w, http.StatusOK)
	var got []models.LoadoutPlace
	decodeJSON(t, w, &got)
	if len(got) != 3 || got[0].PlaceID != pad || got[1].PlaceID != exos || got[2].PlaceID != lid {
		t.Fatalf("response order = %+v", got)
	}
	// A second session reads the same order.
	if order := f.bagOrder(f.lo); fmt.Sprint(order) != fmt.Sprint([]int64{pad, exos, lid}) {
		t.Errorf("stored order = %v", order)
	}
	// Removing a bag keeps the order of the others; a new bag goes last.
	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, exos, "delete"), nil); code != http.StatusOK {
		t.Fatal(code, body)
	}
	if order := f.bagOrder(f.lo); fmt.Sprint(order) != fmt.Sprint([]int64{pad, lid}) {
		t.Errorf("order after removal = %v", order)
	}
	tent := f.env.seedGearX(true, false, 0, f.env.seedTopCategory("More"))
	neu := f.addBag(f.env.SeedRegistration(f.alice, tent), nil)
	if order := f.bagOrder(f.lo); fmt.Sprint(order) != fmt.Sprint([]int64{pad, lid, neu}) {
		t.Errorf("order with a new bag = %v", order)
	}
	// The same call with only the remaining bags works, and Last minute is not a bag.
	f.refused(http.MethodPost, pPath(f.lo, "order"), map[string]any{"place_ids": []int64{lid, pad, neu, lm}}, http.StatusBadRequest)
}

func TestPlaceOrder_Refusals(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	ord := func(ids any, want int) string {
		return f.refused(http.MethodPost, pPath(f.lo, "order"), map[string]any{"place_ids": ids}, want)
	}
	// 404: an ID of another loadout, another user's, or unknown: identical.
	a := ord([]int64{exos, lid, f.otherBag}, http.StatusNotFound)
	b := ord([]int64{exos, lid, f.bobBag}, http.StatusNotFound)
	c := ord([]int64{exos, lid, 999999}, http.StatusNotFound)
	if a != b || b != c {
		t.Errorf("404 bodies differ: %q %q %q", a, b, c)
	}
	// 404 beats 400: an unknown ID together with a duplicate.
	ord([]int64{exos, exos, 999999}, http.StatusNotFound)
	// 400: Worn, duplicate, missing bag, not a list, absent.
	ord([]int64{exos, lid, f.worn}, http.StatusBadRequest)
	ord([]int64{exos, exos}, http.StatusBadRequest)
	ord([]int64{exos}, http.StatusBadRequest)
	ord([]int64{}, http.StatusBadRequest)
	ord("x", http.StatusBadRequest)
	f.refused(http.MethodPost, pPath(f.lo, "order"), map[string]any{}, http.StatusBadRequest)
	f.refused(http.MethodPost, pPath(f.lo, "order"), map[string]any{"place_ids": nil}, http.StatusBadRequest)
	// A loadout without bags orders the empty list.
	code, body := f.do(http.MethodPost, pPath(f.loOther, "order"), map[string]any{"place_ids": []int64{f.otherBag}})
	if code != http.StatusOK {
		t.Errorf("order of the other loadout: %d %s", code, body)
	}
}

// ---- Close (BR-025 AC-3, AC-5) ----------------------------------------------

func TestPlaceClose_SetsToValueAndTouchesNothingElse(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, map[string]any{"parent_place_id": exos})
	lm := f.addLastMinute()
	tick := f.env.seedRegItem(f.lo, f.tent, exos, 1)
	untick := f.env.seedRegItem(f.lo, f.tent, exos, 1)
	if _, err := f.env.DB.Exec(`UPDATE loadout_items SET ticked = 1 WHERE loadoutItemId = ?`, tick); err != nil {
		t.Fatal(err)
	}

	closed := func(id int64) bool { return f.place(f.lo, id).Closed }
	for i := 0; i < 2; i++ { // idempotent
		code, body := f.do(http.MethodPost, pPlacePath(f.lo, exos, "close"), map[string]any{"closed": true})
		if code != http.StatusOK || !strings.Contains(body, `"closed":true`) || !strings.Contains(body, fmt.Sprintf(`"place_id":%d`, exos)) {
			t.Fatalf("close #%d: %d %s", i, code, body)
		}
	}
	// Closing a place with an unticked item is allowed; nothing else changed.
	if !closed(exos) || closed(lid) || closed(f.worn) || closed(lm) {
		t.Errorf("closed states = exos %v lid %v worn %v lm %v", closed(exos), closed(lid), closed(f.worn), closed(lm))
	}
	var t1, t2 bool
	f.env.DB.QueryRow(`SELECT ticked FROM loadout_items WHERE loadoutItemId = ?`, tick).Scan(&t1)
	f.env.DB.QueryRow(`SELECT ticked FROM loadout_items WHERE loadoutItemId = ?`, untick).Scan(&t2)
	if !t1 || t2 {
		t.Errorf("ticks = %v %v, want true false", t1, t2)
	}
	// Open again: ticks unchanged. Worn and Last minute are closable.
	for _, id := range []int64{exos, f.worn, lm} {
		if code, body := f.do(http.MethodPost, pPlacePath(f.lo, id, "close"), map[string]any{"closed": true}); code != http.StatusOK {
			t.Fatal(code, body)
		}
	}
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, exos, "close"), map[string]any{"closed": false}); code != http.StatusOK || !strings.Contains(body, `"closed":false`) {
		t.Fatal(code, body)
	}
	if closed(exos) || !closed(f.worn) || !closed(lm) || closed(lid) {
		t.Errorf("closed states after reopen wrong")
	}
	f.env.DB.QueryRow(`SELECT ticked FROM loadout_items WHERE loadoutItemId = ?`, tick).Scan(&t1)
	if !t1 {
		t.Error("opening a place changed a tick")
	}
	// Moving items and changing them never changes a closed state (BR-025 AC-4).
	if code, body := f.do(http.MethodPost, itemPath(f.lo, tick, "update"), map[string]any{"place_id": f.worn, "quantity": 3}); code != http.StatusOK {
		t.Fatal(code, body)
	}
	if !closed(f.worn) {
		t.Error("moving an item into Worn opened it")
	}
}

func TestPlaceClose_Refusals(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	cl := func(place any, body any, want int) string {
		return f.refused(http.MethodPost, fmt.Sprintf("/api/v1/loadout/%d/place/%v/close", f.lo, place), body, want)
	}
	yes := map[string]any{"closed": true}
	cl(0, yes, http.StatusBadRequest) // "no place" cannot be closed
	cl("abc", yes, http.StatusBadRequest)
	cl("-0", yes, http.StatusBadRequest)
	a := cl(f.otherBag, yes, http.StatusNotFound)
	b := cl(f.bobBag, yes, http.StatusNotFound)
	c := cl(999999, yes, http.StatusNotFound)
	if a != b || b != c {
		t.Errorf("404 bodies differ: %q %q %q", a, b, c)
	}
	cl(exos, map[string]any{}, http.StatusBadRequest)
	cl(exos, map[string]any{"closed": "yes"}, http.StatusBadRequest)
	cl(exos, map[string]any{"closed": nil}, http.StatusBadRequest)
	cl(exos, "[1]", http.StatusBadRequest)
	// 404 before 400: an unknown place with a bad body.
	cl(999999, map[string]any{}, http.StatusNotFound)
}

// ---- Delete (BR-024 AC-8, BR-025 AC-5) --------------------------------------

func TestPlaceDelete_BagUnplacesItemsLiftsChildrenAndEndsClosedState(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, map[string]any{"parent_place_id": exos})
	pad := f.addBag(f.pad, nil)
	inExos := f.env.seedRegItem(f.lo, f.tent, exos, 1)
	inLid := f.env.seedRegItem(f.lo, f.tent, lid, 1)
	inPad := f.env.seedRegItem(f.lo, f.tent, pad, 1)
	if _, err := f.env.DB.Exec(`UPDATE loadout_items SET ticked = 1 WHERE loadoutItemId = ?`, inExos); err != nil {
		t.Fatal(err)
	}
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, exos, "close"), map[string]any{"closed": true}); code != http.StatusOK {
		t.Fatal(code, body)
	}

	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, exos, "delete"), nil); code != http.StatusOK {
		t.Fatalf("delete bag: %d %s", code, body)
	}
	itemPlace := func(id int64) (place sql.NullInt64, ticked bool) {
		if err := f.env.DB.QueryRow(`SELECT placeId, ticked FROM loadout_items WHERE loadoutItemId = ?`, id).Scan(&place, &ticked); err != nil {
			t.Fatalf("item %d: %v", id, err)
		}
		return
	}
	if p, tk := itemPlace(inExos); p.Valid || !tk {
		t.Errorf("item of the removed bag: place %v ticked %v, want no place and still ticked", p, tk)
	}
	if p, _ := itemPlace(inLid); !p.Valid || p.Int64 != lid {
		t.Errorf("item of the child bag moved: %v", p)
	}
	if p, _ := itemPlace(inPad); !p.Valid || p.Int64 != pad {
		t.Errorf("item of another bag moved: %v", p)
	}
	if f.parentOf(lid) != nil {
		t.Error("child bag is not top level")
	}
	if f.place(f.lo, exos) != nil || f.place(f.lo, lid) == nil || f.place(f.lo, pad) == nil {
		t.Error("wrong places left")
	}
	if n := f.env.Count("user_gear_registrations", "userGearRegistrationId = ?", f.exos); n != 1 {
		t.Error("the registration left the closet")
	}
	// Added again later, the bag starts open (BR-025 AC-5).
	again := f.addBag(f.exos, nil)
	if f.place(f.lo, again).Closed {
		t.Error("a re-added bag is closed")
	}
}

func TestPlaceDelete_LastMinuteUnplacesItsItems(t *testing.T) {
	f := newPlaceFixture(t)
	lm := f.addLastMinute()
	exos := f.addBag(f.exos, nil)
	inLM := f.env.seedRegItem(f.lo, f.tent, lm, 1)
	inBag := f.env.seedRegItem(f.lo, f.tent, exos, 1)
	if code, body := f.do(http.MethodDelete, pPlacePath(f.lo, lm, "delete"), nil); code != http.StatusOK {
		t.Fatal(code, body)
	}
	var p1, p2 sql.NullInt64
	f.env.DB.QueryRow(`SELECT placeId FROM loadout_items WHERE loadoutItemId = ?`, inLM).Scan(&p1)
	f.env.DB.QueryRow(`SELECT placeId FROM loadout_items WHERE loadoutItemId = ?`, inBag).Scan(&p2)
	if p1.Valid || !p2.Valid || p2.Int64 != exos {
		t.Errorf("places after removing Last minute = %v %v", p1, p2)
	}
	if n := f.env.Count("loadout_items", "loadoutId = ?", f.lo); n != 2 {
		t.Errorf("items left = %d, want 2", n)
	}
}

func TestPlaceDelete_Refusals(t *testing.T) {
	f := newPlaceFixture(t)
	f.addBag(f.exos, nil)
	a := f.refused(http.MethodDelete, pPlacePath(f.lo, f.otherBag, "delete"), nil, http.StatusNotFound)
	b := f.refused(http.MethodDelete, pPlacePath(f.lo, f.bobBag, "delete"), nil, http.StatusNotFound)
	c := f.refused(http.MethodDelete, pPlacePath(f.lo, 999999, "delete"), nil, http.StatusNotFound)
	if a != b || b != c {
		t.Errorf("404 bodies differ: %q %q %q", a, b, c)
	}
	f.refused(http.MethodDelete, pPlacePath(f.lo, f.worn, "delete"), nil, http.StatusBadRequest)
	f.refused(http.MethodDelete, fmt.Sprintf("/api/v1/loadout/%d/place/abc/delete", f.lo), nil, http.StatusBadRequest)
}

// ---- Ownership (BR-015 AC-7, BR-018) ----------------------------------------

func TestPlaceEndpoints_ForeignAndUnknownLoadoutsAreIdentical404(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	ops := func(lo int64) map[string]struct {
		method, path string
		body         any
	} {
		return map[string]struct {
			method, path string
			body         any
		}{
			"insert": {http.MethodPut, pPath(lo, "insert"), map[string]any{"place_kind": "bag", "usergear_registration_id": f.lid}},
			"order":  {http.MethodPost, pPath(lo, "order"), map[string]any{"place_ids": []int64{exos}}},
			"update": {http.MethodPost, pPlacePath(lo, exos, "update"), map[string]any{"parent_place_id": nil}},
			"close":  {http.MethodPost, pPlacePath(lo, exos, "close"), map[string]any{"closed": true}},
			"delete": {http.MethodDelete, pPlacePath(lo, exos, "delete"), nil},
		}
	}
	before := f.stored()
	for name, o := range ops(f.lo) {
		bob := f.env.Do(o.method, o.path, f.bTok, o.body)
		unknown := f.env.Do(o.method, ops(999999)[name].path, f.bTok, o.body)
		if bob.Code != http.StatusNotFound || unknown.Code != http.StatusNotFound || bob.Body.String() != unknown.Body.String() {
			t.Errorf("%s: foreign %d %q, unknown %d %q; want identical 404", name, bob.Code, bob.Body.String(), unknown.Code, unknown.Body.String())
		}
		if anon := f.env.Do(o.method, o.path, "", o.body); anon.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: %d, want 401", name, anon.Code)
		}
	}
	if f.stored() != before {
		t.Error("a refused request changed stored state")
	}
	// Admins get no exception.
	admin := f.env.SeedUser("root", "root@example.com", true)
	if w := f.env.Do(http.MethodDelete, pPlacePath(f.lo, exos, "delete"), f.env.Token(admin, true), nil); w.Code != http.StatusNotFound {
		t.Errorf("admin delete of another user's place: %d, want 404", w.Code)
	}
}

// ---- Registration delete (BR-030 AC-3, AC-4) --------------------------------

func TestPlace_DeletingABagRegistrationThroughTheAPIStopsItBeingABag(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, map[string]any{"parent_place_id": exos})
	item := f.env.seedRegItem(f.lo, f.tent, exos, 2)
	if _, err := f.env.DB.Exec(`UPDATE loadout_items SET ticked = 1 WHERE loadoutItemId = ?`, item); err != nil {
		t.Fatal(err)
	}
	selfItem := f.env.seedRegItem(f.lo, f.exos, lid, 1)
	if _, err := f.env.DB.Exec(`UPDATE loadout_items SET ticked = 1 WHERE loadoutItemId = ?`, selfItem); err != nil {
		t.Fatal(err)
	}
	if code, body := f.do(http.MethodPost, pPlacePath(f.lo, exos, "close"), map[string]any{"closed": true}); code != http.StatusOK {
		t.Fatal(code, body)
	}

	w := f.env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/usergear/registration/%d/delete", f.exos), f.aTok, nil)
	expectStatus(t, w, http.StatusOK)

	if f.place(f.lo, exos) != nil {
		t.Error("the deleted registration is still a bag")
	}
	if p := f.place(f.lo, lid); p == nil || p.ParentPlaceID != nil {
		t.Errorf("child bag = %+v, want top level", p)
	}
	var place sql.NullInt64
	var ticked bool
	var reason sql.NullString
	if err := f.env.DB.QueryRow(`SELECT placeId, ticked, detachedReason FROM loadout_items WHERE loadoutItemId = ?`, item).Scan(&place, &ticked, &reason); err != nil {
		t.Fatal(err)
	}
	if place.Valid || !ticked || reason.String != "" {
		t.Errorf("item = place %v ticked %v reason %q", place, ticked, reason.String)
	}
	// An item backed by the deleted bag registration itself is kept as a snapshot, ticked, in its place (the child bag).
	if err := f.env.DB.QueryRow(`SELECT placeId, ticked, detachedReason FROM loadout_items WHERE loadoutItemId = ?`, selfItem).Scan(&place, &ticked, &reason); err != nil {
		t.Fatal(err)
	}
	if !place.Valid || place.Int64 != lid || !ticked || reason.String != "registration_deleted" {
		t.Errorf("self-backed item = place %v ticked %v reason %q", place, ticked, reason.String)
	}
}

// ---- Concurrency (BR-024 AC-10) ---------------------------------------------

// hasCycle reports whether any bag of the loadout is inside itself.
func (f *placeFixture) hasCycle(loadout int64) bool {
	f.env.T.Helper()
	for _, id := range f.bagOrder(loadout) {
		var inside bool
		err := f.env.DB.QueryRow(
			`WITH RECURSIVE up(id) AS (
			   SELECT parentPlaceId FROM loadout_places WHERE placeId = ?
			   UNION
			   SELECT p.parentPlaceId FROM loadout_places p JOIN up ON p.placeId = up.id WHERE p.parentPlaceId IS NOT NULL)
			 SELECT EXISTS(SELECT 1 FROM up WHERE id = ?)`, id, id).Scan(&inside)
		if err != nil {
			f.env.T.Fatal(err)
		}
		if inside {
			return true
		}
	}
	return false
}

// race runs the functions at the same time and returns their status codes.
func race(fns ...func() int) []int {
	codes := make([]int, len(fns))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		go func(i int, fn func() int) {
			defer wg.Done()
			<-start
			codes[i] = fn()
		}(i, fn)
	}
	close(start)
	wg.Wait()
	return codes
}

func TestPlaceRace_OppositeNestingLeavesAtMostOneAndNoCycle(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	for round := 0; round < 40; round++ {
		if _, err := f.env.DB.Exec(`UPDATE loadout_places SET parentPlaceId = NULL WHERE loadoutId = ?`, f.lo); err != nil {
			t.Fatal(err)
		}
		codes := race(
			func() int {
				return f.env.Do(http.MethodPost, pPlacePath(f.lo, lid, "update"), f.aTok, map[string]any{"parent_place_id": exos}).Code
			},
			func() int {
				return f.env.Do(http.MethodPost, pPlacePath(f.lo, exos, "update"), f.aTok, map[string]any{"parent_place_id": lid}).Code
			},
		)
		ok, conflict := 0, 0
		for _, c := range codes {
			switch c {
			case http.StatusOK:
				ok++
			case http.StatusConflict:
				conflict++
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatalf("round %d: statuses %v, want one 200 and one 409", round, codes)
		}
		if f.hasCycle(f.lo) {
			t.Fatalf("round %d: a bag is inside itself", round)
		}
		inLid, inExos := f.parentOf(exos), f.parentOf(lid)
		if (inLid != nil) == (inExos != nil) {
			t.Fatalf("round %d: exactly one nesting must be in effect, got exos-parent %v lid-parent %v", round, inLid, inExos)
		}
	}
}

func TestPlaceRace_ConcurrentMovesLeaveEveryItemInExactlyOnePlace(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	lm := f.addLastMinute()
	targets := []int64{exos, lid, lm, f.worn}
	var items []int64
	for i := 0; i < 4; i++ {
		items = append(items, f.env.seedRegItem(f.lo, f.tent, 0, 1))
	}
	for round := 0; round < 25; round++ {
		var fns []func() int
		want := map[int64][]int64{}
		for k, it := range items {
			for _, target := range []int64{targets[(k+round)%4], targets[(k+round+1)%4]} {
				it, target := it, target
				want[it] = append(want[it], target)
				fns = append(fns, func() int {
					return f.env.Do(http.MethodPost, itemPath(f.lo, it, "update"), f.aTok, map[string]any{"place_id": target}).Code
				})
			}
		}
		for i, code := range race(fns...) {
			if code != http.StatusOK {
				t.Fatalf("round %d request %d: status %d", round, i, code)
			}
		}
		for _, it := range items {
			var n int
			var place sql.NullInt64
			if err := f.env.DB.QueryRow(`SELECT COUNT(*), MAX(placeId) FROM loadout_items WHERE loadoutItemId = ?`, it).Scan(&n, &place); err != nil {
				t.Fatal(err)
			}
			if n != 1 || !place.Valid {
				t.Fatalf("round %d item %d: rows %d place %v", round, it, n, place)
			}
			if place.Int64 != want[it][0] && place.Int64 != want[it][1] {
				t.Fatalf("round %d item %d is in %d, want one of %v", round, it, place.Int64, want[it])
			}
		}
	}
}

func TestPlaceRace_ConcurrentAddsRespectOnePerLoadout(t *testing.T) {
	f := newPlaceFixture(t)
	// Two Last minute adds, two adds of the same bag, and a bag add that
	// races its own removal: never two of a kind.
	for round := 0; round < 15; round++ {
		lmAdd := func() int {
			return f.env.Do(http.MethodPut, pPath(f.lo, "insert"), f.aTok, map[string]any{"place_kind": "last_minute"}).Code
		}
		bagAdd := func() int {
			return f.env.Do(http.MethodPut, pPath(f.lo, "insert"), f.aTok,
				map[string]any{"place_kind": "bag", "usergear_registration_id": f.exos}).Code
		}
		codes := race(lmAdd, lmAdd, bagAdd, bagAdd)
		if created := countCode(codes[:2], http.StatusCreated); created != 1 || countCode(codes[:2], http.StatusConflict) != 1 {
			t.Fatalf("round %d Last minute statuses %v", round, codes[:2])
		}
		if created := countCode(codes[2:], http.StatusCreated); created != 1 || countCode(codes[2:], http.StatusConflict) != 1 {
			t.Fatalf("round %d bag statuses %v", round, codes[2:])
		}
		// Reset for the next round.
		if _, err := f.env.DB.Exec(`DELETE FROM loadout_places WHERE loadoutId = ? AND placeKind <> 'worn'`, f.lo); err != nil {
			t.Fatal(err)
		}
	}
}

func countCode(codes []int, want int) int {
	n := 0
	for _, c := range codes {
		if c == want {
			n++
		}
	}
	return n
}

func TestPlaceRace_ConcurrentOrderAndAddKeepEveryBagOnce(t *testing.T) {
	f := newPlaceFixture(t)
	exos := f.addBag(f.exos, nil)
	lid := f.addBag(f.lid, nil)
	for round := 0; round < 20; round++ {
		codes := race(
			func() int {
				return f.env.Do(http.MethodPost, pPath(f.lo, "order"), f.aTok, map[string]any{"place_ids": []int64{lid, exos}}).Code
			},
			func() int {
				return f.env.Do(http.MethodPost, pPath(f.lo, "order"), f.aTok, map[string]any{"place_ids": []int64{exos, lid}}).Code
			},
		)
		if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
			t.Fatalf("round %d: %v", round, codes)
		}
		order := f.bagOrder(f.lo)
		if len(order) != 2 || order[0] == order[1] {
			t.Fatalf("round %d: order %v", round, order)
		}
		var distinct int
		f.env.DB.QueryRow(`SELECT COUNT(DISTINCT position) FROM loadout_places WHERE loadoutId = ? AND placeKind = 'bag'`, f.lo).Scan(&distinct)
		if distinct != 2 {
			t.Fatalf("round %d: positions are not distinct", round)
		}
	}
}
