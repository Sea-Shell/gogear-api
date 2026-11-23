package endpoints

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Item contract of ARS-001 milestone A (BR-015, BR-023, BR-025, BR-016 AC-5,
// ADR-004 D-2). Bags are seeded with SQL here; API-created bags are covered
// with the place endpoints.

type itemFixture struct {
	env            *authzEnv
	alice, bob     int64
	aTok, bTok     string
	lo, loBob      int64
	tent, pack     int64 // alice's registrations: tent (not a container), pack (container)
	packGear       int64
	tentGear       int64
	bobReg         int64
	bag, worn, lm  int64 // places of lo
	noCapBag       int64 // a bag whose gear cannot hold items (BR-024 AC-6)
	bobBag         int64 // a place of bob's loadout
	zeroGear       int64 // catalog weight 0
	zeroReg        int64
	top            int64
	packW, tentW   int64
	sleepGear      int64 // 250 g, not a container
	sleepRegLow    int64
	sleepRegHigher int64
}

func newItemFixture(t *testing.T) *itemFixture {
	env := newAuthzEnv(t)
	f := &itemFixture{env: env}
	f.alice = env.SeedUser("alice", "alice@example.com", false)
	f.bob = env.SeedUser("bob", "bob@example.com", false)
	f.aTok, f.bTok = env.Token(f.alice, false), env.Token(f.bob, false)
	f.top = env.seedTopCategory("Packs")
	f.packGear = env.seedGearX(true, false, 0, f.top) // container, 100 g
	f.tentGear = env.seedGearW(1000)
	f.zeroGear = env.seedGearW(0)
	f.sleepGear = env.seedGearW(250)
	f.pack = env.SeedRegistration(f.alice, f.packGear)
	f.tent = env.SeedRegistration(f.alice, f.tentGear)
	f.zeroReg = env.SeedRegistration(f.alice, f.zeroGear)
	f.sleepRegLow = env.SeedRegistration(f.alice, f.sleepGear)
	f.sleepRegHigher = env.SeedRegistration(f.alice, f.sleepGear)
	f.bobReg = env.SeedRegistration(f.bob, f.tentGear)
	f.lo = env.seedLO(f.alice, false, "alice-lo")
	f.loBob = env.seedLO(f.bob, false, "bob-lo")
	f.worn = f.placeOfKind(f.lo, "worn")
	f.lm = f.addLastMinute(f.lo)
	f.bag = f.addBag(f.lo, f.pack, 1)
	f.noCapBag = f.addBag(f.lo, f.zeroReg, 2) // 0 g, so it adds nothing to totals
	f.bobBag = f.addBag(f.loBob, env.SeedRegistration(f.bob, f.packGear), 1)
	return f
}

func (f *itemFixture) placeOfKind(loadout int64, kind string) int64 {
	f.env.T.Helper()
	var id int64
	if err := f.env.DB.QueryRow(`SELECT placeId FROM loadout_places WHERE loadoutId = ? AND placeKind = ?`, loadout, kind).Scan(&id); err != nil {
		f.env.T.Fatal(err)
	}
	return id
}

func (f *itemFixture) addLastMinute(loadout int64) int64 {
	f.env.T.Helper()
	res, err := f.env.DB.Exec(`INSERT INTO loadout_places (loadoutId, placeKind) VALUES (?, 'last_minute')`, loadout)
	if err != nil {
		f.env.T.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (f *itemFixture) addBag(loadout, registration, position int64) int64 {
	f.env.T.Helper()
	res, err := f.env.DB.Exec(`INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (?, 'bag', ?, ?)`,
		loadout, registration, position)
	if err != nil {
		f.env.T.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (f *itemFixture) insert(body any) (int, models.LoadoutItem, string) {
	f.env.T.Helper()
	w := f.env.Do(http.MethodPut, loPath(f.lo, "item/insert"), f.aTok, body)
	var it models.LoadoutItem
	if w.Code == http.StatusCreated {
		decodeJSON(f.env.T, w, &it)
	}
	return w.Code, it, w.Body.String()
}

func (f *itemFixture) items() []models.LoadoutItem {
	f.env.T.Helper()
	w := f.env.Do(http.MethodGet, loPath(f.lo, "item/list"), f.aTok, nil)
	expectStatus(f.env.T, w, http.StatusOK)
	var items []models.LoadoutItem
	decodeJSON(f.env.T, w, &items)
	return items
}

func (f *itemFixture) loadoutTotal(id int64, tok string) int64 {
	f.env.T.Helper()
	w := f.env.Do(http.MethodGet, loPath(id, "get"), tok, nil)
	expectStatus(f.env.T, w, http.StatusOK)
	var l models.Loadout
	decodeJSON(f.env.T, w, &l)
	return l.TotalWeight
}

// ---- insert by registration -------------------------------------------------

func TestItemInsert_ByRegistrationReturnsContractFields(t *testing.T) {
	f := newItemFixture(t)
	code, it, body := f.insert(map[string]any{"usergear_registration_id": f.tent, "quantity": 2, "notes": "fly"})
	if code != http.StatusCreated {
		t.Fatalf("status %d: %s", code, body)
	}
	if it.LoadoutItemID == nil || it.LoadoutID != f.lo || it.Quantity != 2 || it.Notes != "fly" {
		t.Errorf("basic fields: %+v", it)
	}
	if it.GearID == nil || *it.GearID != f.tentGear {
		t.Errorf("gear_id = %v, want %d", it.GearID, f.tentGear)
	}
	if it.UsergearRegistrationID == nil || *it.UsergearRegistrationID != f.tent || !it.RegistrationExists || it.DetachedReason != nil {
		t.Errorf("registration fields: %+v", it)
	}
	if it.GearName != "g" || it.ManufactureName == nil || it.CategoryName == nil || it.TopCategoryName == nil {
		t.Errorf("names: %+v", it)
	}
	if it.PlaceID != nil || it.Ticked {
		t.Errorf("a new item has no place and is not ticked: %+v", it)
	}
	if it.CatalogWeight == nil || *it.CatalogWeight != 1000 || it.UnitWeight == nil || *it.UnitWeight != 1000 || it.WeightSource != "catalog" {
		t.Errorf("weights: %+v", it)
	}
	if got := f.loadoutTotal(f.lo, f.aTok); got != 2100 {
		t.Errorf("loadout total_weight = %d, want 2100 (items without a place and the 100 g pack count)", got)
	}
}

func TestItemInsert_DefaultsQuantityAndReportsZeroWeightAsNull(t *testing.T) {
	f := newItemFixture(t)
	code, it, body := f.insert(map[string]any{"usergear_registration_id": f.zeroReg})
	if code != http.StatusCreated {
		t.Fatalf("status %d: %s", code, body)
	}
	if it.Quantity != 1 || it.Notes != "" {
		t.Errorf("defaults: %+v", it)
	}
	if it.CatalogWeight != nil || it.UnitWeight != nil || it.WeightSource != "none" {
		t.Errorf("a gear weight of 0 is no weight: %+v", it)
	}
	if !strings.Contains(f.env.Do(http.MethodGet, loPath(f.lo, "item/list"), f.aTok, nil).Body.String(), `"unit_weight":null`) {
		t.Error("unit_weight is not encoded as null")
	}
}

func TestItemInsert_IntoPlaces(t *testing.T) {
	f := newItemFixture(t)
	for _, tc := range []struct {
		name  string
		place int64
	}{{"worn", f.worn}, {"last minute", f.lm}, {"capable bag", f.bag}} {
		code, it, body := f.insert(map[string]any{"usergear_registration_id": f.tent, "place_id": tc.place})
		if code != http.StatusCreated || it.PlaceID == nil || *it.PlaceID != tc.place {
			t.Errorf("%s: status %d place %v: %s", tc.name, code, it.PlaceID, body)
		}
	}
}

func TestItemInsert_RefusalsChangeNothing(t *testing.T) {
	f := newItemFixture(t)
	before := f.env.snapshot()
	for _, tc := range []struct {
		name string
		body any
		want int
	}{
		{"foreign registration", map[string]any{"usergear_registration_id": f.bobReg}, 404},
		{"unknown registration", map[string]any{"usergear_registration_id": 987654}, 404},
		{"place of another loadout", map[string]any{"usergear_registration_id": f.tent, "place_id": f.bobBag}, 404},
		{"unknown place", map[string]any{"usergear_registration_id": f.tent, "place_id": 987654}, 404},
		{"both gear_id and registration", map[string]any{"usergear_registration_id": f.tent, "gear_id": f.tentGear}, 400},
		{"neither", map[string]any{"quantity": 1}, 400},
		{"quantity zero", map[string]any{"usergear_registration_id": f.tent, "quantity": 0}, 400},
		{"quantity negative", map[string]any{"usergear_registration_id": f.tent, "quantity": -3}, 400},
		{"quantity not a number", `{"usergear_registration_id": ` + fmt.Sprint(f.tent) + `, "quantity": "two"}`, 400},
		{"loadout_id differs", map[string]any{"usergear_registration_id": f.tent, "loadout_id": f.loBob}, 400},
		{"malformed JSON", `{`, 400},
		{"non-capable bag", map[string]any{"usergear_registration_id": f.sleepRegLow, "place_id": f.noCapBag}, 409},
	} {
		code, _, body := f.insert(tc.body)
		if code != tc.want {
			t.Errorf("%s: status %d, want %d: %s", tc.name, code, tc.want, body)
		}
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("refused inserts changed data:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

func TestItemInsert_ForeignAndUnknownRegistrationAreIdentical(t *testing.T) {
	f := newItemFixture(t)
	foreign := f.env.Do(http.MethodPut, loPath(f.lo, "item/insert"), f.aTok, map[string]any{"usergear_registration_id": f.bobReg})
	unknown := f.env.Do(http.MethodPut, loPath(f.lo, "item/insert"), f.aTok, map[string]any{"usergear_registration_id": 987654})
	if foreign.Code != http.StatusNotFound || foreign.Code != unknown.Code || foreign.Body.String() != unknown.Body.String() {
		t.Fatalf("foreign %d %s / unknown %d %s", foreign.Code, foreign.Body, unknown.Code, unknown.Body)
	}
}

// 404 for the target comes before 400 for the request (ARS-001 section 1).
func TestItemInsert_RefusalOrderIs404Then400Then409(t *testing.T) {
	f := newItemFixture(t)
	code, _, _ := f.insert(map[string]any{"usergear_registration_id": 987654, "quantity": 0})
	if code != http.StatusNotFound {
		t.Errorf("unknown registration with bad quantity: %d, want 404", code)
	}
	code, _, _ = f.insert(map[string]any{"usergear_registration_id": f.sleepRegLow, "quantity": 0, "place_id": f.noCapBag})
	if code != http.StatusBadRequest {
		t.Errorf("bad quantity with non-capable bag: %d, want 400", code)
	}
	w := f.env.Do(http.MethodPut, loPath(f.loBob, "item/insert"), f.aTok, map[string]any{"usergear_registration_id": f.tent})
	expectStatus(t, w, http.StatusNotFound)
}

// ---- legacy insert by gear_id (ADR-004 D-2) ---------------------------------

func TestItemInsert_LegacyGearIDBindsToLowestOwnRegistration(t *testing.T) {
	f := newItemFixture(t)
	code, it, body := f.insert(map[string]any{"gear_id": f.sleepGear, "quantity": 1})
	if code != http.StatusCreated {
		t.Fatalf("status %d: %s", code, body)
	}
	if it.UsergearRegistrationID == nil || *it.UsergearRegistrationID != f.sleepRegLow || !it.RegistrationExists || it.DetachedReason != nil {
		t.Errorf("not bound to the lowest registration %d: %+v", f.sleepRegLow, it)
	}
	if it.GearID == nil || *it.GearID != f.sleepGear {
		t.Errorf("gear_id: %+v", it)
	}
}

func TestItemInsert_LegacyGearIDWithoutRegistrationIsStoredLegacyAndReadLive(t *testing.T) {
	f := newItemFixture(t)
	gear := f.env.seedGearW(400) // alice owns no registration of it
	code, it, body := f.insert(map[string]any{"gear_id": gear, "quantity": 3, "notes": "old frontend"})
	if code != http.StatusCreated {
		t.Fatalf("status %d: %s", code, body)
	}
	if it.UsergearRegistrationID != nil || it.RegistrationExists || it.DetachedReason == nil || *it.DetachedReason != "legacy" {
		t.Errorf("legacy fields: %+v", it)
	}
	if it.GearID == nil || *it.GearID != gear || it.GearName != "g" || it.ManufactureName == nil || it.UnitWeight == nil || *it.UnitWeight != 400 || it.WeightSource != "catalog" {
		t.Errorf("legacy item does not read its gear: %+v", it)
	}
	if got := f.loadoutTotal(f.lo, f.aTok); got != 1300 {
		t.Errorf("total = %d, want 1300", got)
	}

	// An admin's catalog edit shows at once, as for today's items.
	if _, err := f.env.DB.Exec(`UPDATE gear SET gearName = 'Renamed', gearWeight = 500 WHERE gearId = ?`, gear); err != nil {
		t.Fatal(err)
	}
	items := f.items()
	if len(items) != 1 || items[0].GearName != "Renamed" || items[0].UnitWeight == nil || *items[0].UnitWeight != 500 {
		t.Errorf("legacy item did not follow the catalog edit: %+v", items)
	}
	if got := f.loadoutTotal(f.lo, f.aTok); got != 1600 {
		t.Errorf("total after catalog edit = %d, want 1600", got)
	}

	// The gear cannot be deleted while a legacy item uses it (409, ADR-004 D-2).
	admin := f.env.SeedUser("root", "root@example.com", true)
	w := f.env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/gear/%d/delete", gear), f.env.Token(admin, true), nil)
	expectStatus(t, w, http.StatusConflict)
	if items := f.items(); len(items) != 1 || items[0].GearName != "Renamed" {
		t.Errorf("refused delete changed the item: %+v", items)
	}
}

func TestItemInsert_LegacyUnknownGearIs404(t *testing.T) {
	f := newItemFixture(t)
	before := f.env.snapshot()
	code, _, body := f.insert(map[string]any{"gear_id": 987654})
	if code != http.StatusNotFound || !strings.Contains(body, "Gear not found") {
		t.Fatalf("status %d: %s", code, body)
	}
	if f.env.snapshot() != before {
		t.Fatal("refused insert changed data")
	}
}

// ---- update ------------------------------------------------------------------

func (f *itemFixture) update(item int64, body any) (int, models.LoadoutItem, string) {
	f.env.T.Helper()
	w := f.env.Do(http.MethodPost, itemPath(f.lo, item, "update"), f.aTok, body)
	var it models.LoadoutItem
	if w.Code == http.StatusOK {
		decodeJSON(f.env.T, w, &it)
	}
	return w.Code, it, w.Body.String()
}

func (f *itemFixture) newItem(reg int64) int64 {
	f.env.T.Helper()
	code, it, body := f.insert(map[string]any{"usergear_registration_id": reg})
	if code != http.StatusCreated {
		f.env.T.Fatalf("insert: %d %s", code, body)
	}
	return *it.LoadoutItemID
}

func TestItemUpdate_QuantityNotesAndClearablePlace(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)

	// The deployed frontend's request shape.
	code, it, body := f.update(item, map[string]any{"loadout_item_id": item, "quantity": 4, "notes": "n"})
	if code != http.StatusOK || it.Quantity != 4 || it.Notes != "n" || it.PlaceID != nil {
		t.Fatalf("status %d: %s", code, body)
	}
	// Place set; absent keeps it; null clears it.
	code, it, body = f.update(item, map[string]any{"place_id": f.bag})
	if code != http.StatusOK || it.PlaceID == nil || *it.PlaceID != f.bag {
		t.Fatalf("set place: %d %s", code, body)
	}
	code, it, body = f.update(item, map[string]any{"notes": "again"})
	if code != http.StatusOK || it.PlaceID == nil || *it.PlaceID != f.bag || it.Notes != "again" || it.Quantity != 4 {
		t.Fatalf("absent place_id must keep the place: %d %s", code, body)
	}
	code, it, body = f.update(item, `{"place_id": null}`)
	if code != http.StatusOK || it.PlaceID != nil {
		t.Fatalf("null place_id must clear the place: %d %s", code, body)
	}
	code, it, body = f.update(item, map[string]any{"place_id": f.worn})
	if code != http.StatusOK || it.PlaceID == nil || *it.PlaceID != f.worn {
		t.Fatalf("move to Worn: %d %s", code, body)
	}
}

func TestItemUpdate_Refusals(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)
	bobItem := f.env.seedItem(f.loBob, f.tentGear, 1)
	before := f.env.snapshot()
	for _, tc := range []struct {
		name string
		item int64
		body any
		want int
	}{
		{"foreign place", item, map[string]any{"place_id": f.bobBag}, 404},
		{"unknown place", item, map[string]any{"place_id": 987654}, 404},
		{"unknown item", 987654, map[string]any{"quantity": 2}, 404},
		{"bob's item through alice's loadout", bobItem, map[string]any{"quantity": 2}, 404},
		{"quantity zero", item, map[string]any{"quantity": 0}, 400},
		{"no updatable field", item, map[string]any{}, 400},
		{"place_id not a number", item, `{"place_id": "x"}`, 400},
		{"loadout_item_id differs", item, map[string]any{"loadout_item_id": item + 1, "quantity": 2}, 400},
		{"loadout_id differs", item, map[string]any{"loadout_id": f.loBob, "quantity": 2}, 400},
		{"into a non-capable bag", item, map[string]any{"place_id": f.noCapBag}, 409},
		{"unknown place beats bad quantity", item, map[string]any{"place_id": 987654, "quantity": 0}, 404},
	} {
		if code, _, body := f.update(tc.item, tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d: %s", tc.name, code, tc.want, body)
		}
	}
	if f.env.snapshot() != before {
		t.Fatal("refused updates changed data")
	}
}

// ---- tick (BR-025) -------------------------------------------------------------

func (f *itemFixture) tick(loadout, item int64, tok string, body any) *struct {
	code int
	body string
} {
	w := f.env.Do(http.MethodPost, itemPath(loadout, item, "tick"), tok, body)
	return &struct {
		code int
		body string
	}{w.Code, w.Body.String()}
}

func (f *itemFixture) ticked(item int64) bool {
	f.env.T.Helper()
	var n int
	if err := f.env.DB.QueryRow(`SELECT ticked FROM loadout_items WHERE loadoutItemId = ?`, item).Scan(&n); err != nil {
		f.env.T.Fatal(err)
	}
	return n == 1
}

func TestItemTick_SetsAndIsIdempotent(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)
	for i, want := range []bool{true, true, false, false} { // set, not toggle
		r := f.tick(f.lo, item, f.aTok, map[string]any{"ticked": want})
		if r.code != http.StatusOK || !strings.Contains(r.body, fmt.Sprintf(`"ticked":%v`, want)) || !strings.Contains(r.body, fmt.Sprintf(`"loadout_item_id":%d`, item)) {
			t.Fatalf("call %d: %d %s", i, r.code, r.body)
		}
		if f.ticked(item) != want {
			t.Fatalf("call %d: stored ticked is not %v", i, want)
		}
	}
	f.tick(f.lo, item, f.aTok, map[string]any{"ticked": true})
	if got := f.items(); !got[0].Ticked {
		t.Error("list does not report ticked")
	}
}

func TestItemTick_Refusals(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)
	bobItem := f.env.seedItem(f.loBob, f.tentGear, 1)
	before := f.env.snapshot()
	for _, tc := range []struct {
		name         string
		loadout, itm int64
		tok          string
		body         any
		want         int
	}{
		{"unknown item", f.lo, 987654, f.aTok, map[string]any{"ticked": true}, 404},
		{"item of another loadout of mine", f.lo, bobItem, f.aTok, map[string]any{"ticked": true}, 404},
		{"foreign loadout", f.loBob, bobItem, f.aTok, map[string]any{"ticked": true}, 404},
		{"unknown loadout", 987654, item, f.aTok, map[string]any{"ticked": true}, 404},
		{"missing ticked", f.lo, item, f.aTok, map[string]any{}, 400},
		{"null ticked", f.lo, item, f.aTok, `{"ticked": null}`, 400},
		{"non-bool ticked", f.lo, item, f.aTok, `{"ticked": "yes"}`, 400},
		{"numeric ticked", f.lo, item, f.aTok, `{"ticked": 1}`, 400},
		{"no body", f.lo, item, f.aTok, nil, 400},
		{"unknown item beats bad body", f.lo, 987654, f.aTok, `{`, 404},
	} {
		if r := f.tick(tc.loadout, tc.itm, tc.tok, tc.body); r.code != tc.want {
			t.Errorf("%s: status %d, want %d: %s", tc.name, r.code, tc.want, r.body)
		}
	}
	if f.env.snapshot() != before {
		t.Fatal("refused ticks changed data")
	}
}

// A tick for an item that was removed meanwhile is 404 and touches nothing
// else (BR-025 AC-7).
func TestItemTick_RemovedItemChangesNothing(t *testing.T) {
	f := newItemFixture(t)
	gone := f.newItem(f.tent)
	kept := f.newItem(f.sleepRegLow)
	expectStatus(t, f.env.Do(http.MethodDelete, itemPath(f.lo, gone, "delete"), f.aTok, nil), http.StatusOK)
	before := f.env.snapshot()
	if r := f.tick(f.lo, gone, f.aTok, map[string]any{"ticked": true}); r.code != http.StatusNotFound {
		t.Fatalf("status %d: %s", r.code, r.body)
	}
	if f.env.snapshot() != before || f.ticked(kept) {
		t.Fatal("tick of a removed item changed data")
	}
}

// Moving or editing an item never changes ticked or closed (BR-025 AC-3, AC-4).
func TestItemUpdate_NeverChangesTickedOrClosed(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)
	f.tick(f.lo, item, f.aTok, map[string]any{"ticked": true})
	if _, err := f.env.DB.Exec(`UPDATE loadout_places SET closed = 1 WHERE placeId = ?`, f.bag); err != nil {
		t.Fatal(err)
	}
	for _, body := range []any{
		map[string]any{"place_id": f.bag}, map[string]any{"place_id": f.worn}, `{"place_id": null}`,
		map[string]any{"quantity": 3, "notes": "x", "ticked": false},
	} {
		if code, _, b := f.update(item, body); code != http.StatusOK {
			t.Fatalf("update %v: %d %s", body, code, b)
		}
		if !f.ticked(item) {
			t.Fatalf("update %v unticked the item", body)
		}
		if f.env.Count("loadout_places", "placeId = ? AND closed = 1", f.bag) != 1 {
			t.Fatalf("update %v reopened the bag", body)
		}
	}
	// Inserting into a closed bag does not reopen it either.
	if code, _, b := f.insert(map[string]any{"usergear_registration_id": f.sleepRegLow, "place_id": f.bag}); code != http.StatusCreated {
		t.Fatalf("insert into closed bag: %d %s", code, b)
	}
	if f.env.Count("loadout_places", "placeId = ? AND closed = 1", f.bag) != 1 {
		t.Fatal("insert reopened the bag")
	}
}

// ---- computed weights (BR-029 AC-3) ---------------------------------------------

func TestLoadoutTotal_OnBackWeightFollowsPlacesAndCatalog(t *testing.T) {
	f := newItemFixture(t)
	// pack 100 g (a bag of the loadout counts), tent 2 x 1000 g in the pack,
	// sleep 250 g worn (not counted), sleep 250 g loose.
	f.env.seedRegItem(f.lo, f.tent, f.bag, 2)
	f.env.seedRegItem(f.lo, f.sleepRegLow, f.worn, 1)
	f.env.seedRegItem(f.lo, f.sleepRegHigher, 0, 1)
	if got := f.loadoutTotal(f.lo, f.aTok); got != 100+2000+250 {
		t.Fatalf("total = %d, want 2350", got)
	}
	w := f.env.Do(http.MethodGet, "/api/v1/loadout/list", f.aTok, nil)
	expectStatus(t, w, http.StatusOK)
	var list []models.Loadout
	decodeJSON(t, w, &list)
	for _, l := range list {
		if *l.LoadoutID == f.lo && l.TotalWeight != 2350 {
			t.Errorf("list total = %d, want 2350", l.TotalWeight)
		}
	}
	if _, err := f.env.DB.Exec(`UPDATE gear SET gearWeight = 1100 WHERE gearId = ?`, f.tentGear); err != nil {
		t.Fatal(err)
	}
	if got := f.loadoutTotal(f.lo, f.aTok); got != 100+2200+250 {
		t.Fatalf("total after catalog edit = %d, want 2550", got)
	}
}

func TestItemRemoval_FixesTotalAndRefusalsKeepIt(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)
	if got := f.loadoutTotal(f.lo, f.aTok); got != 1100 {
		t.Fatalf("total = %d", got)
	}
	expectStatus(t, f.env.Do(http.MethodDelete, itemPath(f.lo, item, "delete"), f.bTok, nil), http.StatusNotFound)
	if got := f.loadoutTotal(f.lo, f.aTok); got != 1100 {
		t.Fatalf("refused delete changed the total: %d", got)
	}
	expectStatus(t, f.env.Do(http.MethodDelete, itemPath(f.lo, item, "delete"), f.aTok, nil), http.StatusOK)
	if got := f.loadoutTotal(f.lo, f.aTok); got != 100 {
		t.Fatalf("total after delete = %d, want the 100 g pack", got)
	}
}

// ---- deleted registrations keep their items (BR-030) as detached snapshots ---------

func TestItemList_RegistrationDeletedKeepsSnapshot(t *testing.T) {
	f := newItemFixture(t)
	item := f.newItem(f.tent)
	expectStatus(t, f.env.Do(http.MethodDelete, ugPath(f.tent, "delete"), f.aTok, nil), http.StatusOK)

	items := f.items()
	if len(items) != 1 || *items[0].LoadoutItemID != item {
		t.Fatalf("item lost: %+v", items)
	}
	it := items[0]
	if it.RegistrationExists || it.UsergearRegistrationID != nil || it.DetachedReason == nil || *it.DetachedReason != "registration_deleted" {
		t.Errorf("detached fields: %+v", it)
	}
	if it.GearName != "g" || it.ManufactureName != nil || it.CategoryName != nil || it.TopCategoryName != nil {
		t.Errorf("snapshot name and null maker/category expected: %+v", it)
	}
	if it.UnitWeight == nil || *it.UnitWeight != 1000 || it.WeightSource != "catalog" || it.CatalogWeight != nil {
		t.Errorf("snapshot weight: %+v", it)
	}
	if it.GearID == nil || *it.GearID != f.tentGear {
		t.Errorf("gear_id: %+v", it)
	}
	if got := f.loadoutTotal(f.lo, f.aTok); got != 1100 {
		t.Errorf("total after registration delete = %d, want the kept 1000 plus the pack", got)
	}
}

// ---- public items (BR-016 AC-5) ----------------------------------------------------

func TestPublicItems_GearNameAndCurrentGearFilter(t *testing.T) {
	f := newItemFixture(t)
	if _, err := f.env.DB.Exec(`UPDATE loadouts SET loadoutIsPublic = 1 WHERE loadoutId = ?`, f.lo); err != nil {
		t.Fatal(err)
	}
	f.newItem(f.tent)                                        // registration-backed
	f.insert(map[string]any{"gear_id": f.env.seedGearW(10)}) // legacy
	gone := f.newItem(f.sleepRegLow)                         // registration deleted, gear stays
	expectStatus(t, f.env.Do(http.MethodDelete, ugPath(f.sleepRegLow, "delete"), f.aTok, nil), http.StatusOK)
	// Orphan: a deleted registration whose gear record no longer exists.
	if _, err := f.env.DB.Exec(`INSERT INTO loadout_items (loadoutId, detachedReason, detachedGearName, detachedUnitWeight, detachedWeightSource)
		VALUES (?, 'registration_deleted', 'Ghost', 77, 'weighed')`, f.lo); err != nil {
		t.Fatal(err)
	}

	w := f.env.Do(http.MethodGet, "/api/v1/public/loadout/alice-lo", "", nil)
	expectStatus(t, w, http.StatusOK)
	var loadout models.LoadoutPublic
	decodeJSON(t, w, &loadout)
	if loadout.TotalWeight != 1000+10+250+77+100 {
		t.Errorf("public total = %d, want 1437 (hidden items still count)", loadout.TotalWeight)
	}

	w = f.env.Do(http.MethodGet, "/api/v1/public/loadout/alice-lo/items", "", nil)
	expectStatus(t, w, http.StatusOK)
	var items []models.LoadoutItemPublic
	decodeJSON(t, w, &items)
	if len(items) != 3 {
		t.Fatalf("%d public items, want 3 (the orphan is not listed): %s", len(items), w.Body)
	}
	for _, it := range items {
		if it.GearName != "g" || it.LoadoutID != f.lo || it.Quantity != 1 {
			t.Errorf("public item: %+v", it)
		}
	}
	if *items[2].LoadoutItemID != gone || items[2].GearID != f.sleepGear {
		t.Errorf("kept item of a deleted registration: %+v", items[2])
	}
	for _, field := range []string{"loadout_item_id", "loadout_id", "gear_id", "quantity", "notes", "gear_name"} {
		if !strings.Contains(w.Body.String(), `"`+field+`"`) {
			t.Errorf("public item lacks %s", field)
		}
	}
	if strings.Contains(w.Body.String(), "Ghost") || strings.Contains(w.Body.String(), "registration") {
		t.Errorf("public item leaks private detail: %s", w.Body)
	}
}
