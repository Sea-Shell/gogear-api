package main

import (
	"database/sql"
	"io"
	"os"
	"reflect"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
	"go.uber.org/zap"
)

// Migration 000010 (ADR-002 D-7, BR-021 AC-5 amended, P-18): the category
// settings are seeded from the old per-gear flags, once. The tests use the
// version 8 fixture of migration9_test.go:
//
//	category 1 (top 1): gear 1 flagged, gear 2 not flagged   -> any flagged: container
//	category 2 (top 2): gear 3 flagged                       -> container
//	category 999 (missing): gear 4 flagged                   -> no row, nothing to seed
//
// plus two rows added here:
//
//	category 3 (top 1): gear 5 not flagged                   -> no explicit setting
//	category 4 (top 2): no gear                              -> no explicit setting

func containerV8DBWithPlainCategories(t *testing.T) *sql.DB {
	t.Helper()
	db := containerV8DB(t)
	if _, err := db.Exec(`
INSERT INTO gear_category (categoryId, categoryTopCategoryId, categoryName) VALUES (3, 1, 'c3'), (4, 2, 'c4');
INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearIsContainer, gearName, gearWeight)
	VALUES (5, 1, 3, 1, 0, 'plain3', 100);
`); err != nil {
		t.Fatalf("seed extra categories: %v", err)
	}
	return db
}

func migrateTo(t *testing.T, db *sql.DB, version uint) {
	t.Helper()
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(version) })
}

func categorySettings(t *testing.T, db *sql.DB) []int {
	t.Helper()
	return columnValues(t, db, `SELECT categoryIsContainer FROM gear_category ORDER BY categoryId`)
}

func topSettings(t *testing.T, db *sql.DB) []int {
	t.Helper()
	return columnValues(t, db, `SELECT topCategoryIsContainer FROM gear_top_category ORDER BY topCategoryId`)
}

func capability(t *testing.T, db *sql.DB) []int {
	t.Helper()
	return columnValues(t, db, `SELECT isContainer FROM gear_container_capability ORDER BY gearId`)
}

func execAll(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatal(err)
	}
}

// BR-021 AC-5: from a version 8 database with all, some and no flags, exactly
// the sub-categories with at least one flagged gear item become explicit
// containers; the rest have no explicit setting; top categories stay 0; the
// reported status of every gear item follows.
func TestMigration10SeedsFromV8MixedFlags(t *testing.T) {
	db := containerV8DBWithPlainCategories(t)
	migrateTo(t, db, 10)

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 10 {
		t.Fatalf("version = %d, want 10", v)
	}
	if got := categorySettings(t, db); !reflect.DeepEqual(got, []int{1, 1, -1, -1}) {
		t.Errorf("sub-category settings = %v, want [1 1 NULL NULL]", got)
	}
	if got := topSettings(t, db); !reflect.DeepEqual(got, []int{0, 0}) {
		t.Errorf("top category settings = %v, want all 0", got)
	}
	// Gear 1, 2 (category 1) and 3 (category 2) are capable; the dangling gear 4
	// and the gear of the unflagged category 3 are not.
	if got := capability(t, db); !reflect.DeepEqual(got, []int{1, 1, 1, 0, 0}) {
		t.Errorf("derived status = %v, want [1 1 1 0 0]", got)
	}
	// Seeding reads the old flags and does not change them, nor links.
	if got := columnValues(t, db, `SELECT gearIsContainerLegacy FROM gear ORDER BY gearId`); !reflect.DeepEqual(got, []int{1, 0, 1, 1, 0}) {
		t.Errorf("legacy flags = %v", got)
	}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("links = %v, want [1 2]", got)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM gear_category`); n != 4 {
		t.Errorf("a category row was created: %d rows", n)
	}
}

// The same result when version 9 is reached first (a database that already
// applied 000009), including a top category an admin made a container: its
// setting is kept and its inheriting sub-category without flags inherits it.
func TestMigration10SeedsFromV9AndLeavesTopCategoriesAlone(t *testing.T) {
	db := containerV8DBWithPlainCategories(t)
	migrateTo(t, db, 9)
	execAll(t, db, `UPDATE gear_top_category SET topCategoryIsContainer = 1 WHERE topCategoryId = 2`)
	migrateTo(t, db, 10)

	if got := topSettings(t, db); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Errorf("top category settings = %v, want [0 1] (unchanged)", got)
	}
	if got := categorySettings(t, db); !reflect.DeepEqual(got, []int{1, 1, -1, -1}) {
		t.Errorf("sub-category settings = %v, want [1 1 NULL NULL]", got)
	}
	// Category 4 (top 2, no setting) inherits its top category; category 3 does not.
	if got := columnValues(t, db, `SELECT effectiveIsContainer FROM category_container_setting ORDER BY categoryId`); !reflect.DeepEqual(got, []int{1, 1, 0, 1}) {
		t.Errorf("effective settings = %v, want [1 1 0 1]", got)
	}
}

// P-18: an explicit setting made by an admin is never overwritten, whether it
// is 0 on a sub-category with flagged gear, or 1 on one without.
func TestMigration10NeverOverwritesAdminSettings(t *testing.T) {
	db := containerV8DBWithPlainCategories(t)
	migrateTo(t, db, 9)
	execAll(t, db, `
UPDATE gear_category SET categoryIsContainer = 0 WHERE categoryId = 1;
UPDATE gear_category SET categoryIsContainer = 1 WHERE categoryId = 3;`)
	migrateTo(t, db, 10)

	// Category 1: flagged gear but explicit 0 stays 0. Category 2: no setting,
	// flagged gear -> 1. Category 3: explicit 1 without flags stays 1.
	if got := categorySettings(t, db); !reflect.DeepEqual(got, []int{0, 1, 1, -1}) {
		t.Errorf("sub-category settings = %v, want [0 1 1 NULL]", got)
	}
	if got := capability(t, db); !reflect.DeepEqual(got, []int{0, 0, 1, 0, 1}) {
		t.Errorf("derived status = %v, want [0 0 1 0 1]", got)
	}
}

// ADR-002 D-7: the seeding runs once. golang-migrate records version 10; a
// later Up does not run the statement again, so an admin who resets a seeded
// sub-category to "inherit" is not overruled.
func TestMigration10SeedsOnceThroughGolangMigrate(t *testing.T) {
	db := containerV8DBWithPlainCategories(t)
	migrateTo(t, db, 10)
	execAll(t, db, `UPDATE gear_category SET categoryIsContainer = NULL WHERE categoryId = 2`)

	runMigrate(t, db, migrationsPath(t)) // Up: already at the latest version
	runMigrate(t, db, migrationsPath(t))

	if got := categorySettings(t, db); !reflect.DeepEqual(got, []int{1, -1, -1, -1}) {
		t.Errorf("sub-category settings after a second Up = %v, want [1 NULL NULL NULL]", got)
	}
	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 10 {
		t.Errorf("version = %d, want 10", v)
	}
}

// ADR-002 D-7: down changes no data, because a seeded 1 cannot be told apart
// from an admin's 1.
func TestMigration10DownChangesNoData(t *testing.T) {
	db := containerV8DBWithPlainCategories(t)
	migrateTo(t, db, 9)
	execAll(t, db, `
UPDATE gear_category SET categoryIsContainer = 0 WHERE categoryId = 3;
UPDATE gear_top_category SET topCategoryIsContainer = 1 WHERE topCategoryId = 2;`)
	migrateTo(t, db, 10)

	cats, tops, legacy := categorySettings(t, db), topSettings(t, db),
		columnValues(t, db, `SELECT gearIsContainerLegacy FROM gear ORDER BY gearId`)
	links, schema := linkIDs(t, db), schemaSQL(t, db)

	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Steps(-1) })

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 9 {
		t.Fatalf("version after down = %d, want 9", v)
	}
	if got := categorySettings(t, db); !reflect.DeepEqual(got, cats) {
		t.Errorf("sub-category settings changed by down: %v -> %v", cats, got)
	}
	if got := topSettings(t, db); !reflect.DeepEqual(got, tops) {
		t.Errorf("top category settings changed by down: %v -> %v", tops, got)
	}
	if got := columnValues(t, db, `SELECT gearIsContainerLegacy FROM gear ORDER BY gearId`); !reflect.DeepEqual(got, legacy) {
		t.Errorf("legacy flags changed by down: %v -> %v", legacy, got)
	}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, links) {
		t.Errorf("links changed by down: %v -> %v", links, got)
	}
	if schemaSQL(t, db) != schema {
		t.Error("down changed the schema")
	}
}

// A fresh database has no gear: the statement changes nothing and the schema
// reaches version 10 (BR-022: no admin step).
func TestMigration10OnFreshDatabaseSeedsNothing(t *testing.T) {
	db := tempDB(t)
	runMigrate(t, db, migrationsPath(t))
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM gear_category WHERE categoryIsContainer IS NOT NULL`); n != 0 {
		t.Errorf("%d categories seeded on a fresh database", n)
	}
}

// ADR-002 D-8: runMigrations writes one stdout line the deploy check parses,
// whatever the logger's level, both when it migrates and when nothing is left.
func TestRunMigrationsPrintsVersionLineToStdout(t *testing.T) {
	db := tempDB(t)
	// A logger that drops every level, like general.log_level=error would.
	silent := zap.NewNop().Sugar()

	first := captureStdout(t, func() {
		if err := runMigrations(db, migrationsPath(t), silent); err != nil {
			t.Fatalf("runMigrations: %v", err)
		}
	})
	want := "gogear-api migrations: version=10 dirty=false\n"
	if first != want {
		t.Errorf("stdout after migrating = %q, want %q", first, want)
	}

	second := captureStdout(t, func() {
		if err := runMigrations(db, migrationsPath(t), silent); err != nil {
			t.Fatalf("runMigrations again: %v", err)
		}
	})
	if second != want {
		t.Errorf("stdout with nothing to migrate = %q, want %q", second, want)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	func() {
		defer func() {
			os.Stdout = orig
			w.Close()
		}()
		fn()
	}()
	out := <-done
	r.Close()
	return out
}
