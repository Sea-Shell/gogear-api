package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
)

// Migration 000011 (ARS-001 section 2.1, ADR-003/004, BR-023 to BR-025,
// BR-029, BR-030). The tests run on a version 10 database with these rows
// (ten users, gear and registrations are kept small on purpose):
//
//	users 1 and 2
//	gear 1 'g1' 100 g, gear 2 'g2' 200 g, gear 3 'g3' 0 g, gear 4 'g4' 300 g; gear 98, 99 do not exist
//	registrations: 1 (gear 1, user 1), 2 and 3 (gear 2, user 1), 4 (gear 2, user 2),
//	               5 (gear 3, user 1), 6 (gear 99 = missing gear, user 1)
//	loadouts: 1 and 2 (user 1; 2 has no items), 3 (user 2)
//	old items (gearId): 1 g1 x2, 2 g2, 3 g4 (no registration), 4 g98 (gear missing, no
//	  registration), 5 g99 x3 (gear missing but a registration exists), 6 loadout 3 g2,
//	  7 loadout 77 (loadout missing); IDs 8..50 existed and were deleted.
func loadoutV10DB(t *testing.T) *sql.DB {
	t.Helper()
	file := filepath.Join(t.TempDir(), "v10.db")
	raw, err := sql.Open("sqlite3", file+"?_foreign_keys=off")
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	stepMigrate(t, raw, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(10) })
	if _, err := raw.Exec(`
INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (1, 'a', '', 'a@example.com'), (2, 'b', '', 'b@example.com');
INSERT INTO gear_top_category (topCategoryId, topCategoryName) VALUES (1, 't');
INSERT INTO gear_category (categoryId, categoryTopCategoryId, categoryName) VALUES (1, 1, 'c');
INSERT INTO manufacture (manufactureId, manufactureName) VALUES (1, 'm');
INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight) VALUES
	(1, 1, 1, 1, 'g1', 100), (2, 1, 1, 1, 'g2', 200), (3, 1, 1, 1, 'g3', 0), (4, 1, 1, 1, 'g4', 300);
INSERT INTO user_gear_registrations (userGearRegistrationId, gearId, userId) VALUES
	(1, 1, 1), (2, 2, 1), (3, 2, 1), (4, 2, 2), (5, 3, 1), (6, 99, 1);
INSERT INTO loadouts (loadoutId, userId, loadoutName) VALUES (1, 1, 'L1'), (2, 1, 'L2'), (3, 2, 'L3');
INSERT INTO loadout_items (loadoutItemId, loadoutId, gearId, quantity, notes) VALUES
	(1, 1, 1, 2, 'a'), (2, 1, 2, 1, 'b'), (3, 1, 4, 1, 'legacy'), (4, 1, 98, 1, 'orphan'),
	(5, 1, 99, 3, 'orphanreg'), (6, 3, 2, 1, 'u2'), (7, 77, 1, 1, 'noloadout');
WITH RECURSIVE n(i) AS (SELECT 8 UNION ALL SELECT i + 1 FROM n WHERE i < 50)
	INSERT INTO loadout_items (loadoutItemId, loadoutId, gearId) SELECT i, 1, 1 FROM n;
DELETE FROM loadout_items WHERE loadoutItemId >= 8;
`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	raw.Close()
	return openEnforced(t, file)
}

// loadoutV11DB is loadoutV10DB migrated to version 11 by golang-migrate, with
// foreign keys enforced (as in production).
func loadoutV11DB(t *testing.T) *sql.DB {
	t.Helper()
	db := loadoutV10DB(t)
	migrateTo(t, db, 11)
	return db
}

// table returns every row of a query as "a|b|c" with NULL spelled out.
func table(t *testing.T, db *sql.DB, q string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			switch x := v.(type) {
			case nil:
				parts[i] = "NULL"
			case []byte:
				parts[i] = string(x)
			default:
				parts[i] = fmt.Sprint(x)
			}
		}
		out = append(out, strings.Join(parts, "|"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) sql.Result {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return res
}

func placeID(t *testing.T, db *sql.DB, loadout int, kind string) int64 {
	t.Helper()
	return scalar[int64](t, db, `SELECT placeId FROM loadout_places WHERE loadoutId = ? AND placeKind = ?`, loadout, kind)
}

func addBag(t *testing.T, db *sql.DB, loadout, registration, position int) int64 {
	t.Helper()
	res := mustExec(t, db, `INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (?, 'bag', ?, ?)`, loadout, registration, position)
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

const itemColumns = `SELECT loadoutItemId, loadoutId, registrationId, quantity, notes, ticked, placeId,
	detachedReason, detachedGearId, detachedGearName, detachedUnitWeight, detachedWeightSource
	FROM loadout_items ORDER BY loadoutItemId`

// BR-023 AC-7, ADR-004 D-2: matched items are bound to the lowest registration
// of the loadout owner for the gear; unmatched items with existing gear become
// legacy items that read the gear live; orphans and items of missing loadouts
// are dropped; nothing else is lost, and loadouts without items stay.
func TestMigration11BindsLegacyAndOrphanItems(t *testing.T) {
	db := loadoutV11DB(t)

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 11 {
		t.Fatalf("version = %d, want 11", v)
	}
	if d := scalar[bool](t, db, `SELECT dirty FROM schema_migrations`); d {
		t.Fatal("migration left the database dirty")
	}
	want := []string{
		// id|loadout|registration|qty|notes|ticked|place|reason|gear|name|weight|source
		"1|1|1|2|a|0|NULL|NULL|NULL|NULL|NULL|NULL",         // bound to the only registration of g1
		"2|1|2|1|b|0|NULL|NULL|NULL|NULL|NULL|NULL",         // g2 has registrations 2 and 3: the lowest
		"3|1|NULL|1|legacy|0|NULL|legacy|4|NULL|NULL|NULL",  // no registration: legacy, no snapshot
		"5|1|6|3|orphanreg|0|NULL|NULL|NULL|NULL|NULL|NULL", // gear missing but a registration exists: bound
		"6|3|4|1|u2|0|NULL|NULL|NULL|NULL|NULL|NULL",        // the loadout owner's own registration, not user 1's
	}
	if got := table(t, db, itemColumns); !reflect.DeepEqual(got, want) {
		t.Errorf("items after migration:\n got %q\nwant %q", got, want)
	}
	if got := table(t, db, `SELECT loadoutId FROM loadouts ORDER BY 1`); !reflect.DeepEqual(got, []string{"1", "2", "3"}) {
		t.Errorf("loadouts = %v", got)
	}
}

// BR-024: every loadout, existing or new, has exactly one Worn; the partial
// unique indexes refuse a second Worn, a second Last minute and a bag twice.
func TestMigration11WornForEveryLoadout(t *testing.T) {
	db := loadoutV11DB(t)

	if got := table(t, db, `SELECT loadoutId, placeKind, registrationId, parentPlaceId, position, closed FROM loadout_places ORDER BY loadoutId, placeId`); !reflect.DeepEqual(got,
		[]string{"1|worn|NULL|NULL|NULL|0", "2|worn|NULL|NULL|NULL|0", "3|worn|NULL|NULL|NULL|0"}) {
		t.Errorf("places after migration = %v", got)
	}

	mustExec(t, db, `INSERT INTO loadouts (userId, loadoutName) VALUES (1, 'new')`)
	if got := table(t, db, `SELECT placeKind FROM loadout_places WHERE loadoutId = (SELECT MAX(loadoutId) FROM loadouts)`); !reflect.DeepEqual(got, []string{"worn"}) {
		t.Errorf("a new loadout has places %v, want one worn", got)
	}

	mustFail(t, db, "UNIQUE", `INSERT INTO loadout_places (loadoutId, placeKind) VALUES (1, 'worn')`)
	mustExec(t, db, `INSERT INTO loadout_places (loadoutId, placeKind) VALUES (1, 'last_minute')`)
	mustFail(t, db, "UNIQUE", `INSERT INTO loadout_places (loadoutId, placeKind) VALUES (1, 'last_minute')`)
	mustExec(t, db, `INSERT INTO loadout_places (loadoutId, placeKind) VALUES (2, 'last_minute')`) // another loadout is fine

	addBag(t, db, 1, 1, 1)
	mustFail(t, db, "UNIQUE", `INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (1, 'bag', 1, 2)`)
}

// The shape rules of loadout_places and loadout_items that need no trigger.
func TestMigration11ColumnRules(t *testing.T) {
	db := loadoutV11DB(t)
	worn := placeID(t, db, 1, "worn")

	mustFail(t, db, "CHECK", `INSERT INTO loadout_places (loadoutId, placeKind, registrationId) VALUES (1, 'bag', 1)`)                         // bag without position
	mustFail(t, db, "CHECK", `INSERT INTO loadout_places (loadoutId, placeKind, position) VALUES (1, 'bag', 1)`)                               // bag without registration
	mustFail(t, db, "CHECK", `INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (1, 'last_minute', 1, NULL)`) // non-bag with registration
	mustFail(t, db, "CHECK", `UPDATE loadout_places SET position = 1 WHERE placeId = ?`, worn)                                                 // non-bag with position
	mustFail(t, db, "CHECK", `UPDATE loadout_places SET closed = 2 WHERE placeId = ?`, worn)
	mustFail(t, db, "CHECK", `INSERT INTO loadout_items (loadoutId, registrationId, quantity) VALUES (1, 1, 0)`)
	mustFail(t, db, "CHECK", `INSERT INTO loadout_items (loadoutId, registrationId, ticked) VALUES (1, 1, 2)`)
	mustFail(t, db, "CHECK", `INSERT INTO loadout_items (loadoutId) VALUES (1)`) // neither a registration nor a detached reason
	mustFail(t, db, "CHECK", `INSERT INTO loadout_items (loadoutId, detachedReason) VALUES (1, 'other')`)
	mustFail(t, db, "item registration must belong to the loadout owner", `INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 4242)`) // a missing registration has no owner
	mustFail(t, db, "FOREIGN KEY", `INSERT INTO loadout_items (loadoutId, detachedReason, detachedGearId) VALUES (1, 'legacy', 4242)`)
	mustExec(t, db, `UPDATE loadout_places SET closed = 1 WHERE placeId = ?`, worn)
}

// ADR-003 D-3: the database refuses a bag of another owner, a parent that is
// not a bag of the same loadout, itself as parent and a cycle.
func TestMigration11PlaceTriggers(t *testing.T) {
	db := loadoutV11DB(t)
	a := addBag(t, db, 1, 1, 1) // user 1's registration 1
	b := addBag(t, db, 1, 2, 2)
	c := addBag(t, db, 1, 3, 3)
	foreign := addBag(t, db, 3, 4, 1) // loadout 3 belongs to user 2, registration 4 too
	worn := placeID(t, db, 1, "worn")

	t.Run("bag of another owner", func(t *testing.T) {
		mustFail(t, db, "bag registration must belong to the loadout owner",
			`INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (1, 'bag', 4, 9)`)
		mustFail(t, db, "bag registration must belong to the loadout owner",
			`UPDATE loadout_places SET registrationId = 4 WHERE placeId = ?`, a)
		mustFail(t, db, "bag registration must belong to the loadout owner",
			`UPDATE loadout_places SET loadoutId = 3 WHERE placeId = ?`, a)
	})

	t.Run("parent must be a bag of the same loadout", func(t *testing.T) {
		const msg = "parent must be another bag of the same loadout"
		mustFail(t, db, msg, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, foreign, a) // bag of another loadout
		mustFail(t, db, msg, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, worn, a)    // Worn is not a bag
		mustFail(t, db, msg, `UPDATE loadout_places SET parentPlaceId = 9999 WHERE placeId = ?`, a)
		mustFail(t, db, msg, `INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position, parentPlaceId) VALUES (1, 'bag', 5, 4, ?)`, foreign)
		if got := table(t, db, `SELECT parentPlaceId FROM loadout_places WHERE placeId = ?`, a); !reflect.DeepEqual(got, []string{"NULL"}) {
			t.Errorf("a refused update changed the parent: %v", got)
		}
	})

	t.Run("a bag is not its own parent", func(t *testing.T) {
		// Either rule may report it; the update must be refused and change nothing.
		_, err := db.Exec(`UPDATE loadout_places SET parentPlaceId = placeId WHERE placeId = ?`, a)
		if err == nil {
			t.Fatal("a bag became its own parent")
		}
		if !strings.Contains(err.Error(), "parent must be another bag") && !strings.Contains(err.Error(), "its own contents") {
			t.Errorf("unexpected error: %v", err)
		}
		if got := table(t, db, `SELECT parentPlaceId FROM loadout_places WHERE placeId = ?`, a); !reflect.DeepEqual(got, []string{"NULL"}) {
			t.Errorf("a refused update changed the parent: %v", got)
		}
	})

	t.Run("no cycle", func(t *testing.T) {
		const msg = "a bag cannot be inside one of its own contents"
		mustExec(t, db, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, a, b) // b inside a
		mustFail(t, db, msg, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, b, a)
		mustExec(t, db, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, b, c) // c inside b inside a
		mustFail(t, db, msg, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, c, a)
		mustFail(t, db, msg, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, c, b)
		// Moving a bag to a legal place still works, and so does clearing the parent.
		mustExec(t, db, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, a, c)
		mustExec(t, db, `UPDATE loadout_places SET parentPlaceId = NULL WHERE placeId = ?`, b)
		if got := table(t, db, `SELECT placeId, parentPlaceId FROM loadout_places WHERE placeKind = 'bag' AND loadoutId = 1 ORDER BY placeId`); !reflect.DeepEqual(got,
			[]string{fmt.Sprintf("%d|NULL", a), fmt.Sprintf("%d|NULL", b), fmt.Sprintf("%d|%d", c, a)}) {
			t.Errorf("bag tree = %v", got)
		}
	})
}

// ADR-003 D-3 / ADR-004: an item's registration belongs to the loadout owner
// and its place to the same loadout.
func TestMigration11ItemTriggers(t *testing.T) {
	db := loadoutV11DB(t)
	bag1 := addBag(t, db, 1, 1, 1)
	bag3 := addBag(t, db, 3, 4, 1)
	last1 := placeID(t, db, 1, "worn")

	mustFail(t, db, "item registration must belong to the loadout owner",
		`INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 4)`) // user 2's registration in user 1's loadout
	mustFail(t, db, "item registration must belong to the loadout owner",
		`UPDATE loadout_items SET registrationId = 4 WHERE loadoutItemId = 1`)
	mustFail(t, db, "item registration must belong to the loadout owner",
		`UPDATE loadout_items SET loadoutId = 3 WHERE loadoutItemId = 1`) // user 1's registration moved into user 2's loadout

	mustFail(t, db, "item place must belong to the same loadout",
		`INSERT INTO loadout_items (loadoutId, registrationId, placeId) VALUES (1, 1, ?)`, bag3)
	mustFail(t, db, "item place must belong to the same loadout",
		`UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 1`, bag3)

	// Legal writes still work.
	mustExec(t, db, `UPDATE loadout_items SET placeId = ?, ticked = 1 WHERE loadoutItemId = 1`, bag1)
	mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId, placeId) VALUES (1, 2, ?)`, last1)
	if got := table(t, db, `SELECT placeId, ticked FROM loadout_items WHERE loadoutItemId = 1`); !reflect.DeepEqual(got, []string{fmt.Sprintf("%d|1", bag1)}) {
		t.Errorf("item 1 = %v", got)
	}
}

// A registration never changes owner; other updates of it are unaffected.
func TestMigration11RegistrationOwnerIsFixed(t *testing.T) {
	db := loadoutV11DB(t)
	mustFail(t, db, "a registration cannot change owner", `UPDATE user_gear_registrations SET userId = 2 WHERE userGearRegistrationId = 1`)
	mustExec(t, db, `UPDATE user_gear_registrations SET userId = 1, maxContainerWeight = 500 WHERE userGearRegistrationId = 1`)
	if got := table(t, db, `SELECT userId, maxContainerWeight FROM user_gear_registrations WHERE userGearRegistrationId = 1`); !reflect.DeepEqual(got, []string{"1|500"}) {
		t.Errorf("registration 1 = %v", got)
	}
}

// BR-025 AC-7 (F2): the item ID sequence survives the rebuild. IDs up to 50
// were issued and 8..50 deleted before the migration; the next item gets an ID
// above 50 and an old ID addresses nothing.
func TestMigration11KeepsItemIDSequence(t *testing.T) {
	db := loadoutV10DB(t)
	if seq := scalar[int](t, db, `SELECT seq FROM sqlite_sequence WHERE name = 'loadout_items'`); seq != 50 {
		t.Fatalf("fixture sequence = %d, want 50", seq)
	}
	migrateTo(t, db, 11)

	if seq := scalar[int](t, db, `SELECT seq FROM sqlite_sequence WHERE name = 'loadout_items'`); seq != 50 {
		t.Errorf("sequence after migration = %d, want 50", seq)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM sqlite_sequence WHERE name = 'loadout_items'`); n != 1 {
		t.Errorf("%d sequence rows for loadout_items, want 1", n)
	}
	res := mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 1)`)
	if id, _ := res.LastInsertId(); id != 51 {
		t.Errorf("next item ID = %d, want 51", id)
	}
	for _, id := range []int{8, 43, 50} {
		for _, q := range []string{`UPDATE loadout_items SET ticked = 1 WHERE loadoutItemId = ?`, `DELETE FROM loadout_items WHERE loadoutItemId = ?`} {
			if n, _ := mustExec(t, db, q, id).RowsAffected(); n != 0 {
				t.Errorf("%s with old ID %d changed %d rows", q, id, n)
			}
		}
	}
}

// The sequence rule when the old table never issued an ID (a fresh database),
// and when the highest ID still exists.
func TestMigration11SequenceOnFreshAndFullDatabases(t *testing.T) {
	fresh := tempDB(t)
	runMigrate(t, fresh, migrationsPath(t))
	if v, want := scalar[int](t, fresh, `SELECT version FROM schema_migrations`), latestMigration(t); v != want {
		t.Fatalf("fresh database at version %d, want %d", v, want)
	}
	mustExec(t, fresh, `INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (1, 'a', '', 'a@example.com')`)
	mustExec(t, fresh, `INSERT INTO loadouts (userId, loadoutName) VALUES (1, 'L')`)
	mustExec(t, fresh, `INSERT INTO loadout_items (loadoutId, detachedReason) VALUES (1, 'legacy')`)
	if id := scalar[int](t, fresh, `SELECT MAX(loadoutItemId) FROM loadout_items`); id != 1 {
		t.Errorf("first item on a fresh database has ID %d, want 1", id)
	}

	db := loadoutV10DB(t)
	mustExec(t, db, `INSERT INTO loadout_items (loadoutItemId, loadoutId, gearId) VALUES (60, 1, 1)`) // above the old maximum
	migrateTo(t, db, 11)
	res := mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 1)`)
	if id, _ := res.LastInsertId(); id != 61 {
		t.Errorf("next item ID = %d, want 61", id)
	}
}

// BR-029 AC-3 and ARS-001 2.6: the item and on-your-back views.
func TestMigration11WeightViews(t *testing.T) {
	db := loadoutV11DB(t)

	// Registration weights: gear weight > 0 is the catalog weight, else none.
	if got := table(t, db, `SELECT registrationId, catalogWeight, unitWeight, weightSource FROM registration_effective_weight ORDER BY registrationId`); !reflect.DeepEqual(got, []string{
		"1|100|100|catalog", "2|200|200|catalog", "3|200|200|catalog", "4|200|200|catalog", "5|NULL|NULL|none", "6|NULL|NULL|none"}) {
		t.Errorf("registration weights = %v", got)
	}
	// Item weights: registration-backed from the registration, legacy live from the gear.
	if got := table(t, db, `SELECT loadoutItemId, unitWeight, weightSource FROM loadout_item_weight ORDER BY loadoutItemId`); !reflect.DeepEqual(got, []string{
		"1|100|catalog", "2|200|catalog", "3|300|catalog", "5|NULL|none", "6|200|catalog"}) {
		t.Errorf("item weights = %v", got)
	}
	// A legacy item follows its gear when the catalog weight changes.
	mustExec(t, db, `UPDATE gear SET gearWeight = 350 WHERE gearId = 4`)
	if got := table(t, db, `SELECT unitWeight FROM loadout_item_weight WHERE loadoutItemId = 3`); !reflect.DeepEqual(got, []string{"350"}) {
		t.Errorf("legacy item weight = %v, want 350", got)
	}
	mustExec(t, db, `UPDATE gear SET gearWeight = 300 WHERE gearId = 4`)

	onBack := func(loadout int) string {
		return scalar[string](t, db, `SELECT CAST(totalWeight AS TEXT) FROM loadout_on_back_weight WHERE loadoutId = ?`, loadout)
	}
	// Loadout 1: 2*100 + 200 + 300 + 3*0.
	if got := onBack(1); got != "700" {
		t.Errorf("on-back weight = %s, want 700", got)
	}
	if got := onBack(2); got != "0" {
		t.Errorf("empty loadout on-back weight = %s, want 0", got)
	}
	// An item in Worn does not count; the bag (registration 3, 200 g) counts once, its contents too.
	mustExec(t, db, `UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 2`, placeID(t, db, 1, "worn"))
	if got := onBack(1); got != "500" {
		t.Errorf("with one item worn = %s, want 500", got)
	}
	bag := addBag(t, db, 1, 3, 1)
	if got := onBack(1); got != "700" {
		t.Errorf("with a 200 g bag = %s, want 700", got)
	}
	mustExec(t, db, `UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 1`, bag)
	if got := onBack(1); got != "700" {
		t.Errorf("with an item in the bag = %s, want 700", got)
	}
	// The stored total is gone.
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM pragma_table_info('loadouts') WHERE name = 'totalWeight'`); n != 0 {
		t.Error("loadouts.totalWeight still exists")
	}
}

// BR-030: deleting a registration keeps its items as snapshots, removes its bag
// and empties what the bag held without losing anything else.
func TestMigration11RegistrationDeleteDetachesItems(t *testing.T) {
	db := loadoutV11DB(t)
	bag1 := addBag(t, db, 1, 1, 1) // registration 1 as a bag
	bag2 := addBag(t, db, 1, 2, 2)
	mustExec(t, db, `UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, bag1, bag2) // bag 2 inside bag 1
	mustExec(t, db, `UPDATE loadout_items SET placeId = ?, ticked = 1 WHERE loadoutItemId = 1`, bag1)
	mustExec(t, db, `UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 2`, bag1)

	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId = 1`)

	// Item 1 (registration 1) is a snapshot; item 2 (registration 2) is untouched but loose.
	if got := table(t, db, itemColumns+``); got[0] != "1|1|NULL|2|a|1|NULL|registration_deleted|1|g1|100|catalog" {
		t.Errorf("item 1 = %q", got[0])
	}
	if got := table(t, db, `SELECT loadoutItemId, registrationId, placeId, detachedReason FROM loadout_items WHERE loadoutItemId = 2`); !reflect.DeepEqual(got, []string{"2|2|NULL|NULL"}) {
		t.Errorf("item 2 = %v, want it kept, without a place", got)
	}
	// The bag row is gone; the bag inside it stays at the top.
	if got := table(t, db, `SELECT placeId, registrationId, parentPlaceId FROM loadout_places WHERE placeKind = 'bag'`); !reflect.DeepEqual(got, []string{fmt.Sprintf("%d|2|NULL", bag2)}) {
		t.Errorf("bags after deleting registration 1 = %v", got)
	}
	// The snapshot weighs what it did.
	if got := table(t, db, `SELECT unitWeight, weightSource FROM loadout_item_weight WHERE loadoutItemId = 1`); !reflect.DeepEqual(got, []string{"100|catalog"}) {
		t.Errorf("snapshot weight = %v", got)
	}

	// No catalog weight: the snapshot has no weight and no source. Gear missing:
	// no gear reference and the name 'Unknown gear'.
	mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 5)`)
	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId IN (5, 6)`)
	if got := table(t, db, `SELECT detachedReason, detachedGearId, detachedGearName, detachedUnitWeight, detachedWeightSource, registrationId
		FROM loadout_items WHERE loadoutItemId = 5 OR loadoutItemId > 50 ORDER BY loadoutItemId`); !reflect.DeepEqual(got, []string{
		"registration_deleted|NULL|Unknown gear|NULL|NULL|NULL",
		"registration_deleted|3|g3|NULL|NULL|NULL"}) {
		t.Errorf("snapshots without weight or gear = %v", got)
	}
	if got := table(t, db, `SELECT unitWeight, weightSource FROM loadout_item_weight WHERE loadoutItemId = 5`); !reflect.DeepEqual(got, []string{"NULL|none"}) {
		t.Errorf("snapshot of missing gear weighs %v, want none", got)
	}

	// Nothing but the deleted registrations' bags and the place links changed.
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM loadout_items`); n != 6 {
		t.Errorf("%d items, want 6", n)
	}
}

// ADR-004 D-2 / F3: gear that a legacy or registration-deleted item still
// names cannot be deleted; the foreign key refuses it (the catalog maps that
// to 409), and it can be once the item is gone.
func TestMigration11GearInUseByDetachedItemsIsRefused(t *testing.T) {
	db := loadoutV11DB(t)

	mustFail(t, db, "FOREIGN KEY constraint failed", `DELETE FROM gear WHERE gearId = 4`) // legacy item 3
	mustExec(t, db, `DELETE FROM loadout_items WHERE loadoutItemId = 3`)
	mustExec(t, db, `DELETE FROM gear WHERE gearId = 4`)

	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId = 1`) // item 1 becomes a snapshot of gear 1
	mustFail(t, db, "FOREIGN KEY constraint failed", `DELETE FROM gear WHERE gearId = 1`)
	mustExec(t, db, `DELETE FROM loadout_items WHERE loadoutItemId = 1`)
	mustExec(t, db, `DELETE FROM gear WHERE gearId = 1`)
}

// Account deletion order (ADR-004): loadouts first, which removes their items
// and places, then registrations and gear; nothing is refused.
func TestMigration11DeletingLoadoutsRemovesPlacesAndItems(t *testing.T) {
	db := loadoutV11DB(t)
	bag := addBag(t, db, 1, 1, 1)
	mustExec(t, db, `UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 1`, bag)

	mustExec(t, db, `DELETE FROM loadouts WHERE userId = 1`)
	if got := table(t, db, `SELECT COUNT(*) FROM loadout_places WHERE loadoutId IN (1, 2)`); !reflect.DeepEqual(got, []string{"0"}) {
		t.Errorf("places left after deleting loadouts: %v", got)
	}
	if got := table(t, db, `SELECT loadoutItemId FROM loadout_items`); !reflect.DeepEqual(got, []string{"6"}) {
		t.Errorf("items left = %v, want only user 2's item 6", got)
	}
	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userId = 1`)
}

// Down (ARS-001 2.1): the schema order is safe with data present, the old item
// shape and its triggers return, legacy and snapshot items keep their gear when
// it exists, the stored total is recomputed and the sequence is carried.
func TestMigration11DownRestoresVersion10Shape(t *testing.T) {
	db := loadoutV11DB(t)
	bag := addBag(t, db, 1, 3, 1)
	mustExec(t, db, `UPDATE loadout_items SET placeId = ?, ticked = 1 WHERE loadoutItemId = 1`, bag)
	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId = 6`) // item 5: snapshot of missing gear
	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId = 4`) // item 6: snapshot of gear 2
	mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 2)`)  // item 51

	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Steps(-1) })

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 10 {
		t.Fatalf("version after down = %d, want 10", v)
	}
	if d := scalar[bool](t, db, `SELECT dirty FROM schema_migrations`); d {
		t.Fatal("down left the database dirty")
	}
	// Items 1, 2, 3 (legacy gear 4), 6 (snapshot of gear 2) and 51 keep a gear; item 5 (no gear) is dropped.
	if got := table(t, db, `SELECT loadoutItemId, loadoutId, gearId, quantity, notes FROM loadout_items ORDER BY loadoutItemId`); !reflect.DeepEqual(got, []string{
		"1|1|1|2|a", "2|1|2|1|b", "3|1|4|1|legacy", "6|3|2|1|u2", "51|1|2|1|"}) {
		t.Errorf("items after down = %v", got)
	}
	// Totals again: 2*100 + 200 + 300 + 200 (item 51) = 900.
	if got := table(t, db, `SELECT loadoutId, totalWeight FROM loadouts ORDER BY loadoutId`); !reflect.DeepEqual(got, []string{"1|900", "2|0", "3|200"}) {
		t.Errorf("recomputed totals = %v", got)
	}
	if seq := scalar[int](t, db, `SELECT seq FROM sqlite_sequence WHERE name = 'loadout_items'`); seq != 51 {
		t.Errorf("sequence after down = %d, want 51", seq)
	}
	// Nothing of 000011 remains, and the 000005 quantity rules are back.
	if got := table(t, db, `SELECT type, name FROM sqlite_master WHERE name LIKE 'loadout_places%' OR name LIKE 'idx_loadout_places%'
		OR name LIKE 'trg_loadout_places%' OR name LIKE 'trg_registration_%' OR name = 'trg_loadouts_create_worn'
		OR name IN ('loadout_item_weight', 'loadout_on_back_weight', 'registration_effective_weight')
		OR name LIKE 'trg_loadout_items_owner%' OR name LIKE 'trg_loadout_items_place%' OR name = 'loadout_items_v11' OR name = 'loadout_items_v10'`); len(got) != 0 {
		t.Errorf("objects left behind by down: %v", got)
	}
	if got := table(t, db, `SELECT name FROM sqlite_master WHERE type = 'trigger' AND tbl_name = 'loadout_items' ORDER BY name`); !reflect.DeepEqual(got,
		[]string{"trg_loadout_items_quantity_insert", "trg_loadout_items_quantity_update"}) {
		t.Errorf("item triggers after down = %v", got)
	}
	mustFail(t, db, "quantity must be an integer greater than 0", `INSERT INTO loadout_items (loadoutId, gearId, quantity) VALUES (1, 1, 0)`)
	mustFail(t, db, "NOT NULL", `INSERT INTO loadout_items (loadoutId) VALUES (1)`)
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name IN ('idx_loadout_items_loadout', 'idx_loadout_items_gear')`); n != 2 {
		t.Errorf("%d of the 2 old item indexes exist", n)
	}
	res := mustExec(t, db, `INSERT INTO loadout_items (loadoutId, gearId) VALUES (1, 1)`)
	if id, _ := res.LastInsertId(); id != 52 {
		t.Errorf("next item ID after down = %d, want 52", id)
	}

	// And up again from the rolled-back database.
	migrateTo(t, db, 11)
	if got := table(t, db, `SELECT loadoutId, placeKind FROM loadout_places ORDER BY loadoutId`); !reflect.DeepEqual(got, []string{"1|worn", "2|worn", "3|worn"}) {
		t.Errorf("places after up again = %v", got)
	}
}

// The full down path (TestMigrationsUpDown) must also work with data in the
// 000011 tables, and a second Up must change nothing.
func TestMigration11FullDownAndUpWithData(t *testing.T) {
	db := loadoutV11DB(t)
	bag := addBag(t, db, 1, 1, 1)
	mustExec(t, db, `UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 1`, bag)

	runMigrate(t, db, migrationsPath(t)) // already at the latest version
	if v, want := scalar[int](t, db, `SELECT version FROM schema_migrations`), latestMigration(t); v != want {
		t.Errorf("version after a second Up = %d, want %d", v, want)
	}
	runMigrateDown(t, db, migrationsPath(t))
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' AND name NOT IN ('schema_migrations', 'audit_000005_clamped_loadout_item_quantity')`); n != 0 {
		t.Errorf("%d tables or views left after the full down", n)
	}
}
