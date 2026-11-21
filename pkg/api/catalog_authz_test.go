package endpoints

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Adversarial regression tests for BR-001, BR-017, BR-018 (catalog: gear,
// manufacturers, top categories, categories) and the catalog write defects
// COR-06. They run through the real JWT middleware and the same route
// registration main uses.

func (e *authzEnv) seedManufacturer(name string) int64 {
	e.T.Helper()
	res, err := e.DB.Exec(`INSERT INTO manufacture (manufactureName) VALUES (?)`, name)
	if err != nil {
		e.T.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (e *authzEnv) seedTopCategory(name string) int64 {
	e.T.Helper()
	res, err := e.DB.Exec(`INSERT INTO gear_top_category (topCategoryName, topCategoryIcon) VALUES (?, 'tent')`, name)
	if err != nil {
		e.T.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// catalogSnapshot is the complete stored state of the four catalog tables.
func (e *authzEnv) catalogSnapshot() string {
	e.T.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT quote(gearId)||'|'||quote(gearTopCategoryId)||'|'||quote(gearCategoryId)||'|'||quote(gearManufactureId)||'|'||quote(gearIsContainer)||'|'||quote(gearSizeDefinition)||'|'||quote(gearName)||'|'||quote(gearWeight)||'|'||quote(gearHeight)||'|'||quote(gearLength)||'|'||quote(gearWidth)||'|'||quote(gearStatus) FROM gear ORDER BY gearId`,
		`SELECT quote(manufactureId)||'|'||quote(manufactureName) FROM manufacture ORDER BY manufactureId`,
		`SELECT quote(topCategoryId)||'|'||quote(topCategoryName)||'|'||quote(topCategoryIcon) FROM gear_top_category ORDER BY topCategoryId`,
		`SELECT quote(categoryId)||'|'||quote(categoryTopCategoryId)||'|'||quote(categoryName) FROM gear_category ORDER BY categoryId`,
	} {
		rows, err := e.DB.Query(q)
		if err != nil {
			e.T.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				e.T.Fatal(err)
			}
			sb.WriteString(s + "\n")
		}
		rows.Close()
		sb.WriteString("--\n")
	}
	return sb.String()
}

type catalogFixture struct {
	env                 *authzEnv
	user, admin         int64
	gear, man, top, cat int64
	userTok, adminTok   string
}

func newCatalogFixture(t *testing.T) *catalogFixture {
	env := newAuthzEnv(t)
	f := &catalogFixture{env: env}
	f.user = env.SeedUser("alice", "alice@example.com", false)
	f.admin = env.SeedUser("root", "root@example.com", true)
	f.userTok, f.adminTok = env.Token(f.user, false), env.Token(f.admin, true)
	f.top = env.seedTopCategory("Shelter")
	f.cat = env.SeedCategory("Tents")
	f.man = env.seedManufacturer("Acme")
	f.gear = env.seedGearX(false, false, f.cat, f.top)
	return f
}

func (f *catalogFixture) writes() []struct{ name, method, path, body string } {
	return []struct{ name, method, path, body string }{
		{"gear insert", http.MethodPut, "/api/v1/gear/insert", `{"gear_top_category_id":1,"gear_category_id":1,"gear_manufacture_id":1,"gear_name":"evil"}`},
		{"gear update", http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), `{"gear_name":"evil"}`},
		{"gear delete", http.MethodDelete, fmt.Sprintf("/api/v1/gear/%d/delete", f.gear), ""},
		{"manufacture insert", http.MethodPut, "/api/v1/manufacture/insert", `{"manufacture_name":"evil"}`},
		{"manufacture update", http.MethodPost, fmt.Sprintf("/api/v1/manufacture/%d/update", f.man), `{"manufacture_name":"evil"}`},
		{"manufacture delete", http.MethodDelete, fmt.Sprintf("/api/v1/manufacture/%d/delete", f.man), ""},
		{"topCategory insert", http.MethodPut, "/api/v1/topCategory/insert", `{"top_category_name":"evil"}`},
		{"topCategory update", http.MethodPost, fmt.Sprintf("/api/v1/topCategory/%d/update", f.top), `{"top_category_name":"evil"}`},
		{"topCategory delete", http.MethodDelete, fmt.Sprintf("/api/v1/topCategory/%d/delete", f.top), ""},
		{"category insert", http.MethodPut, "/api/v1/category/insert", `{"category_top_category_id":1,"category_name":"evil"}`},
		{"category update", http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", f.cat), `{"category_name":"evil"}`},
		{"category delete", http.MethodDelete, fmt.Sprintf("/api/v1/category/%d/delete", f.cat), ""},
	}
}

// BR-001 AC-1, AC-3, BR-018: every catalog write is refused for a non-admin
// with 403 and changes nothing; without a token it is 401.
func TestCatalog_NonAdminWritesRefusedUnchanged(t *testing.T) {
	f := newCatalogFixture(t)
	before := f.env.catalogSnapshot()
	for _, tc := range f.writes() {
		t.Run(tc.name, func(t *testing.T) {
			expectStatus(t, f.env.Do(tc.method, tc.path, f.userTok, tc.body), http.StatusForbidden)
			expectStatus(t, f.env.Do(tc.method, tc.path, "", tc.body), http.StatusUnauthorized)
			if after := f.env.catalogSnapshot(); after != before {
				t.Fatalf("catalog changed by refused request:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// BR-001 AC-4: reads stay open to non-admins.
func TestCatalog_NonAdminReadsStillWork(t *testing.T) {
	f := newCatalogFixture(t)
	for _, p := range []string{
		"/api/v1/gear/list", fmt.Sprintf("/api/v1/gear/%d/get", f.gear),
		"/api/v1/manufacture/list", fmt.Sprintf("/api/v1/manufacture/%d/get", f.man),
		"/api/v1/topCategory/list", fmt.Sprintf("/api/v1/topCategory/%d/get", f.top),
		"/api/v1/category/list", fmt.Sprintf("/api/v1/category/%d/get", f.cat),
	} {
		expectStatus(t, f.env.Do(http.MethodGet, p, f.userTok, nil), http.StatusOK)
	}
}

// BR-001 AC-5: a client-audience token is not admin even for a user whose row
// says admin, and a body flag cannot elevate.
func TestCatalog_ClientAudienceTokenOfAdminUserRefused(t *testing.T) {
	f := newCatalogFixture(t)
	before := f.env.catalogSnapshot()
	tok := f.env.Token(f.admin, false)
	w := f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), tok, `{"gear_name":"evil","user_is_admin":true}`)
	expectStatus(t, w, http.StatusForbidden)
	if f.env.catalogSnapshot() != before {
		t.Fatal("catalog changed")
	}
}

// BR-001 AC-2: admins can do every write, and DeleteGear works on rows with
// NULL dimension columns, reporting the ID rather than a pointer.
func TestCatalog_AdminWritesSucceed(t *testing.T) {
	f := newCatalogFixture(t)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/gear/insert", fmt.Sprintf(`{"gear_top_category_id":%d,"gear_category_id":%d,"gear_manufacture_id":%d,"gear_name":"new"}`, f.top, f.cat, f.man)},
		{http.MethodPut, "/api/v1/manufacture/insert", `{"manufacture_name":"New"}`},
		{http.MethodPut, "/api/v1/topCategory/insert", `{"top_category_name":"New"}`},
		{http.MethodPut, "/api/v1/category/insert", fmt.Sprintf(`{"category_top_category_id":%d,"category_name":"New"}`, f.top)},
		{http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), `{"gear_name":"renamed"}`},
		{http.MethodPost, fmt.Sprintf("/api/v1/manufacture/%d/update", f.man), `{"manufacture_name":"Renamed"}`},
		{http.MethodPost, fmt.Sprintf("/api/v1/topCategory/%d/update", f.top), `{"top_category_name":"Renamed"}`},
		{http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", f.cat), `{"category_name":"Renamed"}`},
	} {
		w := f.env.Do(tc.method, tc.path, f.adminTok, tc.body)
		expectStatus(t, w, http.StatusOK)
	}
	if n := f.env.Count("gear", "gearName = 'renamed'"); n != 1 {
		t.Fatalf("gear not renamed")
	}
	// The inserts above created rows that reference the fixture's category,
	// manufacturer and top category; they would block the deletes (409).
	if _, err := f.env.DB.Exec(`DELETE FROM gear WHERE gearName = 'new'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.DB.Exec(`DELETE FROM gear_category WHERE categoryName = 'New'`); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		fmt.Sprintf("/api/v1/gear/%d/delete", f.gear),
		fmt.Sprintf("/api/v1/category/%d/delete", f.cat),
		fmt.Sprintf("/api/v1/manufacture/%d/delete", f.man),
		fmt.Sprintf("/api/v1/topCategory/%d/delete", f.top),
	} {
		w := f.env.Do(http.MethodDelete, p, f.adminTok, nil)
		expectStatus(t, w, http.StatusOK)
		if strings.Contains(w.Body.String(), "0xc") {
			t.Errorf("response exposes a pointer: %s", w.Body.String())
		}
	}
}

func TestDeleteGear_NullDimensionsAndMessage(t *testing.T) {
	f := newCatalogFixture(t)
	g := f.env.seedGearX(false, true, f.cat, f.top)
	w := f.env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/gear/%d/delete", g), f.adminTok, nil)
	expectStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), fmt.Sprintf("ID %d ", g)) || strings.Contains(w.Body.String(), "0xc") {
		t.Fatalf("message must name the gear ID, not a pointer: %s", w.Body.String())
	}
	if f.env.Count("gear", "gearId = ?", g) != 0 {
		t.Fatal("gear not deleted")
	}
}

// BR-017: the path ID is the target; a body ID never redirects it. Missing
// body ID must not panic (COR-06). Updates touch only the fields sent.
func TestCatalogUpdate_PathIDWinsAndIsPartial(t *testing.T) {
	f := newCatalogFixture(t)
	other := f.env.seedGearX(false, false, f.cat, f.top)
	before := f.env.catalogSnapshot()

	// body gear_id points at another row: that row must not change.
	w := f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), f.adminTok,
		fmt.Sprintf(`{"gear_id":%d,"gear_name":"changed"}`, other))
	expectStatus(t, w, http.StatusBadRequest) // P-2: conflicting body ID is refused
	var name string
	var weight, height int
	f.env.DB.QueryRow(`SELECT gearName FROM gear WHERE gearId = ?`, other).Scan(&name)
	if name != "full" {
		t.Fatalf("body ID redirected the update to row %d (name %q)", other, name)
	}
	if before != f.env.catalogSnapshot() {
		t.Fatal("refused update changed the catalog")
	}
	// a matching body ID is accepted
	expectStatus(t, f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), f.adminTok,
		fmt.Sprintf(`{"gear_id":%d,"gear_name":"changed"}`, f.gear)), http.StatusOK)
	f.env.DB.QueryRow(`SELECT gearName, gearWeight, gearHeight FROM gear WHERE gearId = ?`, f.gear).Scan(&name, &weight, &height)
	if name != "changed" || weight != 100 || height != 1 {
		t.Fatalf("path row = %q weight %d height %d; want renamed, other columns kept", name, weight, height)
	}

	// No ID in the body at all: no panic, partial update.
	expectStatus(t, f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), f.adminTok, `{"gear_weight":555}`), http.StatusOK)
	f.env.DB.QueryRow(`SELECT gearName, gearWeight FROM gear WHERE gearId = ?`, f.gear).Scan(&name, &weight)
	if name != "changed" || weight != 555 {
		t.Fatalf("partial update wrong: %q %d", name, weight)
	}

	for _, tc := range []struct{ path, body, table, where string }{
		{fmt.Sprintf("/api/v1/manufacture/%d/update", f.man), `{"manufacture_name":"M2"}`, "manufacture", "manufactureName = 'M2'"},
		{fmt.Sprintf("/api/v1/topCategory/%d/update", f.top), `{"top_category_name":"T2"}`, "gear_top_category", "topCategoryName = 'T2' AND topCategoryIcon = 'tent'"},
		{fmt.Sprintf("/api/v1/category/%d/update", f.cat), `{"category_name":"C2"}`, "gear_category", "categoryName = 'C2' AND categoryTopCategoryId = 1"},
	} {
		expectStatus(t, f.env.Do(http.MethodPost, tc.path, f.adminTok, tc.body), http.StatusOK)
		if f.env.Count(tc.table, tc.where) != 1 {
			t.Errorf("%s: not a partial update by path ID", tc.path)
		}
	}

	// A body ID on manufacture / topCategory / category must not redirect either.
	m2 := f.env.seedManufacturer("Other")
	expectStatus(t, f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/manufacture/%d/update", f.man), f.adminTok,
		fmt.Sprintf(`{"manufacture_id":%d,"manufacture_name":"M3"}`, m2)), http.StatusBadRequest)
	if f.env.Count("manufacture", "manufactureId = ? AND manufactureName = 'Other'", m2) != 1 {
		t.Fatal("manufacture body ID redirected update")
	}
	t2 := f.env.seedTopCategory("Other")
	expectStatus(t, f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/topCategory/%d/update", f.top), f.adminTok,
		fmt.Sprintf(`{"top_category_id":%d,"top_category_name":"T3"}`, t2)), http.StatusBadRequest)
	if f.env.Count("gear_top_category", "topCategoryId = ? AND topCategoryName = 'Other'", t2) != 1 {
		t.Fatal("topCategory body ID redirected update")
	}
	c2 := f.env.SeedCategory("Other")
	expectStatus(t, f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", f.cat), f.adminTok,
		fmt.Sprintf(`{"category_id":%d,"category_name":"C3"}`, c2)), http.StatusBadRequest)
	if f.env.Count("gear_category", "categoryId = ? AND categoryName = 'Other'", c2) != 1 {
		t.Fatal("category body ID redirected update")
	}
}

// BR-018: unknown targets are 404 (not 200, not 500); malformed input is 400.
func TestCatalog_UnknownTargetIs404(t *testing.T) {
	f := newCatalogFixture(t)
	before := f.env.catalogSnapshot()
	for _, res := range []struct{ prefix, body string }{
		{"gear", `{"gear_name":"x"}`}, {"manufacture", `{"manufacture_name":"x"}`},
		{"topCategory", `{"top_category_name":"x"}`}, {"category", `{"category_name":"x"}`},
	} {
		expectStatus(t, f.env.Do(http.MethodPost, "/api/v1/"+res.prefix+"/9999/update", f.adminTok, res.body), http.StatusNotFound)
		expectStatus(t, f.env.Do(http.MethodDelete, "/api/v1/"+res.prefix+"/9999/delete", f.adminTok, nil), http.StatusNotFound)
		expectStatus(t, f.env.Do(http.MethodPost, "/api/v1/"+res.prefix+"/abc/update", f.adminTok, res.body), http.StatusBadRequest)
		expectStatus(t, f.env.Do(http.MethodPost, fmt.Sprintf("/api/v1/%s/%d/update", res.prefix, map[string]int64{"gear": f.gear, "manufacture": f.man, "topCategory": f.top, "category": f.cat}[res.prefix]), f.adminTok, `{`), http.StatusBadRequest)
	}
	if f.env.catalogSnapshot() != before {
		t.Fatal("catalog changed")
	}
}

// BR-018 AC-4: an unknown target is 404 before any body conflict is judged.
func TestCatalog_UnknownTargetWithConflictingBodyIDIs404(t *testing.T) {
	f := newCatalogFixture(t)
	before := f.env.catalogSnapshot()
	for _, res := range []struct{ prefix, body string }{
		{"gear", `{"gear_id":1,"gear_name":"x"}`},
		{"manufacture", `{"manufacture_id":1,"manufacture_name":"x"}`},
		{"topCategory", `{"top_category_id":1,"top_category_name":"x"}`},
		{"category", `{"category_id":1,"category_name":"x"}`},
		{"gear", `{}`}, // an empty body on an unknown ID is also 404, not 400
	} {
		expectStatus(t, f.env.Do(http.MethodPost, "/api/v1/"+res.prefix+"/9999/update", f.adminTok, res.body), http.StatusNotFound)
	}
	if f.env.catalogSnapshot() != before {
		t.Fatal("catalog changed")
	}
}

// BR-001: every non-GET route under a catalog prefix refuses a non-admin with
// 403. The routes come from the router, so a future write route that skips
// RequireAdmin fails here instead of passing unnoticed.
func TestCatalog_EveryWriteRouteRequiresAdmin(t *testing.T) {
	f := newCatalogFixture(t)
	prefixes := []string{"/api/v1/gear/", "/api/v1/manufacture/", "/api/v1/category/", "/api/v1/topCategory/"}
	before := f.env.catalogSnapshot()

	checked := 0
	for _, rt := range f.env.Router.Routes() {
		if rt.Method == http.MethodGet || rt.Method == http.MethodHead || rt.Method == http.MethodOptions {
			continue
		}
		catalog := false
		for _, p := range prefixes {
			catalog = catalog || strings.HasPrefix(rt.Path, p)
		}
		if !catalog {
			continue
		}
		path := rt.Path
		for _, param := range []string{":gear", ":manufacture", ":category", ":topCategory"} {
			path = strings.ReplaceAll(path, param, "1")
		}
		resp := f.env.Do(rt.Method, path, f.userTok, `{}`)
		if resp.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: status %d, want 403", rt.Method, rt.Path, resp.Code)
		}
		checked++
	}
	if checked < 12 {
		t.Fatalf("walked only %d catalog write routes; the route walk is not seeing the router", checked)
	}
	if f.env.catalogSnapshot() != before {
		t.Fatal("catalog changed")
	}
}
