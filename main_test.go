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
	if version != 3 {
		t.Errorf("expected version 3, got %d", version)
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

	// Down rolls back ALL migrations (V003, V002, V001)
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

	// Should still be at version 3
	var version int
	err := db.QueryRow("SELECT version FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != 3 {
		t.Errorf("expected version 3 after second up, got %d", version)
	}
}
