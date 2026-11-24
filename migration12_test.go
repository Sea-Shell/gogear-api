package main

import (
	"database/sql"
	"reflect"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
)

// Migration 000012 (ARS-001 sections 2.2 and 2.6, BR-026, BR-027, BR-028,
// BR-036). The tests start from the version 11 database of migration11_test.go
// (registrations 1 to 6, loadouts 1 to 3, items 1, 2, 3 (legacy), 5 and 6)
// with this addition, made before 000012 runs so the migration meets real data:
//
//	registration 3 (gear 2, 200 g) is a bag of loadout 1 and holds item 1
//
// Effective weights before and after the migration must be identical.
func loadoutV11DBWithBag(t *testing.T) *sql.DB {
	t.Helper()
	db := loadoutV11DB(t)
	bag := addBag(t, db, 1, 3, 1)
	mustExec(t, db, `UPDATE loadout_items SET placeId = ? WHERE loadoutItemId = 1`, bag)
	return db
}

const (
	registrationWeights = `SELECT registrationId, catalogWeight, unitWeight, weightSource FROM registration_effective_weight ORDER BY registrationId`
	itemWeights         = `SELECT loadoutItemId, unitWeight, weightSource FROM loadout_item_weight ORDER BY loadoutItemId`
	onBackWeights       = `SELECT loadoutId, totalWeight FROM loadout_on_back_weight ORDER BY loadoutId`
	dependentObjects    = `SELECT name FROM sqlite_master WHERE name IN
		('loadout_item_weight', 'loadout_on_back_weight', 'trg_registration_delete_detach', 'trg_registration_owner_fixed') ORDER BY name`
)

// The migration adds columns with defaults, gives every existing registration
// closet count 1 (BR-036 AC-1), and replaces the view without recreating the
// views and the delete trigger that name it. Nothing they report changes.
func TestMigration12KeepsExistingDataAndDependentViews(t *testing.T) {
	db := loadoutV11DBWithBag(t)
	beforeRegs := table(t, db, registrationWeights)
	beforeItems := table(t, db, itemWeights)
	beforeTotals := table(t, db, onBackWeights)
	beforeObjects := table(t, db, dependentObjects)
	beforeFK := table(t, db, `PRAGMA foreign_key_check`) // the seed has a registration of missing gear 99
	if len(beforeObjects) != 4 {
		t.Fatalf("objects the migration must keep, before it runs: %v", beforeObjects)
	}

	migrateTo(t, db, 12)

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 12 {
		t.Fatalf("version = %d, want 12", v)
	}
	if d := scalar[bool](t, db, `SELECT dirty FROM schema_migrations`); d {
		t.Fatal("migration left the database dirty")
	}
	// BR-036 AC-1: every existing registration has count 1 and no measured weight.
	if got := table(t, db, `SELECT userGearRegistrationId, closetCount, measuredWeight, measuredWeightSource FROM user_gear_registrations ORDER BY 1`); !reflect.DeepEqual(got, []string{
		"1|1|NULL|NULL", "2|1|NULL|NULL", "3|1|NULL|NULL", "4|1|NULL|NULL", "5|1|NULL|NULL", "6|1|NULL|NULL"}) {
		t.Errorf("registrations after migration = %v", got)
	}
	// Existing loadouts are in planning with no dates or goal; existing items are not consumable.
	if got := table(t, db, `SELECT loadoutId, stage, doneAt, startDate, endDate, weightGoal FROM loadouts ORDER BY 1`); !reflect.DeepEqual(got, []string{
		"1|planning|NULL|NULL|NULL|NULL", "2|planning|NULL|NULL|NULL|NULL", "3|planning|NULL|NULL|NULL|NULL"}) {
		t.Errorf("loadouts after migration = %v", got)
	}
	if got := table(t, db, `SELECT DISTINCT consumable FROM loadout_items`); !reflect.DeepEqual(got, []string{"0"}) {
		t.Errorf("consumable of existing items = %v, want only 0", got)
	}

	// The replaced view, and the views and trigger that name it, still work and
	// report what they reported before.
	if got := table(t, db, registrationWeights); !reflect.DeepEqual(got, beforeRegs) {
		t.Errorf("registration weights changed:\n got %q\nwant %q", got, beforeRegs)
	}
	if got := table(t, db, itemWeights); !reflect.DeepEqual(got, beforeItems) {
		t.Errorf("item weights changed:\n got %q\nwant %q", got, beforeItems)
	}
	if got := table(t, db, onBackWeights); !reflect.DeepEqual(got, beforeTotals) {
		t.Errorf("on-back weights changed:\n got %q\nwant %q", got, beforeTotals)
	}
	if got := table(t, db, dependentObjects); !reflect.DeepEqual(got, beforeObjects) {
		t.Errorf("objects after migration = %v, want %v", got, beforeObjects)
	}
	// The delete trigger still snapshots (registration 2 backs item 2 and has no measured weight).
	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId = 2`)
	if got := table(t, db, `SELECT detachedReason, detachedGearName, detachedUnitWeight, detachedWeightSource FROM loadout_items WHERE loadoutItemId = 2`); !reflect.DeepEqual(got, []string{"registration_deleted|g2|200|catalog"}) {
		t.Errorf("snapshot after migration = %v", got)
	}
	if got := table(t, db, `PRAGMA integrity_check`); !reflect.DeepEqual(got, []string{"ok"}) {
		t.Errorf("integrity_check = %v", got)
	}
	if got := table(t, db, `PRAGMA foreign_key_check`); !reflect.DeepEqual(got, beforeFK) {
		t.Errorf("foreign_key_check = %v, want what it was before the migration: %v", got, beforeFK)
	}
}

// A column default applies to rows inserted later too.
func TestMigration12Defaults(t *testing.T) {
	db := loadoutV11DB(t)
	migrateTo(t, db, 12)

	mustExec(t, db, `INSERT INTO user_gear_registrations (gearId, userId) VALUES (1, 1)`)
	mustExec(t, db, `INSERT INTO loadouts (userId, loadoutName) VALUES (1, 'new')`)
	mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId) VALUES (1, 1)`)
	if got := table(t, db, `SELECT closetCount, measuredWeight, measuredWeightSource FROM user_gear_registrations ORDER BY userGearRegistrationId DESC LIMIT 1`); !reflect.DeepEqual(got, []string{"1|NULL|NULL"}) {
		t.Errorf("new registration = %v", got)
	}
	if got := table(t, db, `SELECT stage, doneAt, startDate, endDate, weightGoal FROM loadouts ORDER BY loadoutId DESC LIMIT 1`); !reflect.DeepEqual(got, []string{"planning|NULL|NULL|NULL|NULL"}) {
		t.Errorf("new loadout = %v", got)
	}
	if got := table(t, db, `SELECT consumable FROM loadout_items ORDER BY loadoutItemId DESC LIMIT 1`); !reflect.DeepEqual(got, []string{"0"}) {
		t.Errorf("new item = %v", got)
	}
}

// BR-026 AC-1, BR-036 AC-1, BR-027, BR-028: the database refuses what the API
// refuses, so a handler that misses a check cannot store a bad value.
func TestMigration12ColumnRules(t *testing.T) {
	db := loadoutV11DB(t)
	migrateTo(t, db, 12)

	// Measured weight: whole grams, at least 1, and always with its source.
	for _, v := range []string{"0", "-5", "1.5", "'abc'", "'1.5'"} {
		mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeight = `+v+`, measuredWeightSource = 'weighed' WHERE userGearRegistrationId = 1`)
	}
	mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeight = 5 WHERE userGearRegistrationId = 1`)                                    // a value without a source
	mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeightSource = 'guess' WHERE userGearRegistrationId = 1`)                        // a source without a value
	mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeight = 5, measuredWeightSource = 'measured' WHERE userGearRegistrationId = 1`) // any other source
	mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeight = 5, measuredWeightSource = 'catalog' WHERE userGearRegistrationId = 1`)  // 'catalog' is a reported source, never a stored one
	mustFail(t, db, "CHECK", `INSERT INTO user_gear_registrations (gearId, userId, measuredWeight) VALUES (1, 1, 5)`)
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 687, measuredWeightSource = 'weighed' WHERE userGearRegistrationId = 1`)
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 650, measuredWeightSource = 'guess' WHERE userGearRegistrationId = 1`)
	mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeightSource = NULL WHERE userGearRegistrationId = 1`) // dropping only the source
	mustFail(t, db, "CHECK", `UPDATE user_gear_registrations SET measuredWeight = NULL WHERE userGearRegistrationId = 1`)       // dropping only the value
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = NULL, measuredWeightSource = NULL WHERE userGearRegistrationId = 1`)

	// Closet count: whole number, at least 1, never NULL.
	for _, v := range []string{"0", "-1", "2.5", "'x'", "NULL"} {
		mustFail(t, db, "", `UPDATE user_gear_registrations SET closetCount = `+v+` WHERE userGearRegistrationId = 1`)
	}
	mustExec(t, db, `UPDATE user_gear_registrations SET closetCount = 8 WHERE userGearRegistrationId = 1`)

	// Consumable role.
	mustFail(t, db, "CHECK", `UPDATE loadout_items SET consumable = 2 WHERE loadoutItemId = 1`)
	mustFail(t, db, "", `UPDATE loadout_items SET consumable = NULL WHERE loadoutItemId = 1`)
	mustExec(t, db, `UPDATE loadout_items SET consumable = 1 WHERE loadoutItemId = 1`)

	// Stage, dates and goal.
	mustFail(t, db, "CHECK", `UPDATE loadouts SET stage = 'finished' WHERE loadoutId = 1`)
	mustFail(t, db, "", `UPDATE loadouts SET stage = NULL WHERE loadoutId = 1`)
	for _, d := range []string{"'2026-02-30'", "'2026-2-3'", "'2026-02-03T10:00:00Z'", "'tomorrow'", "20260203"} {
		mustFail(t, db, "CHECK", `UPDATE loadouts SET startDate = `+d+` WHERE loadoutId = 1`)
		mustFail(t, db, "CHECK", `UPDATE loadouts SET endDate = `+d+` WHERE loadoutId = 1`)
	}
	mustExec(t, db, `UPDATE loadouts SET startDate = '2026-07-10', endDate = '2026-07-10' WHERE loadoutId = 1`) // a one-day trip
	mustFail(t, db, "CHECK", `UPDATE loadouts SET endDate = '2026-07-09' WHERE loadoutId = 1`)                  // end before start
	mustFail(t, db, "CHECK", `UPDATE loadouts SET startDate = '2026-07-11' WHERE loadoutId = 1`)                // start after the stored end
	mustExec(t, db, `UPDATE loadouts SET startDate = NULL WHERE loadoutId = 1`)                                 // an end date alone is allowed
	mustExec(t, db, `UPDATE loadouts SET endDate = NULL WHERE loadoutId = 1`)
	for _, g := range []string{"0", "-1", "1.5", "'x'"} {
		mustFail(t, db, "CHECK", `UPDATE loadouts SET weightGoal = `+g+` WHERE loadoutId = 1`)
	}
	mustExec(t, db, `UPDATE loadouts SET weightGoal = 4500, stage = 'done', doneAt = '2026-07-12T10:00:00Z' WHERE loadoutId = 1`)
	mustExec(t, db, `UPDATE loadouts SET weightGoal = NULL, stage = 'packing', doneAt = NULL WHERE loadoutId = 1`)
}

// BR-026 AC-1, AC-3, AC-4 and BR-029 AC-3 at the view level: a measured weight
// replaces the catalog weight in the effective weight of every item and bag
// that uses the registration, the catalog column is untouched, and removing it
// returns the catalog weight. The 1 994 g example of BR-029 is built in
// pkg/api; this pins the arithmetic the views own.
func TestMigration12MeasuredWeightInViews(t *testing.T) {
	db := loadoutV11DBWithBag(t)
	migrateTo(t, db, 12)
	onBack := func(loadout int) string {
		return scalar[string](t, db, `SELECT CAST(totalWeight AS TEXT) FROM loadout_on_back_weight WHERE loadoutId = ?`, loadout)
	}
	// Loadout 1: item 1 (reg 1, 100 g) x2 in the bag reg 3 (200 g), item 2 (reg 2, 200 g),
	// legacy item 3 (300 g), item 5 x3 (no weight) = 200 + 200 + 200 + 300.
	if got := onBack(1); got != "900" {
		t.Fatalf("on-back weight before any measured weight = %s, want 900", got)
	}

	// Measured below the catalog weight: registration 1 weighs 80 g.
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 80, measuredWeightSource = 'weighed' WHERE userGearRegistrationId = 1`)
	if got := table(t, db, `SELECT catalogWeight, unitWeight, weightSource FROM registration_effective_weight WHERE registrationId = 1`); !reflect.DeepEqual(got, []string{"100|80|weighed"}) {
		t.Errorf("registration 1 = %v, want the catalog weight kept and 80 weighed", got)
	}
	if got := table(t, db, `SELECT unitWeight, weightSource FROM loadout_item_weight WHERE loadoutItemId = 1`); !reflect.DeepEqual(got, []string{"80|weighed"}) {
		t.Errorf("item 1 = %v", got)
	}
	if got := onBack(1); got != "860" { // 2 x 80 instead of 2 x 100
		t.Errorf("on-back weight = %s, want 860", got)
	}

	// A measured weight on gear that has no catalog weight (registration 5), and on the bag (registration 3).
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 40, measuredWeightSource = 'guess' WHERE userGearRegistrationId = 5`)
	if got := table(t, db, `SELECT catalogWeight, unitWeight, weightSource FROM registration_effective_weight WHERE registrationId = 5`); !reflect.DeepEqual(got, []string{"NULL|40|guess"}) {
		t.Errorf("registration 5 = %v, want no catalog weight and 40 guess", got)
	}
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 150, measuredWeightSource = 'weighed' WHERE userGearRegistrationId = 3`)
	if got := onBack(1); got != "810" { // the bag now counts 150, not 200
		t.Errorf("on-back weight with a measured bag = %s, want 810", got)
	}

	// Two users who own the same gear each keep their own weight (BR-026 AC-3): registrations 2 (user 1) and 4 (user 2).
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 190, measuredWeightSource = 'guess' WHERE userGearRegistrationId = 4`)
	if got := table(t, db, `SELECT registrationId, unitWeight, weightSource FROM registration_effective_weight WHERE registrationId IN (2, 4) ORDER BY 1`); !reflect.DeepEqual(got, []string{"2|200|catalog", "4|190|guess"}) {
		t.Errorf("two owners of gear 2 = %v", got)
	}
	if got := table(t, db, `SELECT gearWeight FROM gear WHERE gearId IN (1, 2, 3) ORDER BY gearId`); !reflect.DeepEqual(got, []string{"100", "200", "0"}) {
		t.Errorf("catalog weights changed: %v", got)
	}

	// The measured weight outlives a catalog change; the catalog column follows it.
	mustExec(t, db, `UPDATE gear SET gearWeight = 110 WHERE gearId = 1`)
	if got := table(t, db, `SELECT catalogWeight, unitWeight, weightSource FROM registration_effective_weight WHERE registrationId = 1`); !reflect.DeepEqual(got, []string{"110|80|weighed"}) {
		t.Errorf("after a catalog change = %v", got)
	}
	mustExec(t, db, `UPDATE gear SET gearWeight = 100 WHERE gearId = 1`)

	// A legacy item is not a registration and is never affected.
	if got := table(t, db, `SELECT unitWeight, weightSource FROM loadout_item_weight WHERE loadoutItemId = 3`); !reflect.DeepEqual(got, []string{"300|catalog"}) {
		t.Errorf("legacy item = %v", got)
	}

	// Removing it returns the catalog weight (BR-026 AC-1), and the source with it.
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = NULL, measuredWeightSource = NULL WHERE userGearRegistrationId IN (1, 3, 5)`)
	if got := table(t, db, `SELECT registrationId, unitWeight, weightSource FROM registration_effective_weight WHERE registrationId IN (1, 3, 5) ORDER BY 1`); !reflect.DeepEqual(got, []string{"1|100|catalog", "3|200|catalog", "5|NULL|none"}) {
		t.Errorf("after removing the measured weights = %v", got)
	}
	if got := onBack(1); got != "900" {
		t.Errorf("on-back weight after removing = %s, want 900", got)
	}
}

// BR-030 AC-1 with BR-026: deleting a registration that has a measured weight
// keeps that weight and its source on the items, and a later change of the
// catalog does not reach them. The trigger of 000011 is unchanged; it reads
// the replaced view.
func TestMigration12RegistrationDeleteSnapshotsMeasuredWeight(t *testing.T) {
	db := loadoutV11DBWithBag(t)
	migrateTo(t, db, 12)
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 650, measuredWeightSource = 'guess' WHERE userGearRegistrationId = 1`)
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 33, measuredWeightSource = 'weighed' WHERE userGearRegistrationId = 5`) // gear with no catalog weight
	mustExec(t, db, `INSERT INTO loadout_items (loadoutId, registrationId, quantity) VALUES (1, 5, 2)`)                                          // item 51
	onBack := func() string {
		return scalar[string](t, db, `SELECT CAST(totalWeight AS TEXT) FROM loadout_on_back_weight WHERE loadoutId = 1`)
	}
	// 2 x 650 (item 1, in the 200 g bag) + 200 bag + 200 (item 2) + 300 (legacy) + 3 x 0 (item 5) + 2 x 33 (item 51).
	const total = "2066"
	if got := onBack(); got != total {
		t.Fatalf("on-back weight before the delete = %s, want %s", got, total)
	}

	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId IN (1, 5)`)

	if got := table(t, db, `SELECT loadoutItemId, registrationId, detachedReason, detachedGearName, detachedUnitWeight, detachedWeightSource
		FROM loadout_items WHERE loadoutItemId = 1 OR loadoutItemId > 50 ORDER BY loadoutItemId`); !reflect.DeepEqual(got, []string{
		"1|NULL|registration_deleted|g1|650|guess",
		"51|NULL|registration_deleted|g3|33|weighed"}) {
		t.Errorf("snapshots = %v", got)
	}
	frozen := []string{"1|650|guess", "51|33|weighed"}
	if got := table(t, db, `SELECT loadoutItemId, unitWeight, weightSource FROM loadout_item_weight WHERE loadoutItemId = 1 OR loadoutItemId > 50 ORDER BY 1`); !reflect.DeepEqual(got, frozen) {
		t.Errorf("snapshot weights = %v", got)
	}
	if got := onBack(); got != total { // the kept weight still counts (BR-030 AC-1)
		t.Errorf("on-back weight after the delete = %s, want %s", got, total)
	}
	mustExec(t, db, `UPDATE gear SET gearWeight = 999 WHERE gearId IN (1, 3)`)
	if got := table(t, db, `SELECT loadoutItemId, unitWeight, weightSource FROM loadout_item_weight WHERE loadoutItemId = 1 OR loadoutItemId > 50 ORDER BY 1`); !reflect.DeepEqual(got, frozen) {
		t.Errorf("snapshot weights after a catalog change = %v, want them frozen", got)
	}
}

// 000012 down: the columns and the measured-weight view go, the milestone A
// view returns, data of 000011 stays, and 000012 applies again with defaults.
func TestMigration12DownRestoresVersion11Shape(t *testing.T) {
	db := loadoutV11DBWithBag(t)
	migrateTo(t, db, 12)
	mustExec(t, db, `UPDATE user_gear_registrations SET measuredWeight = 80, measuredWeightSource = 'weighed', closetCount = 5 WHERE userGearRegistrationId = 1`)
	mustExec(t, db, `UPDATE loadouts SET stage = 'packing', startDate = '2026-07-01', endDate = '2026-07-09', weightGoal = 5000 WHERE loadoutId = 1`)
	mustExec(t, db, `UPDATE loadout_items SET consumable = 1 WHERE loadoutItemId = 2`)

	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Steps(-1) })

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 11 {
		t.Fatalf("version after down = %d, want 11", v)
	}
	if d := scalar[bool](t, db, `SELECT dirty FROM schema_migrations`); d {
		t.Fatal("down left the database dirty")
	}
	for tbl, cols := range map[string][]string{
		"user_gear_registrations": {"measuredWeight", "measuredWeightSource", "closetCount"},
		"loadout_items":           {"consumable"},
		"loadouts":                {"stage", "doneAt", "startDate", "endDate", "weightGoal"},
	} {
		for _, c := range cols {
			if n := scalar[int](t, db, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, tbl, c); n != 0 {
				t.Errorf("%s.%s still exists after down", tbl, c)
			}
		}
	}
	// The milestone A view: registration 1 weighs its catalog weight again; the rest of 000011 works.
	if got := table(t, db, `SELECT catalogWeight, unitWeight, weightSource FROM registration_effective_weight WHERE registrationId = 1`); !reflect.DeepEqual(got, []string{"100|100|catalog"}) {
		t.Errorf("registration 1 after down = %v", got)
	}
	if got := table(t, db, onBackWeights); !reflect.DeepEqual(got, []string{"1|900", "2|0", "3|200"}) {
		t.Errorf("on-back weights after down = %v", got)
	}
	mustExec(t, db, `DELETE FROM user_gear_registrations WHERE userGearRegistrationId = 1`)
	if got := table(t, db, `SELECT detachedUnitWeight, detachedWeightSource FROM loadout_items WHERE loadoutItemId = 1`); !reflect.DeepEqual(got, []string{"100|catalog"}) {
		t.Errorf("snapshot after down = %v", got)
	}

	// And up again from the rolled-back database: defaults, no trace of the earlier values.
	migrateTo(t, db, 12)
	if got := table(t, db, `SELECT userGearRegistrationId, closetCount, measuredWeight FROM user_gear_registrations ORDER BY 1`); !reflect.DeepEqual(got, []string{
		"2|1|NULL", "3|1|NULL", "4|1|NULL", "5|1|NULL", "6|1|NULL"}) {
		t.Errorf("registrations after up again = %v", got)
	}
	if got := table(t, db, `SELECT stage, startDate, weightGoal FROM loadouts WHERE loadoutId = 1`); !reflect.DeepEqual(got, []string{"planning|NULL|NULL"}) {
		t.Errorf("loadout after up again = %v", got)
	}
}
