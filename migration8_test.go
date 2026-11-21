package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"
)

// A database at version 7 (built from version 4 data, so the old rules never
// applied) whose links break every container rule: several
// direct containers, rings of two and three with a tail, a link across owners,
// a self-link and orphans (inserted with foreign keys off, as legacy data was).
func containerLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	file := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", file+"?_foreign_keys=off")
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	stepMigrate(t, raw, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(4) })
	var regs []string
	for i := 1; i <= 20; i++ {
		owner := 1
		if i >= 15 {
			owner = 2
		}
		regs = append(regs, fmt.Sprintf("(%d, 1, %d)", i, owner))
	}
	// Link ids follow insert order, so the expectations below can name them.
	if _, err := raw.Exec(`
INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (1, 'a', '', 'a@example.com'), (2, 'b', '', 'b@example.com');
INSERT INTO gear_top_category (topCategoryId, topCategoryName) VALUES (1, 't');
INSERT INTO gear_category (categoryId, categoryTopCategoryId, categoryName) VALUES (1, 1, 'c');
INSERT INTO manufacture (manufactureId, manufactureName) VALUES (1, 'm');
INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight) VALUES (1, 1, 1, 1, 'tent', 100);
INSERT INTO user_gear_registrations (userGearRegistrationId, gearId, userId) VALUES ` + strings.Join(regs, ", ") + `;
INSERT INTO user_container_registration (containerRegistrationId, userContainerId, userGearRegistrationId) VALUES
	(1, 1, 2),    -- 2 in 1: kept (oldest container of 2)
	(2, 3, 2),    -- 2 in 3 too: removed, second container
	(3, 4, 5), (4, 5, 6), (5, 6, 4),    -- ring 4>5>6>4: the link into 4 (id 5) goes
	(6, 4, 7),    -- tail into the ring: kept
	(7, 8, 9), (8, 9, 10), (9, 10, 11), -- ordinary chain: kept
	(10, 12, 12), -- self-link: removed
	(11, 1, 15),  -- across owners: removed
	(12, 13, 999), (13, 998, 13), -- orphans: removed
	(14, 16, 17), (15, 17, 16);          -- ring 16<>17 (owner 2): the link into 16 (id 15) goes
`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	raw.Close()
	db := openEnforced(t, file)
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(7) })
	return db
}

func linkIDs(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT containerRegistrationId FROM user_container_registration ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// BR-020 AC-8: after the migration no stored link breaks a rule, and only the
// links that had to go are gone.
func TestMigration8RemovesViolatingLinks(t *testing.T) {
	db := containerLegacyDB(t)
	runMigrate(t, db, migrationsPath(t))

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 8 {
		t.Fatalf("version = %d", v)
	}
	want := []int{1, 3, 4, 6, 7, 8, 9, 14}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("links left = %v, want %v", got, want)
	}
	// No self-link, no second container, no link across owners, no ring.
	for q, what := range map[string]string{
		`SELECT COUNT(*) FROM user_container_registration WHERE userContainerId = userGearRegistrationId`:                      "self-links",
		`SELECT COUNT(*) FROM (SELECT 1 FROM user_container_registration GROUP BY userGearRegistrationId HAVING COUNT(*) > 1)`: "second containers",
		`SELECT COUNT(*) FROM user_container_registration l
			JOIN user_gear_registrations c ON c.userGearRegistrationId = l.userContainerId
			JOIN user_gear_registrations x ON x.userGearRegistrationId = l.userGearRegistrationId WHERE c.userId <> x.userId`: "cross-owner links",
		`WITH RECURSIVE reach(node, anc) AS (
			SELECT userGearRegistrationId, userContainerId FROM user_container_registration
			UNION SELECT r.node, l.userContainerId FROM reach r JOIN user_container_registration l ON l.userGearRegistrationId = r.anc)
			SELECT COUNT(*) FROM reach WHERE node = anc`: "rings",
	} {
		if n := scalar[int](t, db, q); n != 0 {
			t.Errorf("%d %s remain", n, what)
		}
	}
	// Registrations themselves are untouched.
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_gear_registrations`); n != 20 {
		t.Fatalf("registrations = %d", n)
	}

	// The 000005 content-pair index is superseded by the content-unique index.
	idx := schemaNames(t, db, "index")
	for name, want := range map[string]bool{
		"idx_container_links_content_unique": true, "idx_container_links_container": true,
		"idx_container_links_unique": false, "idx_container_links_content": false,
	} {
		if idx[name] != want {
			t.Errorf("index %s present = %v, want %v", name, idx[name], want)
		}
	}
	for q, wantIdx := range map[string]string{
		`SELECT 1 FROM user_container_registration WHERE userContainerId = 4`:        "idx_container_links_container",
		`SELECT 1 FROM user_container_registration WHERE userGearRegistrationId = 2`: "idx_container_links_content_unique",
	} {
		if plan := queryPlan(t, db, q); !strings.Contains(plan, wantIdx) {
			t.Errorf("%s: plan %q does not use %s", q, plan, wantIdx)
		}
	}

	// The rules are enforced from now on, by the database alone.
	mustFail(t, db, "UNIQUE constraint failed", `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (3, 2)`)
	mustFail(t, db, "inside one of its own contents", `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (7, 4)`)
	if _, err := db.Exec(`INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (3, 3)`); err == nil {
		t.Fatal("self-link accepted")
	}
	mustFail(t, db, "UNIQUE constraint failed", `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (11, 5)`) // 5 is already in 4
	if _, err := db.Exec(`INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (3, 12)`); err != nil {
		t.Fatalf("valid link refused: %v", err)
	}
}

// Replaying the script, and down then up, change nothing that is already valid.
func TestMigration8ReplayAndDownUp(t *testing.T) {
	db := containerLegacyDB(t)
	path := migrationsPath(t)
	runMigrate(t, db, path)
	before := linkIDs(t, db)

	script, err := os.ReadFile(filepath.Join(path, "000008_container_nesting.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(script)); err != nil {
			t.Fatalf("replay %d: %v", i, err)
		}
	}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("replay changed links: %v -> %v", before, got)
	}

	stepMigrate(t, db, path, func(m *migrate.Migrate) error { return m.Steps(-1) })
	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 7 {
		t.Fatalf("version after down = %d", v)
	}
	idx := schemaNames(t, db, "index")
	for name, want := range map[string]bool{
		"idx_container_links_unique": true, "idx_container_links_content": true,
		"idx_container_links_content_unique": false, "idx_container_links_container": false,
	} {
		if idx[name] != want {
			t.Errorf("after down: index %s present = %v, want %v", name, idx[name], want)
		}
	}
	if schemaNames(t, db, "trigger")["trg_container_links_no_cycle_insert"] {
		t.Error("cycle trigger left after down")
	}
	if got := linkIDs(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("down changed links: %v", got)
	}
	runMigrate(t, db, path)
	if got := linkIDs(t, db); !reflect.DeepEqual(got, before) {
		t.Fatalf("up again changed links: %v", got)
	}
}

// A clean database migrates without touching anything and without error.
func TestMigration8OnCleanDatabase(t *testing.T) {
	db := openEnforced(t, filepath.Join(t.TempDir(), "clean.db"))
	runMigrate(t, db, migrationsPath(t))
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_container_registration`); n != 0 {
		t.Fatalf("links on a clean database: %d", n)
	}
}
