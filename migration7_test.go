package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
)

func permanentAdminID(t *testing.T, db *sql.DB) (int64, bool) {
	t.Helper()
	var id int64
	err := db.QueryRow(`SELECT userId FROM permanent_admin`).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return id, true
}

// BR-019 AC-1 (P-8): when the migration runs, the admin with the lowest user ID
// is designated, whichever order the admin flags were set in.
func TestMigration7DesignatesLowestIDAdmin(t *testing.T) {
	db := openEnforced(t, filepath.Join(t.TempDir(), "v6.db"))
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(6) })
	if _, err := db.Exec(`
INSERT INTO users (userId, userUsername, userPassword, userEmail, userIsAdmin) VALUES
	(1, 'plain', '', 'p@example.com', 0), (3, 'third', '', 't@example.com', 1), (5, 'fifth', '', 'f@example.com', 1);`); err != nil {
		t.Fatal(err)
	}
	runMigrate(t, db, migrationsPath(t))
	if id, ok := permanentAdminID(t, db); !ok || id != 3 {
		t.Fatalf("permanent admin = %d (%v), want 3", id, ok)
	}
	// Promoting a lower ID afterwards does not move it.
	if _, err := db.Exec(`UPDATE users SET userIsAdmin = 1 WHERE userId = 1`); err != nil {
		t.Fatal(err)
	}
	if id, _ := permanentAdminID(t, db); id != 3 {
		t.Fatalf("designation moved to %d", id)
	}
}

// With no admin when the migration runs, the first account that becomes admin
// afterwards is designated, by insert or by update.
func TestMigration7NoAdminYetDesignatesFirstToBecomeAdmin(t *testing.T) {
	for name, becomeAdmin := range map[string]func(*sql.DB) error{
		"update": func(db *sql.DB) error {
			_, err := db.Exec(`UPDATE users SET userIsAdmin = 1 WHERE userId = 2`)
			return err
		},
		"insert": func(db *sql.DB) error {
			_, err := db.Exec(`INSERT INTO users (userId, userUsername, userPassword, userEmail, userIsAdmin) VALUES (9, 'new', '', 'n@example.com', 1)`)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := openEnforced(t, filepath.Join(t.TempDir(), "v6.db"))
			stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(6) })
			if _, err := db.Exec(`INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (1, 'a', '', 'a@example.com'), (2, 'b', '', 'b@example.com')`); err != nil {
				t.Fatal(err)
			}
			runMigrate(t, db, migrationsPath(t))
			if _, ok := permanentAdminID(t, db); ok {
				t.Fatal("designated with no admin")
			}
			if err := becomeAdmin(db); err != nil {
				t.Fatal(err)
			}
			want := int64(2)
			if name == "insert" {
				want = 9
			}
			if id, ok := permanentAdminID(t, db); !ok || id != want {
				t.Fatalf("permanent admin = %d (%v), want %d", id, ok, want)
			}
			// A later admin never replaces it.
			if _, err := db.Exec(`UPDATE users SET userIsAdmin = 1 WHERE userId = 1`); err != nil {
				t.Fatal(err)
			}
			if id, _ := permanentAdminID(t, db); id != want {
				t.Fatalf("designation moved to %d", id)
			}
		})
	}
}

func TestMigration7DownRemovesDesignationOnly(t *testing.T) {
	db := openEnforced(t, filepath.Join(t.TempDir(), "x.db"))
	runMigrate(t, db, migrationsPath(t))
	if _, err := db.Exec(`INSERT INTO users (userId, userUsername, userPassword, userEmail, userIsAdmin) VALUES (1, 'a', '', 'a@example.com', 1)`); err != nil {
		t.Fatal(err)
	}
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Steps(-1) })
	if schemaNames(t, db, "table")["permanent_admin"] {
		t.Fatal("permanent_admin left after down")
	}
	if !scalar[bool](t, db, `SELECT userIsAdmin FROM users WHERE userId = 1`) {
		t.Fatal("down changed admin status")
	}
	// Up again designates the existing admin.
	runMigrate(t, db, migrationsPath(t))
	if id, ok := permanentAdminID(t, db); !ok || id != 1 {
		t.Fatalf("after re-up: %d %v", id, ok)
	}
}
