package endpoints

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Database integrity (COR-03, COR-05) seen through the real handlers, with
// foreign keys enforced on the file database.
//
// Status policy under test:
//   - a request that names catalog gear that does not exist (registration
//     insert, loadout item insert, import) is 404 "Gear not found";
//   - any other invalid reference in a write is 400;
//   - deleting a catalog row that other rows still reference is 409.

type intFixture struct {
	env              *authzEnv
	alice, admin     int64
	aTok, adminTok   string
	gear, top, cat   int64
	man              int64
	loadout, regBox  int64
	regThing         int64
	loadoutItem      int64
	missingGear      int64
	loadoutWeightRaw func() int
}

func newIntFixture(t *testing.T) *intFixture {
	env := newAuthzEnv(t)
	f := &intFixture{env: env}
	f.alice = env.SeedUser("alice", "alice@example.com", false)
	f.admin = env.SeedUser("root", "root@example.com", true)
	f.aTok, f.adminTok = env.Token(f.alice, false), env.Token(f.admin, true)
	f.top = env.seedTopCategory("Shelter")
	f.cat = env.SeedCategory("Tents")
	f.man = env.seedManufacturer("Acme")
	f.gear = env.seedGearX(false, false, f.cat, f.top) // weighs 100
	env.setCategoryContainer(f.cat, boolPtr(true))     // its registrations hold items below
	f.missingGear = 987654
	f.loadout = env.seedLO(f.alice, false, "alice-lo")
	f.loadoutItem = env.seedItem(f.loadout, f.gear, 2)
	f.regBox = env.SeedRegistration(f.alice, f.gear)
	f.regThing = env.SeedRegistration(f.alice, f.gear)
	f.loadoutWeightRaw = func() int {
		var w int
		if err := env.DB.QueryRow(`SELECT totalWeight FROM loadouts WHERE loadoutId = ?`, f.loadout).Scan(&w); err != nil {
			t.Fatal(err)
		}
		return w
	}
	return f
}

func TestCatalogDelete_StillReferencedIs409AndChangesNothing(t *testing.T) {
	f := newIntFixture(t)
	for _, tc := range []struct{ name, path string }{
		{"gear (registered and in a loadout)", fmt.Sprintf("/api/v1/gear/%d/delete", f.gear)},
		{"category (used by gear)", fmt.Sprintf("/api/v1/category/%d/delete", f.cat)},
		{"manufacturer (used by gear)", fmt.Sprintf("/api/v1/manufacture/%d/delete", f.man)},
		{"top category (used by gear)", fmt.Sprintf("/api/v1/topCategory/%d/delete", f.top)},
	} {
		// The manufacturer seeded by the fixture is unused by the seeded gear
		// (it uses manufacturer 1), so reference it first.
		if strings.HasPrefix(tc.name, "manufacturer") {
			if _, err := f.env.DB.Exec(`UPDATE gear SET gearManufactureId = ? WHERE gearId = ?`, f.man, f.gear); err != nil {
				t.Fatal(err)
			}
		}
		before := f.env.catalogSnapshot()
		w := f.env.Do(http.MethodDelete, tc.path, f.adminTok, nil)
		expectStatus(t, w, http.StatusConflict)
		if !strings.Contains(w.Body.String(), "still referenced") {
			t.Errorf("%s: message does not explain the conflict: %s", tc.name, w.Body.String())
		}
		if f.env.catalogSnapshot() != before {
			t.Errorf("%s: refused delete changed the catalog", tc.name)
		}
	}
	if f.env.Count("loadout_items", "") != 1 || f.env.Count("user_gear_registrations", "") != 2 {
		t.Fatal("user data changed by refused catalog deletes")
	}
	// Once nothing refers to the gear, it can go.
	if _, err := f.env.DB.Exec(`DELETE FROM user_gear_registrations; DELETE FROM loadout_items`); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, f.env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/gear/%d/delete", f.gear), f.adminTok, nil), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/category/%d/delete", f.cat), f.adminTok, nil), http.StatusOK)
}

func TestCatalogWrites_UnknownReferenceIs400(t *testing.T) {
	f := newIntFixture(t)
	before := f.env.catalogSnapshot()
	for _, tc := range []struct{ name, method, path, body string }{
		{"gear insert", http.MethodPut, "/api/v1/gear/insert", `{"gear_top_category_id":1,"gear_category_id":555,"gear_manufacture_id":1,"gear_name":"x"}`},
		{"category insert", http.MethodPut, "/api/v1/category/insert", `{"category_top_category_id":555,"category_name":"x"}`},
		{"gear update", http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", f.gear), `{"gear_manufacture_id":555}`},
		{"category update", http.MethodPost, fmt.Sprintf("/api/v1/category/%d/update", f.cat), `{"category_top_category_id":555}`},
	} {
		w := f.env.Do(tc.method, tc.path, f.adminTok, tc.body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400; body %s", tc.name, w.Code, w.Body.String())
		}
	}
	if f.env.catalogSnapshot() != before {
		t.Fatal("refused writes changed the catalog")
	}
}

func TestRegistrationInsert_MissingGearIs404(t *testing.T) {
	f := newIntFixture(t)
	before := f.env.Count("user_gear_registrations", "")
	w := f.env.Do(http.MethodPut, "/api/v1/usergear/insert", f.aTok, map[string]any{"usergear_gear_id": f.missingGear})
	expectStatus(t, w, http.StatusNotFound)
	if !strings.Contains(w.Body.String(), "Gear not found") {
		t.Errorf("body: %s", w.Body.String())
	}
	if f.env.Count("user_gear_registrations", "") != before {
		t.Fatal("a registration was created for missing gear")
	}
	expectStatus(t, f.env.Do(http.MethodPut, "/api/v1/usergear/insert", f.aTok, map[string]any{"usergear_gear_id": f.gear}), http.StatusOK)
	if f.env.Count("user_gear_registrations", "") != before+1 {
		t.Fatal("valid registration not created")
	}
}

func TestLoadoutItemInsert_MissingGearIs404AndChangesNothing(t *testing.T) {
	f := newIntFixture(t)
	weight := f.loadoutWeightRaw()
	w := f.env.Do(http.MethodPut, loPath(f.loadout, "item/insert"), f.aTok, map[string]any{"gear_id": f.missingGear, "quantity": 1})
	expectStatus(t, w, http.StatusNotFound)
	if f.env.Count("loadout_items", "loadoutId = ?", f.loadout) != 1 || f.loadoutWeightRaw() != weight {
		t.Fatal("refused insert changed the loadout")
	}
	expectStatus(t, f.env.Do(http.MethodPut, loPath(f.loadout, "item/insert"), f.aTok, map[string]any{"gear_id": f.gear, "quantity": 1}), http.StatusCreated)
}

// Import stays all-or-nothing: one missing gear ID rolls everything back and
// the stored weight still matches the items.
func TestImport_MissingGearRollsBackEverything(t *testing.T) {
	f := newIntFixture(t)
	weight := f.loadoutWeightRaw()
	w := f.env.Do(http.MethodPost, loPath(f.loadout, "import"), f.aTok,
		map[string]any{"gear_ids": []int64{f.gear, f.gear, f.missingGear, f.gear}})
	expectStatus(t, w, http.StatusNotFound)
	if n := f.env.Count("loadout_items", "loadoutId = ?", f.loadout); n != 1 {
		t.Fatalf("%d items after failed import, want the 1 original", n)
	}
	if f.loadoutWeightRaw() != weight {
		t.Fatalf("weight changed from %d to %d", weight, f.loadoutWeightRaw())
	}
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.loadout, "import"), f.aTok, map[string]any{"gear_ids": []int64{f.gear, f.gear}}), http.StatusCreated)
	if n := f.env.Count("loadout_items", "loadoutId = ?", f.loadout); n != 3 {
		t.Fatalf("%d items after good import, want 3", n)
	}
}

// Cascades declared in the schema work with enforcement on, and the explicit
// deletes the handlers also run stay correct on top of them.
func TestCascadesWithForeignKeysEnforced(t *testing.T) {
	f := newIntFixture(t)
	f.env.SeedContainerLink(f.regBox, f.regThing)

	// Database level: deleting the parent row takes its children.
	if _, err := f.env.DB.Exec(`DELETE FROM user_gear_registrations WHERE userGearRegistrationId = ?`, f.regThing); err != nil {
		t.Fatal(err)
	}
	if n := f.env.Count("user_container_registration", ""); n != 0 {
		t.Fatalf("links left after registration delete: %d", n)
	}
	// A parent that still has non-cascading children cannot be deleted.
	if _, err := f.env.DB.Exec(`DELETE FROM users WHERE userId = ?`, f.alice); err == nil {
		t.Fatal("user with registrations deleted at database level")
	}

	// Handler level.
	f.env.SeedContainerLink(f.regBox, f.env.SeedRegistration(f.alice, f.gear))
	expectStatus(t, f.env.Do(http.MethodDelete, ugPath(f.regBox, "delete"), f.aTok, nil), http.StatusOK)
	if n := f.env.Count("user_container_registration", ""); n != 0 {
		t.Fatalf("links left after handler delete: %d", n)
	}
	expectStatus(t, f.env.Do(http.MethodDelete, loPath(f.loadout, "delete"), f.aTok, nil), http.StatusOK)
	if n := f.env.Count("loadout_items", ""); n != 0 {
		t.Fatalf("items left after loadout delete: %d", n)
	}
	expectStatus(t, f.env.Do(http.MethodDelete, userPath(f.alice, "delete"), f.aTok, nil), http.StatusOK)
	if f.env.Count("user_gear_registrations", "userId = ?", f.alice) != 0 || f.env.Count("loadouts", "userId = ?", f.alice) != 0 {
		t.Fatal("account data left after self-delete")
	}
}
