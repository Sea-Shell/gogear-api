package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
	sqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

// migrationsPath resolves the migrations directory relative to the test file.
func migrationsPath(t *testing.T) string {
	t.Helper()
	// Walk up from the test file to find the migrations directory.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := filepath.Join(dir, "migrations")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("migrations directory not found at %s", path)
	}
	return path
}

func tempDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func runMigrate(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		t.Fatalf("driver init: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+path,
		"sqlite3",
		driver,
	)
	if err != nil {
		t.Fatalf("migrate instance: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}
}

func runMigrateDown(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		t.Fatalf("driver init: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+path,
		"sqlite3",
		driver,
	)
	if err != nil {
		t.Fatalf("migrate instance: %v", err)
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate down: %v", err)
	}
}

func TestMigrationsUp(t *testing.T) {
	db := tempDB(t)
	path := migrationsPath(t)

	runMigrate(t, db, path)

	// Verify schema_migrations table was created by golang-migrate
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&count)
	if err != nil {
		t.Fatalf("check schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatal("schema_migrations table not found after migration")
	}

	// Verify all expected tables exist
	expectedTables := []string{
		"users",
		"gear_top_category",
		"gear_category",
		"manufacture",
		"gear",
		"user_gear_registrations",
		"user_container_registration",
		"loadouts",
		"loadout_items",
	}
	for _, table := range expectedTables {
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("expected table %s to exist after migration up", table)
		}
	}

	// Verify migration version is recorded
	var version int
	var dirty bool
	err = db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != 6 {
		t.Errorf("expected version 6, got %d", version)
	}
	if dirty {
		t.Error("expected clean migration")
	}
}

func TestMigrationsUpDown(t *testing.T) {
	db := tempDB(t)
	path := migrationsPath(t)

	// Up
	runMigrate(t, db, path)

	// Verify tables exist
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='loadouts'").Scan(&count)
	if err != nil {
		t.Fatalf("check loadouts: %v", err)
	}
	if count != 1 {
		t.Fatal("loadouts should exist after up")
	}

	// Down rolls back ALL migrations (V005 to V001)
	runMigrateDown(t, db, path)

	// After full rollback, baseline tables (V001) are also dropped
	allTables := []string{
		"users",
		"gear_top_category",
		"gear_category",
		"manufacture",
		"gear",
		"user_gear_registrations",
		"user_container_registration",
		"loadouts",
		"loadout_items",
	}
	for _, table := range allTables {
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("check %s after down: %v", table, err)
		}
		if count != 0 {
			t.Errorf("table %s should be gone after full down", table)
		}
	}
}

func TestMigrationsIdempotentUp(t *testing.T) {
	db := tempDB(t)
	path := migrationsPath(t)

	// Run up twice
	runMigrate(t, db, path)
	runMigrate(t, db, path)

	// Should still be at version 6
	var version int
	err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != 6 {
		t.Errorf("expected version 6 after second up, got %d", version)
	}
}

// stepMigrate moves a database to an exact version.
func stepMigrate(t *testing.T, db *sql.DB, path string, fn func(m *migrate.Migrate) error) {
	t.Helper()
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		t.Fatalf("driver init: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+path, "sqlite3", driver)
	if err != nil {
		t.Fatalf("migrate instance: %v", err)
	}
	if err := fn(m); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func userColumns(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query("SELECT name FROM pragma_table_info('users')")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		cols[n] = true
	}
	return cols
}

// Migration 000004 applies to a database that already has users, leaves them
// unbound (so the backfill can find them) and can be rolled back.
func TestMigration4GoogleSubjectOnExistingDatabase(t *testing.T) {
	db := tempDB(t)
	db.SetMaxOpenConns(1)
	path := migrationsPath(t)

	stepMigrate(t, db, path, func(m *migrate.Migrate) error { return m.Migrate(3) })
	if _, err := db.Exec(`INSERT INTO users (userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal) VALUES ('a', '', 'A', 'a@example.com', 1, 1), ('b', '', 'B', 'b@example.com', 0, 1)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if userColumns(t, db)["googleSubject"] {
		t.Fatal("googleSubject exists before migration 4")
	}

	runMigrate(t, db, path)

	cols := userColumns(t, db)
	if !cols["googleSubject"] || !cols["userEmailEdited"] {
		t.Fatalf("columns after up: %v", cols)
	}
	var n, unbound, edited int
	if err := db.QueryRow(`SELECT COUNT(*), SUM(googleSubject IS NULL), SUM(userEmailEdited) FROM users`).Scan(&n, &unbound, &edited); err != nil {
		t.Fatal(err)
	}
	if n != 2 || unbound != 2 || edited != 0 {
		t.Fatalf("existing users: count=%d unbound=%d edited=%d", n, unbound, edited)
	}

	// The subject is unique among bound accounts; unbound ones may repeat NULL.
	if _, err := db.Exec(`UPDATE users SET googleSubject = 'S' WHERE userUsername = 'a'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET googleSubject = 'S' WHERE userUsername = 'b'`); err == nil {
		t.Fatal("two accounts bound to one subject")
	}

	stepMigrate(t, db, path, func(m *migrate.Migrate) error { return m.Migrate(3) })
	cols = userColumns(t, db)
	if cols["googleSubject"] || cols["userEmailEdited"] || !cols["userEmail"] {
		t.Fatalf("columns after down: %v", cols)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("users after down: %d, %v", n, err)
	}
}
