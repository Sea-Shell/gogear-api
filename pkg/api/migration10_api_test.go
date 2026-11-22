package endpoints

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
	sqlite3driver "github.com/golang-migrate/migrate/v4/database/sqlite3"
)

// BR-021 AC-5 (amended), ADR-002 D-7, end to end: a database from before the
// category-driven capability (version 8, gear carrying the old per-gear
// flags) goes through the real migrations to the latest version with no admin
// action, and the real API then lets the seeded categories' gear hold items
// while the others still refuse. No category setting is written by the test
// after the migration.
func TestMigration10SeededCategoryGearCanHoldItemsThroughTheAPI(t *testing.T) {
	w := newAPIWorld(t)
	db := w.env.DB

	// newAPIWorld migrated to the latest version; return the database to
	// version 8 and write the data that existed before the change.
	driver, err := sqlite3driver.WithInstance(db, &sqlite3driver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath(t), "sqlite3", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(8); err != nil {
		t.Fatalf("migrate to 8: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO gear_category (categoryId, categoryTopCategoryId, categoryName) VALUES (2, 1, 'Backpacks'), (3, 1, 'Tents')`,
		// Gear 10 is flagged and gear 11 is not, both in Backpacks: any flagged
		// gear makes the sub-category a container. Gear 12 is in Tents, unflagged.
		`INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearIsContainer, gearName, gearWeight, gearHeight, gearLength, gearWidth, gearStatus) VALUES
			(10, 1, 2, 1, 1, 'pack', 100, 1, 2, 3, 1), (11, 1, 2, 1, 0, 'pack-plain', 100, 1, 2, 3, 1), (12, 1, 3, 1, 0, 'tent', 100, 1, 2, 3, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed v8: %v", err)
		}
	}

	// The release path: the app only ever calls Up.
	runMigrate(t, db)

	if got := w.catGet(2); got["category_container"] != "container" || got["category_is_container"] != true {
		t.Errorf("Backpacks reads %v/%v, want container/true", got["category_container"], got["category_is_container"])
	}
	if got := w.catGet(3); got["category_container"] != "inherit" || got["category_is_container"] != false {
		t.Errorf("Tents reads %v/%v, want inherit/false", got["category_container"], got["category_is_container"])
	}
	for _, g := range []struct {
		id   int64
		want bool
	}{{10, true}, {11, true}, {12, false}} {
		r := w.env.Do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", g.id), w.user, nil)
		expectStatus(t, r, http.StatusOK)
		var gear map[string]any
		decodeJSON(t, r, &gear)
		if gear["gear_is_container"] != g.want {
			t.Errorf("gear %d reports gear_is_container=%v, want %v", g.id, gear["gear_is_container"], g.want)
		}
	}

	// A seeded category's gear can hold items; an unseeded one still refuses.
	pack, plain, tent := w.register(10), w.register(11), w.register(12)
	expectStatus(t, w.link(pack, plain), http.StatusOK)
	r := w.link(tent, plain)
	expectStatus(t, r, http.StatusConflict)
	if !strings.Contains(r.Body.String(), cannotHold) {
		t.Errorf("link into Tents gear: %s, want the %q refusal", r.Body.String(), cannotHold)
	}
}
