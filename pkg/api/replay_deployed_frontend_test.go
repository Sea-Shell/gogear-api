package endpoints

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// Replay of the deployed frontend (ADR-004 D-1, D-2; ARD-001 merge rule).
//
// The requests below are what gogear-frontend at aabc27d sends, with the same
// methods, paths, query strings and bodies, and the response fields it reads.
// They run against the new API. The suite must stay green for every milestone
// that is merged while that frontend is deployed; it is re-run at each
// integration. Responses are decoded into untyped maps, so a renamed field or a
// changed JSON type fails here, not only a changed Go struct.
//
// Inventory (src/ at aabc27d; request = src/api/endpoints.ts):
//
//	LoadoutApi.insert      PUT    /loadout/insert                 LoadoutFormPage.tsx:84
//	LoadoutApi.list        GET    /loadout/list                   LoadoutListPage.tsx:95, DashboardPage.tsx:29 (reads total_weight, created_at)
//	LoadoutApi.get         GET    /loadout/:id/get                LoadoutFormPage.tsx:38, MobileChecklistPage.tsx:183, PackTree.tsx:68
//	LoadoutApi.update      POST   /loadout/:id/update             LoadoutFormPage.tsx:73
//	LoadoutApi.itemInsert  PUT    /loadout/:id/item/insert        LoadoutWorkspace.tsx:70, MobileChecklistPage.tsx:250 ({loadout_id, gear_id, quantity, notes})
//	LoadoutApi.itemList    GET    /loadout/:id/item/list          LoadoutWorkspace.tsx:32, MobileChecklistPage.tsx:189, PackTree.tsx:75
//	LoadoutApi.itemUpdate  POST   /loadout/:id/item/:item/update  LoadoutWorkspace.tsx:89, MobileChecklistPage.tsx:272 ({loadout_item_id, quantity, notes})
//	LoadoutApi.itemRemove  DELETE /loadout/:id/item/:item/delete  LoadoutWorkspace.tsx:100, MobileChecklistPage.tsx:283, PackTree.tsx:118
//	LoadoutApi.remove      DELETE /loadout/:id/delete             LoadoutListPage.tsx:99, PackTree.tsx:126
//	LoadoutApi.getPublic   GET    /public/loadout/:slug           PublicLoadoutPage.tsx:13 (reads total_weight)
//	LoadoutApi.itemsPublic GET    /public/loadout/:slug/items     PublicLoadoutPage.tsx:19 (reads quantity and the item's gear)
//	GearApi.list           GET    /gear/list?limit=200 | ?limit=1 MobileChecklistPage.tsx:195, useGearList.ts:7, DashboardPage.tsx:34 (reads gear_weight via ?? 0)
//	GearApi.list           GET    /gear/list?page&limit&category& GearPage.tsx:38, CategoriesPage.tsx:263 (reads gear_weight != null, GearPage.tsx:212)
//	GearApi.search/get     GET    /gear/search, /gear/:id/get     declared in endpoints.ts; no call site at aabc27d, replayed because milestone A changes their output
//	GearApi.remove         DELETE /gear/:id/delete                GearPage.tsx:57 (admin)
//	UserGearApi.listByUser GET    /usergear/:user/list?page&limit UserGearPage.tsx:245 (reads gear_weight via ensureWeight, container_*, max_container_weight)
//	UserGearApi.get        GET    /usergear/registration/:id/get  declared in endpoints.ts; no call site
//	UserGearApi.update     POST   /usergear/registration/:id/update UserGearPage.tsx:589
//	UserGearApi.remove     DELETE /usergear/registration/:id/delete UserGearPage.tsx:551
//	ContainerApi.insert    PUT    /container/insert               UserGearPage.tsx:991
//	ContainerApi.remove    DELETE /container/:link/delete         UserGearPage.tsx:524
//
// Not replayed: /auth/*, /health, /users/*, /category/*, /topCategory/*,
// /manufacture/* (no milestone-A change; their own suites cover them).

type replay struct {
	f *itemFixture
	t *testing.T
}

func (r *replay) do(method, path, tok string, body any, want int) map[string]any {
	r.t.Helper()
	w := r.f.env.Do(method, path, tok, body)
	expectStatus(r.t, w, want)
	var m map[string]any
	if w.Body.Len() > 0 {
		decodeJSON(r.t, w, &m)
	}
	return m
}

func (r *replay) doList(method, path, tok string, want int) []map[string]any {
	r.t.Helper()
	w := r.f.env.Do(method, path, tok, nil)
	expectStatus(r.t, w, want)
	var l []map[string]any
	decodeJSON(r.t, w, &l)
	return l
}

// payloadItems returns the items of a ResponsePayload.
func (r *replay) payloadItems(path, tok string) []map[string]any {
	r.t.Helper()
	m := r.do(http.MethodGet, path, tok, nil, http.StatusOK)
	raw, ok := m["items"].([]any)
	if !ok {
		r.t.Fatalf("%s: items is not an array: %v", path, m)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, x := range raw {
		out = append(out, x.(map[string]any))
	}
	return out
}

// need asserts that m has key with a JSON number and returns it.
func (r *replay) num(m map[string]any, key string) float64 {
	r.t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		r.t.Fatalf("%s is %T (%v), want a number; object: %v", key, m[key], m[key], m)
	}
	return v
}

func (r *replay) str(m map[string]any, key string) string {
	r.t.Helper()
	v, ok := m[key].(string)
	if !ok {
		r.t.Fatalf("%s is %T (%v), want a string; object: %v", key, m[key], m[key], m)
	}
	return v
}

// weight asserts the key exists and is a number or null (the frontend reads it
// through ?? 0 and != null), and reports whether it is null.
func (r *replay) weightIsNull(m map[string]any, key string) bool {
	r.t.Helper()
	v, present := m[key]
	if !present {
		r.t.Fatalf("%s missing from %v", key, m)
	}
	if v == nil {
		return true
	}
	if _, ok := v.(float64); !ok {
		r.t.Fatalf("%s is %T, want number or null", key, v)
	}
	return false
}

func byID(rows []map[string]any, key string, id int64) map[string]any {
	for _, m := range rows {
		if v, ok := m[key].(float64); ok && int64(v) == id {
			return m
		}
	}
	return nil
}

func TestReplayDeployedFrontend(t *testing.T) {
	f := newItemFixture(t)
	r := &replay{f: f, t: t}
	env := f.env
	admin := env.SeedUser("root", "root@example.com", true)
	adminTok := env.Token(admin, true)

	// Seed: a 395 g gear and a 0 g gear (ADR-004 D-1), both registered by
	// alice; a gear only a legacy item will use (alice has no registration).
	g395 := env.seedGearW(395)
	reg395 := env.SeedRegistration(f.alice, g395)
	legacyGear := env.seedGearW(400)

	// --- LoadoutFormPage: create, as the form sends it ---------------------
	lo := r.do(http.MethodPut, "/api/v1/loadout/insert", f.aTok, map[string]any{
		"loadout_name": "Replay", "loadout_description": "d", "loadout_is_public": true,
		"loadout_slug": "replay-lo", "user_id": f.alice,
	}, http.StatusCreated)
	loID := int64(r.num(lo, "loadout_id"))
	for _, k := range []string{"loadout_name", "loadout_description", "loadout_slug", "created_at", "updated_at"} {
		r.str(lo, k)
	}
	r.num(lo, "total_weight")
	if lo["loadout_is_public"] != true || int64(r.num(lo, "user_id")) != f.alice {
		t.Errorf("created loadout: %v", lo)
	}

	// --- Add items by gear_id, as LoadoutWorkspace and MobileChecklistPage do.
	// 395 g and 0 g gear are registered by alice; legacyGear is not.
	itemIn := func(gear int64, qty int, notes string) int64 {
		it := r.do(http.MethodPut, loPath(loID, "item/insert"), f.aTok, map[string]any{
			"loadout_id": loID, "gear_id": gear, "quantity": qty, "notes": notes,
		}, http.StatusCreated)
		if int64(r.num(it, "gear_id")) != gear || int64(r.num(it, "loadout_id")) != loID ||
			int(r.num(it, "quantity")) != qty || r.str(it, "notes") != notes {
			t.Errorf("insert response for gear %d: %v", gear, it)
		}
		return int64(r.num(it, "loadout_item_id"))
	}
	i395 := itemIn(g395, 2, "")
	iZero := itemIn(f.zeroGear, 1, "weightless")
	iLegacy := itemIn(legacyGear, 1, "legacy")

	// --- Item list: every field LoadoutItem declares -----------------------
	items := r.doList(http.MethodGet, loPath(loID, "item/list"), f.aTok, http.StatusOK)
	if len(items) != 3 {
		t.Fatalf("item list has %d items, want 3: %v", len(items), items)
	}
	for id, gear := range map[int64]int64{i395: g395, iZero: f.zeroGear, iLegacy: legacyGear} {
		it := byID(items, "loadout_item_id", id)
		if it == nil {
			t.Fatalf("item %d missing from list", id)
		}
		if int64(r.num(it, "gear_id")) != gear || int64(r.num(it, "loadout_id")) != loID {
			t.Errorf("item %d: %v", id, it)
		}
		r.num(it, "quantity")
		r.str(it, "notes")
	}

	// --- Item update, as the workspace sends it ----------------------------
	r.do(http.MethodPost, itemPath(loID, i395, "update"), f.aTok,
		map[string]any{"loadout_item_id": i395, "quantity": 3, "notes": "tent fly"}, http.StatusOK)
	it := byID(r.doList(http.MethodGet, loPath(loID, "item/list"), f.aTok, http.StatusOK), "loadout_item_id", i395)
	if int(r.num(it, "quantity")) != 3 || r.str(it, "notes") != "tent fly" {
		t.Errorf("updated item: %v", it)
	}

	// --- Loadout reads: total_weight is a number, and follows the items ----
	// 395*3 + 0 + 400 = 1585, as the frontend's own sum of gear_weight * quantity.
	wantTotal := float64(395*3 + 0 + 400)
	lists := r.doList(http.MethodGet, "/api/v1/loadout/list", f.aTok, http.StatusOK)
	l := byID(lists, "loadout_id", loID)
	if l == nil || r.num(l, "total_weight") != wantTotal {
		t.Errorf("loadout list entry = %v, want total_weight %v", l, wantTotal)
	}
	for _, k := range []string{"loadout_name", "loadout_description", "loadout_slug", "created_at", "updated_at"} {
		r.str(l, k)
	}
	got := r.do(http.MethodGet, loPath(loID, "get"), f.aTok, nil, http.StatusOK)
	if r.num(got, "total_weight") != wantTotal || got["loadout_is_public"] != true {
		t.Errorf("loadout get = %v", got)
	}

	// --- Loadout update from the form --------------------------------------
	r.do(http.MethodPost, loPath(loID, "update"), f.aTok, map[string]any{
		"loadout_id": loID, "loadout_name": "Replay 2", "loadout_description": "d2",
		"loadout_is_public": true, "loadout_slug": "replay-lo",
	}, http.StatusOK)
	if got := r.do(http.MethodGet, loPath(loID, "get"), f.aTok, nil, http.StatusOK); r.str(got, "loadout_name") != "Replay 2" {
		t.Errorf("loadout update not stored: %v", got)
	}

	// --- Public loadout, no token ------------------------------------------
	pub := r.do(http.MethodGet, "/api/v1/public/loadout/replay-lo", "", nil, http.StatusOK)
	if r.num(pub, "total_weight") != wantTotal || int64(r.num(pub, "loadout_id")) != loID {
		t.Errorf("public loadout: %v", pub)
	}
	for _, k := range []string{"loadout_name", "loadout_description", "loadout_slug", "created_at", "updated_at"} {
		r.str(pub, k)
	}
	pubItems := r.doList(http.MethodGet, "/api/v1/public/loadout/replay-lo/items", "", http.StatusOK)
	if len(pubItems) != 3 {
		t.Fatalf("public items = %d, want 3: %v", len(pubItems), pubItems)
	}
	for _, p := range pubItems {
		r.num(p, "loadout_item_id")
		r.num(p, "gear_id")
		r.num(p, "quantity")
		r.str(p, "notes")
	}

	// --- Catalog list: gear_weight is a number, null for the 0 g gear ------
	for _, q := range []string{"limit=200", "limit=1", "page=1&limit=30", "limit=30&page=1&category=1&topCategory=1"} {
		rows := r.payloadItems("/api/v1/gear/list?"+q, f.aTok)
		if len(rows) == 0 {
			t.Fatalf("gear list ?%s is empty", q)
		}
		for _, row := range rows {
			r.num(row, "gear_id")
			r.str(row, "gear_name")
			r.weightIsNull(row, "gear_weight")
			r.str(row, "category_name")
			r.str(row, "top_category_name")
			r.str(row, "manufacture_name")
			if _, ok := row["gear_is_container"].(bool); !ok {
				t.Errorf("gear_is_container not a bool: %v", row)
			}
		}
	}
	all := r.payloadItems("/api/v1/gear/list?limit=200", f.aTok)
	if row := byID(all, "gear_id", g395); row == nil || r.num(row, "gear_weight") != 395 {
		t.Errorf("395 g gear = %v", row)
	}
	if row := byID(all, "gear_id", f.zeroGear); row == nil || !r.weightIsNull(row, "gear_weight") {
		t.Errorf("0 g gear must report gear_weight null: %v", row)
	}
	search := url.Values{"searchString": {"g"}, "searchType": {"contains"}}.Encode()
	if row := byID(r.payloadItems("/api/v1/gear/search?"+search, f.aTok), "gear_id", f.zeroGear); row == nil || !r.weightIsNull(row, "gear_weight") {
		t.Errorf("search: 0 g gear must report gear_weight null: %v", row)
	}
	full := r.do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", f.zeroGear), f.aTok, nil, http.StatusOK)
	if !r.weightIsNull(full, "gear_weight") || r.str(full, "gear_name") == "" {
		t.Errorf("gear get: %v", full)
	}
	full = r.do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", g395), f.aTok, nil, http.StatusOK)
	if r.num(full, "gear_weight") != 395 {
		t.Errorf("gear get 395 g: %v", full)
	}

	// --- UserGearPage: inventory list, update, get, container links --------
	regs := r.payloadItems(fmt.Sprintf("/api/v1/usergear/%d/list?page=1&limit=50", f.alice), f.aTok)
	if row := byID(regs, "usergear_registration_id", f.zeroReg); row == nil || !r.weightIsNull(row, "gear_weight") {
		t.Errorf("registration of 0 g gear: %v", row)
	}
	row := byID(regs, "usergear_registration_id", reg395)
	if row == nil || r.num(row, "gear_weight") != 395 {
		t.Fatalf("registration of 395 g gear: %v", row)
	}
	for _, k := range []string{"usergear_gear_id", "usergear_user_id", "gear_id", "user_id"} {
		r.num(row, k)
	}
	for _, k := range []string{"gear_name", "manufacture_name", "category_name", "top_category_name"} {
		r.str(row, k)
	}
	if _, ok := row["gear_is_container"].(bool); !ok {
		t.Errorf("gear_is_container: %v", row)
	}
	for _, k := range []string{"container_link_id", "container_registration_id", "max_container_weight"} {
		if v, present := row[k]; !present || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, present)
		}
	}
	r.do(http.MethodPost, ugPath(f.pack, "update"), f.aTok, map[string]any{
		"usergear_registration_id": f.pack, "usergear_gear_id": f.packGear, "usergear_user_id": f.alice,
		"max_container_weight": 5000,
	}, http.StatusOK)
	one := r.do(http.MethodGet, ugPath(f.pack, "get"), f.aTok, nil, http.StatusOK)
	if r.num(one, "max_container_weight") != 5000 || r.num(one, "gear_weight") != 100 {
		t.Errorf("registration get: %v", one)
	}
	r.do(http.MethodPut, "/api/v1/container/insert", f.aTok,
		map[string]any{"user_container_id": f.pack, "user_gear_registration_id": reg395}, http.StatusOK)
	regs = r.payloadItems(fmt.Sprintf("/api/v1/usergear/%d/list", f.alice), f.aTok)
	row = byID(regs, "usergear_registration_id", reg395)
	if row == nil || int64(r.num(row, "container_registration_id")) != f.pack {
		t.Fatalf("container link not listed: %v", row)
	}
	link := int64(r.num(row, "container_link_id"))
	r.do(http.MethodDelete, fmt.Sprintf("/api/v1/container/%d/delete", link), f.aTok, nil, http.StatusOK)

	// --- Delete refused: gear used only by a legacy item (ADR-004 D-2) -----
	before := env.snapshot()
	r.do(http.MethodDelete, fmt.Sprintf("/api/v1/gear/%d/delete", legacyGear), adminTok, nil, http.StatusConflict)
	if env.snapshot() != before || env.Count("gear", "gearId = ?", legacyGear) != 1 {
		t.Error("a refused gear delete changed stored data")
	}
	items = r.doList(http.MethodGet, loPath(loID, "item/list"), f.aTok, http.StatusOK)
	if byID(items, "loadout_item_id", iLegacy) == nil {
		t.Error("legacy item vanished after a refused delete")
	}

	// --- Remove a registration: the loadout keeps the item (BR-030) --------
	r.do(http.MethodDelete, ugPath(reg395, "delete"), f.aTok, nil, http.StatusOK)
	items = r.doList(http.MethodGet, loPath(loID, "item/list"), f.aTok, http.StatusOK)
	if len(items) != 3 || byID(items, "loadout_item_id", i395) == nil {
		t.Errorf("items after deleting a registration: %v", items)
	}
	if got := r.do(http.MethodGet, loPath(loID, "get"), f.aTok, nil, http.StatusOK); r.num(got, "total_weight") != wantTotal {
		t.Errorf("total after deleting the registration = %v, want %v", got["total_weight"], wantTotal)
	}

	// --- Removal: item delete, then loadout delete --------------------------
	r.do(http.MethodDelete, itemPath(loID, iZero, "delete"), f.aTok, nil, http.StatusOK)
	if n := len(r.doList(http.MethodGet, loPath(loID, "item/list"), f.aTok, http.StatusOK)); n != 2 {
		t.Errorf("%d items after item delete, want 2", n)
	}
	r.do(http.MethodDelete, loPath(loID, "delete"), f.aTok, nil, http.StatusOK)
	expectStatus(t, env.Do(http.MethodGet, loPath(loID, "get"), f.aTok, nil), http.StatusNotFound)
}

// BR-029 AC-5, AC-6 (milestone A parts), ADR-003 D-7: a catalog weight of 0 or
// none is reported as null in every gear read, 395 g as 395.
func TestGearWeightReportedNullWhenZeroOrMissing(t *testing.T) {
	env := newAuthzEnv(t)
	user := env.SeedUser("alice", "alice@example.com", false)
	tok := env.Token(user, false)
	r := &replay{f: &itemFixture{env: env}, t: t}

	weights := map[string]any{"w395": 395, "w0": 0, "wnull": nil}
	ids := map[string]int64{}
	for name, w := range weights {
		res, err := env.DB.Exec(`INSERT INTO gear (gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight, gearHeight, gearLength, gearWidth, gearStatus, gearSizeDefinition)
			VALUES (1, 1, 1, ?, ?, 0, 0, 0, 1, '')`, name, w)
		if err != nil {
			t.Fatal(err)
		}
		ids[name], _ = res.LastInsertId()
	}
	regs := map[string]int64{}
	for name, id := range ids {
		regs[name] = env.SeedRegistration(user, id)
	}

	check := func(read string, row map[string]any, name string) {
		t.Helper()
		if row == nil {
			t.Fatalf("%s: %s missing", read, name)
		}
		if name == "w395" {
			if r.num(row, "gear_weight") != 395 {
				t.Errorf("%s: %s = %v, want 395", read, name, row["gear_weight"])
			}
		} else if !r.weightIsNull(row, "gear_weight") {
			t.Errorf("%s: %s = %v, want null", read, name, row["gear_weight"])
		}
	}
	list := r.payloadItems("/api/v1/gear/list?limit=50", tok)
	search := r.payloadItems("/api/v1/gear/search?searchString=w&searchType=startswith", tok)
	mine := r.payloadItems(fmt.Sprintf("/api/v1/usergear/%d/list", user), tok)
	for name, id := range ids {
		check("gear list", byID(list, "gear_id", id), name)
		check("gear search", byID(search, "gear_id", id), name)
		check("gear get", r.do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", id), tok, nil, http.StatusOK), name)
		check("registration list", byID(mine, "usergear_registration_id", regs[name]), name)
		check("registration get", r.do(http.MethodGet, ugPath(regs[name], "get"), tok, nil, http.StatusOK), name)
	}
}
