package endpoints

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests for the admin API of the container settings (BR-021 AC-1, AC-2,
// BR-001 AC-6, ADR-001 D-2) and for E2E-001, which builds the whole D11
// behavior through the real routes and the real JWT middleware. No setting is
// written with SQL here.

type apiWorld struct {
	env        *authzEnv
	admin      string
	user       string
	userID     int64
	adminID    int64
	catalogIDs map[string]int64
}

func newAPIWorld(t *testing.T) *apiWorld {
	env := newAuthzEnv(t)
	w := &apiWorld{env: env, catalogIDs: map[string]int64{}}
	w.adminID = env.SeedUser("root", "root@example.com", true)
	w.userID = env.SeedUser("alice", "alice@example.com", false)
	w.admin, w.user = env.Token(w.adminID, true), env.Token(w.userID, false)
	return w
}

func (w *apiWorld) idOf(r *httptest.ResponseRecorder, key string) int64 {
	w.env.T.Helper()
	expectStatus(w.env.T, r, http.StatusOK)
	var m map[string]any
	decodeJSON(w.env.T, r, &m)
	f, ok := m[key].(float64)
	if !ok {
		w.env.T.Fatalf("response has no %s: %s", key, r.Body.String())
	}
	return int64(f)
}

// topInsert creates a top category through the API and returns its ID. The
// insert answers only {"status":"success"}, so the ID is found in the list.
func (w *apiWorld) topInsert(name string, body map[string]any) int64 {
	w.env.T.Helper()
	body["top_category_name"] = name
	expectStatus(w.env.T, w.env.Do(http.MethodPut, "/api/v1/topCategory/insert", w.admin, body), http.StatusOK)
	r := w.env.Do(http.MethodGet, "/api/v1/topCategory/list?limit=100", w.user, nil)
	expectStatus(w.env.T, r, http.StatusOK)
	items, _ := page(w.env.T, r)
	for _, it := range items {
		if it["top_category_name"] == name {
			id := int64(it["top_category_id"].(float64))
			w.catalogIDs[name] = id
			return id
		}
	}
	w.env.T.Fatalf("top category %q not in the list", name)
	return 0
}

func (w *apiWorld) catInsert(name string, top int64, body map[string]any) *httptest.ResponseRecorder {
	w.env.T.Helper()
	body["category_name"], body["category_top_category_id"] = name, top
	return w.env.Do(http.MethodPut, "/api/v1/category/insert", w.admin, body)
}

func (w *apiWorld) catGet(id int64) map[string]any {
	w.env.T.Helper()
	r := w.env.Do(http.MethodGet, fmt.Sprintf("/api/v1/category/%d/get", id), w.user, nil)
	expectStatus(w.env.T, r, http.StatusOK)
	var m map[string]any
	decodeJSON(w.env.T, r, &m)
	return m
}

func (w *apiWorld) topGet(id int64) map[string]any {
	w.env.T.Helper()
	r := w.env.Do(http.MethodGet, fmt.Sprintf("/api/v1/topCategory/%d/get", id), w.user, nil)
	expectStatus(w.env.T, r, http.StatusOK)
	var m map[string]any
	decodeJSON(w.env.T, r, &m)
	return m
}

func (w *apiWorld) gearInsert(name string, top, cat int64) int64 {
	w.env.T.Helper()
	r := w.env.Do(http.MethodPut, "/api/v1/gear/insert", w.admin, map[string]any{
		"gear_top_category_id": top, "gear_category_id": cat, "gear_manufacture_id": 1,
		"gear_name": name, "gear_weight": 100, "gear_height": 1, "gear_length": 2, "gear_width": 3, "gear_status": true,
	})
	return w.idOf(r, "gear_id")
}

// register registers catalog gear for the user through the API and returns the
// registration ID, found in the user's list.
func (w *apiWorld) register(gear int64) int64 {
	w.env.T.Helper()
	expectStatus(w.env.T, w.env.Do(http.MethodPut, "/api/v1/usergear/insert", w.user,
		map[string]any{"usergear_gear_id": gear}), http.StatusOK)
	r := w.env.Do(http.MethodGet, ugList(w.userID, "limit=100"), w.user, nil)
	expectStatus(w.env.T, r, http.StatusOK)
	items, _ := page(w.env.T, r)
	var id int64
	for _, it := range items {
		if int64(it["gear_id"].(float64)) == gear {
			id = int64(it["usergear_registration_id"].(float64))
		}
	}
	if id == 0 {
		w.env.T.Fatalf("no registration of gear %d in the list", gear)
	}
	return id
}

func (w *apiWorld) link(container, content int64) *httptest.ResponseRecorder {
	return w.env.Do(http.MethodPut, "/api/v1/container/insert", w.user,
		map[string]any{"user_container_id": container, "user_gear_registration_id": content})
}

// E2E-001: BR-021 AC-1 (three examples), AC-2, AC-3, AC-6, AC-7 and BR-020
// AC-1 through the admin and user API only.
//
//	Bags (container)      > Backpacks (no setting), Bag straps (not_container)
//	Clothing (not)        > Stuff sacks (container)
func TestE2E001_CategoryDrivenContainersThroughTheAPI(t *testing.T) {
	w := newAPIWorld(t)
	e := w.env

	bags := w.topInsert("Bags", map[string]any{"top_category_is_container": true})
	clothing := w.topInsert("Clothing", map[string]any{})
	if w.topGet(bags)["top_category_is_container"] != true || w.topGet(clothing)["top_category_is_container"] != false {
		t.Fatalf("top category settings: Bags=%v Clothing=%v", w.topGet(bags), w.topGet(clothing))
	}

	backpacks := w.idOf(w.catInsert("Backpacks", bags, map[string]any{}), "category_id")
	straps := w.idOf(w.catInsert("Bag straps", bags, map[string]any{"category_container": "not_container"}), "category_id")
	sacks := w.idOf(w.catInsert("Stuff sacks", clothing, map[string]any{"category_container": "container"}), "category_id")

	for _, c := range []struct {
		id        int64
		container string
		effective bool
	}{{backpacks, "inherit", true}, {straps, "not_container", false}, {sacks, "container", true}} {
		got := w.catGet(c.id)
		if got["category_container"] != c.container || got["category_is_container"] != c.effective {
			t.Errorf("category %d reads %v/%v, want %s/%v", c.id, got["category_container"], got["category_is_container"], c.container, c.effective)
		}
	}

	pack := w.gearInsert("e2e-pack", bags, backpacks)
	strap := w.gearInsert("e2e-strap", bags, straps)
	sack := w.gearInsert("e2e-sack", clothing, sacks)
	// A gear whose own top-category reference (Clothing, not a container)
	// disagrees with its sub-category (Backpacks, inherits Bags) is capable.
	oddPack := w.gearInsert("e2e-odd-pack", clothing, backpacks)

	want := map[int64]bool{pack: true, strap: false, sack: true, oddPack: true}
	checkGear := func(label string) {
		t.Helper()
		for g, v := range want {
			r := e.Do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", g), w.user, nil)
			expectStatus(t, r, http.StatusOK)
			var m map[string]any
			decodeJSON(t, r, &m)
			if m["gear_is_container"] != v {
				t.Errorf("%s: gear %d reports %v, want %v", label, g, m["gear_is_container"], v)
			}
		}
		for _, filter := range []bool{true, false} {
			r := e.Do(http.MethodGet, fmt.Sprintf("/api/v1/gear/list?limit=100&container=%v", filter), w.user, nil)
			expectStatus(t, r, http.StatusOK)
			items, total := page(t, r)
			got := map[int64]bool{}
			for _, id := range gearIDsOf(items) {
				got[id] = true
			}
			if total != len(items) {
				t.Errorf("%s: container=%v total %d != %d items", label, filter, total, len(items))
			}
			for g, v := range want {
				if got[g] != (v == filter) {
					t.Errorf("%s: container=%v lists gear %d = %v, want %v", label, filter, g, got[g], v == filter)
				}
			}
		}
	}
	checkGear("after building the catalog")

	// The user registers the gear and links through the API.
	rPack, rStrap, rSack, rOdd := w.register(pack), w.register(strap), w.register(sack), w.register(oddPack)
	expectStatus(t, w.link(rPack, rSack), http.StatusOK)
	expectStatus(t, w.link(rOdd, rStrap), http.StatusOK)
	r := w.link(rStrap, w.register(w.gearInsert("e2e-extra", bags, backpacks)))
	expectStatus(t, r, http.StatusConflict)
	if !strings.Contains(r.Body.String(), cannotHold) {
		t.Errorf("link into Bag straps gear: %s, want the %q refusal", r.Body.String(), cannotHold)
	}

	// The registration list filter follows the derived status as well.
	r = e.Do(http.MethodGet, ugList(w.userID, "limit=100&container=false"), w.user, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ := page(t, r)
	for _, it := range items {
		if int64(it["gear_id"].(float64)) != strap {
			t.Errorf("registration container=false lists gear %v, want only the strap", it["gear_id"])
		}
	}

	// AC-6: the admin flips Backpacks to not_container through the API.
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", backpacks), w.admin,
		map[string]any{"category_container": "not_container"}), http.StatusOK)
	want[pack], want[oddPack] = false, false
	checkGear("after Backpacks became not_container")

	r = e.Do(http.MethodGet, ctList(rPack, "limit=100"), w.user, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ = page(t, r)
	if len(items) != 1 {
		t.Fatalf("existing link not listed after the container lost capability: %s", r.Body.String())
	}
	linkID := int64(items[0]["container_link_id"].(float64))
	r = w.link(rPack, rStrap)
	expectStatus(t, r, http.StatusConflict)
	if !strings.Contains(r.Body.String(), cannotHold) {
		t.Errorf("new link into the now not-capable pack: %s", r.Body.String())
	}
	expectStatus(t, e.Do(http.MethodDelete, fmt.Sprintf("/api/v1/container/%d/delete", linkID), w.user, nil), http.StatusOK)
	if e.Count("user_container_registration", "userContainerId = ?", rPack) != 0 {
		t.Error("the existing link was not removable")
	}

	// Removing the explicit setting ("inherit") falls back to Bags (container).
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", backpacks), w.admin,
		map[string]any{"category_container": "inherit"}), http.StatusOK)
	want[pack], want[oddPack] = true, true
	checkGear("after the explicit setting was removed")
	expectStatus(t, w.link(rPack, rSack), http.StatusOK)

	// The top category's setting changes every inheriting sub-category.
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/topCategory/%d/update", bags), w.admin,
		map[string]any{"top_category_is_container": false}), http.StatusOK)
	want[pack], want[oddPack] = false, false
	checkGear("after Bags became not a container")

	// A sub-category moves to another top category (Clothing: not a container).
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/topCategory/%d/update", bags), w.admin,
		map[string]any{"top_category_is_container": true}), http.StatusOK)
	want[pack], want[oddPack] = true, true
	checkGear("after Bags is a container again")
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", backpacks), w.admin,
		map[string]any{"category_top_category_id": clothing}), http.StatusOK)
	want[pack], want[oddPack] = false, false
	checkGear("after Backpacks moved to Clothing")

	// A gear moves to another sub-category.
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", strap), w.admin,
		map[string]any{"gear_category_id": sacks}), http.StatusOK)
	want[strap] = true
	checkGear("after the strap gear moved to Stuff sacks")
	expectStatus(t, w.link(rStrap, rPack), http.StatusOK)
}

// BR-021 AC-2, BR-001 AC-6: only admins write the settings; everyone
// authenticated reads them; a refused request changes nothing.
func TestCategoryContainerSettings_AdminOnlyWrites(t *testing.T) {
	w := newAPIWorld(t)
	e := w.env
	top := w.topInsert("Bags", map[string]any{"top_category_is_container": true})
	cat := w.idOf(w.catInsert("Packs", top, map[string]any{"category_container": "not_container"}), "category_id")
	before := e.catalogSnapshot()

	writes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, fmt.Sprintf("/api/v1/topCategory/%d/update", top), map[string]any{"top_category_is_container": false}},
		{http.MethodPut, "/api/v1/topCategory/insert", map[string]any{"top_category_name": "X", "top_category_is_container": true}},
		{http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", cat), map[string]any{"category_container": "container"}},
		{http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", cat), map[string]any{"category_container": "inherit"}},
		{http.MethodPut, "/api/v1/category/insert", map[string]any{"category_name": "Y", "category_top_category_id": top, "category_container": "container"}},
	}
	for _, c := range writes {
		expectStatus(t, e.Do(c.method, c.path, w.user, c.body), http.StatusForbidden)
		expectStatus(t, e.Do(c.method, c.path, "", c.body), http.StatusUnauthorized)
		expectStatus(t, e.Do(c.method, c.path, "garbage", c.body), http.StatusUnauthorized)
	}
	if after := e.catalogSnapshot(); after != before {
		t.Errorf("a refused request changed the catalog:\nbefore: %s\nafter:  %s", before, after)
	}

	// Reads are open to every authenticated user and carry the settings.
	if got := w.topGet(top); got["top_category_is_container"] != true {
		t.Errorf("top get: %v", got)
	}
	if got := w.catGet(cat); got["category_container"] != "not_container" || got["category_is_container"] != false {
		t.Errorf("category get: %v", got)
	}
	r := e.Do(http.MethodGet, "/api/v1/category/list?limit=100", w.user, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ := page(t, r)
	var seen bool
	for _, it := range items {
		if int64(it["category_id"].(float64)) == cat {
			seen = true
			if it["category_container"] != "not_container" || it["category_is_container"] != false || it["top_category_is_container"] != true {
				t.Errorf("category list item: %v", it)
			}
		}
	}
	if !seen {
		t.Error("category missing from the list")
	}
	r = e.Do(http.MethodGet, "/api/v1/topCategory/list?limit=100", w.user, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ = page(t, r)
	for _, it := range items {
		if int64(it["top_category_id"].(float64)) == top && it["top_category_is_container"] != true {
			t.Errorf("top category list item: %v", it)
		}
	}
	expectStatus(t, e.Do(http.MethodGet, fmt.Sprintf("/api/v1/category/%d/get", cat), "", nil), http.StatusUnauthorized)
}

// ADR-001 D-2: null means "not sent", "inherit" removes, other values are 400
// and change nothing, read-only fields do not count as updatable.
func TestCategoryContainerSettings_RepresentationRules(t *testing.T) {
	w := newAPIWorld(t)
	e := w.env
	top := w.topInsert("Bags", map[string]any{"top_category_is_container": nil}) // null on create: false
	if w.topGet(top)["top_category_is_container"] != false {
		t.Fatalf("null on create must be false: %v", w.topGet(top))
	}
	cat := w.idOf(w.catInsert("Packs", top, map[string]any{"category_container": nil}), "category_id")
	if got := w.catGet(cat); got["category_container"] != "inherit" {
		t.Fatalf("null on create must be inherit: %v", got)
	}
	catPath := fmt.Sprintf("/api/v1/category/%d/update", cat)
	topPath := fmt.Sprintf("/api/v1/topCategory/%d/update", top)

	// An explicit setting set, then null leaves it unchanged, then inherit removes it.
	expectStatus(t, e.Do(http.MethodPost, catPath, w.admin, map[string]any{"category_container": "container"}), http.StatusOK)
	expectStatus(t, e.Do(http.MethodPost, catPath, w.admin, map[string]any{"category_container": nil, "category_name": "Packs2"}), http.StatusOK)
	if got := w.catGet(cat); got["category_container"] != "container" || got["category_name"] != "Packs2" || got["category_is_container"] != true {
		t.Errorf("null must leave the setting unchanged: %v", got)
	}
	expectStatus(t, e.Do(http.MethodPost, catPath, w.admin, map[string]any{"category_container": "inherit"}), http.StatusOK)
	if got := w.catGet(cat); got["category_container"] != "inherit" || got["category_is_container"] != false {
		t.Errorf("inherit must remove the explicit setting: %v", got)
	}

	// top_category_is_container: null leaves it unchanged.
	expectStatus(t, e.Do(http.MethodPost, topPath, w.admin, map[string]any{"top_category_is_container": true}), http.StatusOK)
	expectStatus(t, e.Do(http.MethodPost, topPath, w.admin, map[string]any{"top_category_is_container": nil, "top_category_name": "Bags2"}), http.StatusOK)
	if got := w.topGet(top); got["top_category_is_container"] != true || got["top_category_name"] != "Bags2" {
		t.Errorf("null must leave the top setting unchanged: %v", got)
	}

	before := e.catalogSnapshot()
	for _, bad := range []any{"yes", "", "Container", "INHERIT", 1, true, []string{"container"}, map[string]any{}} {
		r := e.Do(http.MethodPost, catPath, w.admin, map[string]any{"category_container": bad})
		expectStatus(t, r, http.StatusBadRequest)
		if !strings.Contains(r.Body.String(), "category_container") {
			t.Errorf("400 for %v does not name the field: %s", bad, r.Body.String())
		}
		expectStatus(t, w.catInsert("Bad", top, map[string]any{"category_container": bad}), http.StatusBadRequest)
	}
	// A bad value is a malformed body: 400 even for an unknown category.
	expectStatus(t, e.Do(http.MethodPost, "/api/v1/category/9999/update", w.admin, map[string]any{"category_container": "maybe"}), http.StatusBadRequest)
	// A non-bool top setting is 400.
	expectStatus(t, e.Do(http.MethodPost, topPath, w.admin, map[string]any{"top_category_is_container": "yes"}), http.StatusBadRequest)
	// Read-only fields alone are not updatable fields.
	expectStatus(t, e.Do(http.MethodPost, catPath, w.admin, map[string]any{"category_is_container": true}), http.StatusBadRequest)
	if after := e.catalogSnapshot(); after != before {
		t.Errorf("a refused request changed the catalog:\nbefore: %s\nafter:  %s", before, after)
	}

	// category_is_container is ignored on create and on update.
	r := w.catInsert("Ignored", top, map[string]any{"category_is_container": true})
	id := w.idOf(r, "category_id")
	if got := w.catGet(id); got["category_container"] != "inherit" || got["category_is_container"] != true {
		// top is a container (set above), so the effective value is true by inheritance, not by the body.
		t.Errorf("category %d: %v", id, got)
	}
	expectStatus(t, e.Do(http.MethodPost, topPath, w.admin, map[string]any{"top_category_is_container": false}), http.StatusOK)
	if got := w.catGet(id); got["category_is_container"] != false {
		t.Errorf("category_is_container sent on create must have no effect: %v", got)
	}
}

// A non-numeric path ID answers one 400 and stops. Before the fix the handler
// went on and wrote a second body.
func TestCategoryGetBadIDSingleResponse(t *testing.T) {
	w := newAPIWorld(t)
	for _, path := range []string{"/api/v1/category/abc/get", "/api/v1/topCategory/abc/get"} {
		r := w.env.Do(http.MethodGet, path, w.user, nil)
		expectStatus(t, r, http.StatusBadRequest)
		var m map[string]any
		decodeJSON(t, r, &m) // fails on a second concatenated body
		if m["error"] == nil {
			t.Fatalf("%s: no error field: %s", path, r.Body.String())
		}
	}
}
