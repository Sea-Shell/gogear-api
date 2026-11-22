package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
)

// Migration 000009 (ADR-001, BR-021): category settings, the two views, the
// link triggers and the rename of the old per-gear flag. These tests run on a
// version 8 database whose gear carries old flags and whose links already
// point into gear that will not be container-capable.

// containerV8DB builds a version 8 database and returns it still at version 8.
//
//	gear 1 'flagged'  flag 1, category 1 (top 1)
//	gear 2 'plain'    flag 0, category 1 (top 1)
//	gear 3 'other'    flag 1, category 2 (top 2)
//	gear 4 'dangling' flag 1, category 999 (no such row, inserted with foreign keys off)
//
// Registrations 1..5 (gear 1,2,3,4,2) belong to user 1; links 1->2 and 3->4
// (container -> content) already exist.
func containerV8DB(t *testing.T) *sql.DB {
	t.Helper()
	file := filepath.Join(t.TempDir(), "v8.db")
	raw, err := sql.Open("sqlite3", file+"?_foreign_keys=off")
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	stepMigrate(t, raw, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(8) })
	if _, err := raw.Exec(`
INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (1, 'a', '', 'a@example.com');
INSERT INTO gear_top_category (topCategoryId, topCategoryName) VALUES (1, 't1'), (2, 't2');
INSERT INTO gear_category (categoryId, categoryTopCategoryId, categoryName) VALUES (1, 1, 'c1'), (2, 2, 'c2');
INSERT INTO manufacture (manufactureId, manufactureName) VALUES (1, 'm');
INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearIsContainer, gearName, gearWeight) VALUES
	(1, 1, 1, 1, 1, 'flagged', 100), (2, 1, 1, 1, 0, 'plain', 100), (3, 2, 2, 1, 1, 'other', 100), (4, 1, 999, 1, 1, 'dangling', 100);
INSERT INTO user_gear_registrations (userGearRegistrationId, gearId, userId) VALUES (1, 1, 1), (2, 2, 1), (3, 3, 1), (4, 4, 1), (5, 2, 1);
INSERT INTO user_container_registration (containerRegistrationId, userContainerId, userGearRegistrationId) VALUES (1, 1, 2), (2, 3, 4);
`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	raw.Close()
	return openEnforced(t, file)
}

func migrateTo9(t *testing.T, db *sql.DB) {
	t.Helper()
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(9) })
}

func columnValues(t *testing.T, db *sql.DB, q string) []int {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v sql.NullInt64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v.Valid {
			out = append(out, int(v.Int64))
		} else {
			out = append(out, -1) // NULL
		}
	}
	return out
}

// schemaSQL is every schema object's SQL text, so a replay that changes
// nothing can be told from one that did.
func schemaSQL(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT type||' '||name||' '||COALESCE(sql, '') FROM sqlite_master ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		sb.WriteString(s + "\n")
	}
	return sb.String()
}

// BR-021 AC-6 / P-15 and the superseded P-14 start values, at version 9 only:
// right after 000009 the settings are "not a container", the old flags stay
// recoverable, and no link or registration is touched. The amended AC-5 / P-18
// seeding is migration 000010 and is tested in migration10_test.go; a database
// at version 10 or later no longer shows these start values.
func TestMigration9StartValuesLegacyFlagsAndLinks(t *testing.T) {
	db := containerV8DB(t)
	migrateTo9(t, db)

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 9 {
		t.Fatalf("version = %d, want 9", v)
	}
	if got := columnValues(t, db, `SELECT topCategoryIsContainer FROM gear_top_category ORDER BY topCategoryId`); !reflect.DeepEqual(got, []int{0, 0}) {
		t.Errorf("top category settings = %v, want all 0", got)
	}
	if got := columnValues(t, db, `SELECT categoryIsContainer FROM gear_category ORDER BY categoryId`); !reflect.DeepEqual(got, []int{-1, -1}) {
		t.Errorf("sub-category settings = %v, want all NULL (no explicit setting)", got)
	}
	// Every gear reports "not a container", whatever its old flag said, and the
	// dangling sub-category (P-13) is not capable either.
	if got := columnValues(t, db, `SELECT isContainer FROM gear_container_capability ORDER BY gearId`); !reflect.DeepEqual(got, []int{0, 0, 0, 0}) {
		t.Errorf("derived status = %v, want all 0", got)
	}
	// The old values are recoverable under the new name; the old name is gone.
	if got := columnValues(t, db, `SELECT gearIsContainerLegacy FROM gear ORDER BY gearId`); !reflect.DeepEqual(got, []int{1, 0, 1, 1}) {
		t.Errorf("legacy flags = %v, want [1 0 1 1]", got)
	}
	if _, err := db.Query(`SELECT gearIsContainer FROM gear`); err == nil || !strings.Contains(err.Error(), "no such column") {
		t.Errorf("old column name still resolves: %v", err)
	}
	// Links and registrations are untouched.
	if got := linkIDs(t, db); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("links = %v, want [1 2]", got)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_gear_registrations`); n != 5 {
		t.Errorf("registrations = %d", n)
	}
	// The column CHECKs hold.
	mustFail(t, db, "CHECK constraint failed", `UPDATE gear_top_category SET topCategoryIsContainer = 2`)
	mustFail(t, db, "CHECK constraint failed", `UPDATE gear_category SET categoryIsContainer = 2`)
}

// ADR-001 D-3: the views apply explicit setting > top category > not a container.
func TestMigration9ViewsApplyInheritance(t *testing.T) {
	db := containerV8DB(t)
	migrateTo9(t, db)
	status := func() []int {
		return columnValues(t, db, `SELECT isContainer FROM gear_container_capability ORDER BY gearId`)
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	// Top category 1 is a container: both gear of category 1 inherit it; the
	// gear of category 2 (top 2) and the dangling one do not.
	exec(`UPDATE gear_top_category SET topCategoryIsContainer = 1 WHERE topCategoryId = 1`)
	if got := status(); !reflect.DeepEqual(got, []int{1, 1, 0, 0}) {
		t.Errorf("inherited = %v, want [1 1 0 0]", got)
	}
	// An explicit "not a container" overrides the top category.
	exec(`UPDATE gear_category SET categoryIsContainer = 0 WHERE categoryId = 1`)
	if got := status(); !reflect.DeepEqual(got, []int{0, 0, 0, 0}) {
		t.Errorf("explicit false = %v, want all 0", got)
	}
	// An explicit "container" overrides a top category that is not.
	exec(`UPDATE gear_category SET categoryIsContainer = 1 WHERE categoryId = 2`)
	if got := status(); !reflect.DeepEqual(got, []int{0, 0, 1, 0}) {
		t.Errorf("explicit true = %v, want [0 0 1 0]", got)
	}
	// The gear's own top-category reference has no effect (AC-3).
	exec(`UPDATE gear SET gearTopCategoryId = 2 WHERE gearId = 3`)
	exec(`UPDATE gear SET gearTopCategoryId = 1 WHERE gearId = 2`)
	if got := status(); !reflect.DeepEqual(got, []int{0, 0, 1, 0}) {
		t.Errorf("top reference changed status: %v", got)
	}
	// Removing the explicit setting falls back to the top category.
	exec(`UPDATE gear_category SET categoryIsContainer = NULL WHERE categoryId = 1`)
	if got := status(); !reflect.DeepEqual(got, []int{1, 1, 1, 0}) {
		t.Errorf("fallback = %v, want [1 1 1 0]", got)
	}
}

// ADR-001 D-7 (this is also the proof that the bundled SQLite supports what
// the ADR relies on: column CHECKs in ALTER TABLE ADD COLUMN, RENAME COLUMN,
// views read inside triggers, DROP COLUMN): the database refuses a direct
// insert or re-point into a not-capable container, and keeps stored links.
func TestMigration9TriggersGuardLinkTargets(t *testing.T) {
	db := containerV8DB(t)
	migrateTo9(t, db)
	insert := `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (?, ?)`

	mustFail(t, db, "the container registration cannot hold items", insert, 1, 5) // gear 1: not capable now
	mustFail(t, db, "the container registration cannot hold items", insert, 4, 5) // dangling category
	mustFail(t, db, "the container registration cannot hold items", insert, 999, 5)
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_container_registration`); n != 2 {
		t.Fatalf("a refused insert changed links: %d", n)
	}

	if _, err := db.Exec(`UPDATE gear_category SET categoryIsContainer = 1 WHERE categoryId = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, 1, 5); err != nil { // registration 1 (gear 1) is capable now
		t.Fatalf("insert into a capable container refused: %v", err)
	}
	// Re-pointing a link into a container that is not capable is refused.
	mustFail(t, db, "the container registration cannot hold items",
		`UPDATE user_container_registration SET userContainerId = 3 WHERE userGearRegistrationId = 5`)
	// Losing capability afterwards leaves the link alone (P-15) and it stays deletable.
	if _, err := db.Exec(`UPDATE gear_category SET categoryIsContainer = 0 WHERE categoryId = 1`); err != nil {
		t.Fatal(err)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_container_registration WHERE userContainerId = 1`); n != 2 {
		t.Fatalf("links of the container after it lost capability = %d, want 2", n)
	}
	if _, err := db.Exec(`DELETE FROM user_container_registration WHERE userContainerId = 1 AND userGearRegistrationId = 5`); err != nil {
		t.Fatalf("delete refused: %v", err)
	}
}

// ADR-001 D-6: a replay fails on the first statement and changes nothing; down
// discards the settings and keeps links and legacy flags; up again restores the
// start values.
func TestMigration9ReplayAndDownUp(t *testing.T) {
	db := containerV8DB(t)
	path := migrationsPath(t)
	migrateTo9(t, db)

	if _, err := db.Exec(`UPDATE gear_top_category SET topCategoryIsContainer = 1 WHERE topCategoryId = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE gear_category SET categoryIsContainer = 0 WHERE categoryId = 2`); err != nil {
		t.Fatal(err)
	}
	beforeSchema := schemaSQL(t, db)

	script, err := os.ReadFile(filepath.Join(path, "000009_category_container.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(string(script)); err == nil || !strings.Contains(err.Error(), "duplicate column name") {
		t.Fatalf("replay error = %v, want duplicate column name", err)
	}
	tx.Rollback()
	if got := schemaSQL(t, db); got != beforeSchema {
		t.Fatal("a failed replay changed the schema")
	}
	if got := columnValues(t, db, `SELECT topCategoryIsContainer FROM gear_top_category ORDER BY topCategoryId`); !reflect.DeepEqual(got, []int{1, 0}) {
		t.Fatalf("a failed replay changed settings: %v", got)
	}

	stepMigrate(t, db, path, func(m *migrate.Migrate) error { return m.Steps(-1) })
	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 8 {
		t.Fatalf("version after down = %d, want 8", v)
	}
	for _, tc := range []struct{ typ, name string }{
		{"view", "gear_container_capability"}, {"view", "category_container_setting"},
		{"trigger", "trg_container_links_capable_insert"}, {"trigger", "trg_container_links_capable_update"},
	} {
		if schemaNames(t, db, tc.typ)[tc.name] {
			t.Errorf("%s %s left after down", tc.typ, tc.name)
		}
	}
	// Down restores the old column name with its values and keeps the links.
	if got := columnValues(t, db, `SELECT gearIsContainer FROM gear ORDER BY gearId`); !reflect.DeepEqual(got, []int{1, 0, 1, 1}) {
		t.Errorf("flags after down = %v", got)
	}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("links after down = %v", got)
	}
	if _, err := db.Query(`SELECT topCategoryIsContainer FROM gear_top_category`); err == nil {
		t.Error("the top category setting survived down")
	}

	migrateTo9(t, db)
	if got := columnValues(t, db, `SELECT topCategoryIsContainer FROM gear_top_category ORDER BY topCategoryId`); !reflect.DeepEqual(got, []int{0, 0}) {
		t.Errorf("settings after down/up = %v, want the start values", got)
	}
	if got := columnValues(t, db, `SELECT categoryIsContainer FROM gear_category ORDER BY categoryId`); !reflect.DeepEqual(got, []int{-1, -1}) {
		t.Errorf("sub-category settings after down/up = %v", got)
	}
	if got := columnValues(t, db, `SELECT gearIsContainerLegacy FROM gear ORDER BY gearId`); !reflect.DeepEqual(got, []int{1, 0, 1, 1}) {
		t.Errorf("legacy flags after down/up = %v", got)
	}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("links after down/up = %v", got)
	}
}
