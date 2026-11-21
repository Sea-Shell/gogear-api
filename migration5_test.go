package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	migrate "github.com/golang-migrate/migrate/v4"

	"github.com/Sea-Shell/gogear-api/pkg/utils"
)

// legacyDB builds a database at schema version 4 with data written while
// foreign keys were off: orphans, a blank slug, zero and negative quantities,
// duplicate and self container links. It returns the file path.
func legacyDB(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", file+"?_foreign_keys=off")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(4) })
	if _, err := db.Exec(`
INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (1, 'a', '', 'a@example.com');
INSERT INTO gear_top_category (topCategoryId, topCategoryName) VALUES (1, 't');
INSERT INTO gear_category (categoryId, categoryTopCategoryId, categoryName) VALUES (1, 1, 'c');
INSERT INTO manufacture (manufactureId, manufactureName) VALUES (1, 'm');
INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight) VALUES (1, 1, 1, 1, 'tent', 100);
INSERT INTO user_gear_registrations (userGearRegistrationId, gearId, userId) VALUES (1, 1, 1), (2, 1, 1), (3, 1, 99), (4, 77, 1);
INSERT INTO user_container_registration (containerRegistrationId, userContainerId, userGearRegistrationId) VALUES
	(1, 1, 2), (2, 1, 2), (3, 1, 2),   -- three identical links
	(4, 2, 1),                          -- the reverse link is a different link
	(5, 1, 1),                          -- self-link
	(6, 1, 555);                        -- orphan content
INSERT INTO loadouts (loadoutId, userId, loadoutName, loadoutSlug, totalWeight) VALUES
	(1, 1, 'blank slug', '', 12345), (2, 1, 'real slug', 'x', 200), (3, 99, 'orphan owner', NULL, 0);
INSERT INTO loadout_items (loadoutItemId, loadoutId, gearId, quantity) VALUES
	(1, 1, 1, 0), (2, 1, 1, -3), (3, 2, 1, 2), (4, 1, 88, 1), (5, 1, 88, 0);`); err != nil {
		t.Fatalf("seed legacy data: %v", err)
	}
	return file
}

func openEnforced(t *testing.T, file string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", utils.SQLiteDSN(file))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func scalar[T any](t *testing.T, db *sql.DB, q string, args ...any) T {
	t.Helper()
	var v T
	if err := db.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v
}

func schemaNames(t *testing.T, db *sql.DB, typ string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = ?`, typ)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[n] = true
	}
	return out
}

func mustFail(t *testing.T, db *sql.DB, wantMsg, q string, args ...any) {
	t.Helper()
	_, err := db.Exec(q, args...)
	if err == nil {
		t.Fatalf("expected failure, statement succeeded: %s", q)
	}
	if !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("error = %v, want it to mention %q (%s)", err, wantMsg, q)
	}
}

// Migration 000005 on a legacy database under foreign-key enforcement: it
// runs, repairs only what is safe, keeps orphans, and enforces the rules.
func TestMigration5IntegrityOnLegacyData(t *testing.T) {
	file := legacyDB(t)
	db := openEnforced(t, file)
	ctx := context.Background()
	before, err := utils.ForeignKeyViolations(ctx, db)
	if err != nil || len(before) == 0 {
		t.Fatalf("legacy data should contain orphans: %+v, %v", before, err)
	}

	runMigrate(t, db, migrationsPath(t)) // pooled connections enforce foreign keys

	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 6 {
		t.Fatalf("version = %d, want 6", v)
	}
	if scalar[int](t, db, `PRAGMA foreign_keys`) != 1 {
		t.Fatal("foreign keys not enforced during the test")
	}

	// Quantities: clamped to 1, originals audited, affected totals recomputed.
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM loadout_items WHERE quantity <= 0`); n != 0 {
		t.Fatalf("%d items still have quantity <= 0", n)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM loadout_items`); n != 5 {
		t.Fatalf("items deleted: %d left, want 6", n)
	}
	rows, err := db.Query(`SELECT loadoutItemId, quantity FROM audit_000005_clamped_loadout_item_quantity ORDER BY auditId`)
	if err != nil {
		t.Fatal(err)
	}
	var audited [][2]int
	for rows.Next() {
		var id, q int
		if err := rows.Scan(&id, &q); err != nil {
			t.Fatal(err)
		}
		audited = append(audited, [2]int{id, q})
	}
	rows.Close()
	if want := [][2]int{{1, 0}, {2, -3}, {5, 0}}; !reflect.DeepEqual(audited, want) {
		t.Fatalf("audit = %v, want %v", audited, want)
	}
	if w := scalar[int](t, db, `SELECT totalWeight FROM loadouts WHERE loadoutId = 1`); w != 200 {
		t.Fatalf("clamped loadout total = %d, want 200 (two items of 100 g)", w)
	}
	if w := scalar[int](t, db, `SELECT totalWeight FROM loadouts WHERE loadoutId = 2`); w != 200 {
		t.Fatalf("untouched loadout total changed: %d", w)
	}

	// Slugs.
	if !scalar[bool](t, db, `SELECT loadoutSlug IS NULL FROM loadouts WHERE loadoutId = 1`) {
		t.Fatal("blank slug not normalized to NULL")
	}
	if s := scalar[string](t, db, `SELECT loadoutSlug FROM loadouts WHERE loadoutId = 2`); s != "x" {
		t.Fatalf("real slug changed to %q", s)
	}

	// Container links: identical duplicates reduced, everything else kept.
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_container_registration WHERE userContainerId = 1 AND userGearRegistrationId = 2`); n != 1 {
		t.Fatalf("duplicate links left: %d", n)
	}
	if id := scalar[int](t, db, `SELECT containerRegistrationId FROM user_container_registration WHERE userContainerId = 1 AND userGearRegistrationId = 2`); id != 1 {
		t.Fatalf("kept link %d, want the oldest (1)", id)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_container_registration`); n != 4 {
		t.Fatalf("links = %d, want 4 (dup 1, reverse, self, orphan)", n)
	}

	// Orphans were neither deleted nor repaired.
	after, err := utils.ForeignKeyViolations(ctx, db)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("orphans changed: before %+v after %+v (%v)", before, after, err)
	}

	// Rules are enforced from now on.
	mustFail(t, db, "quantity must be", `INSERT INTO loadout_items (loadoutId, gearId, quantity) VALUES (2, 1, 0)`)
	mustFail(t, db, "quantity must be", `INSERT INTO loadout_items (loadoutId, gearId, quantity) VALUES (2, 1, -1)`)
	mustFail(t, db, "quantity must be", `INSERT INTO loadout_items (loadoutId, gearId, quantity) VALUES (2, 1, 'many')`)
	mustFail(t, db, "quantity must be", `UPDATE loadout_items SET quantity = 0 WHERE loadoutItemId = 3`)
	mustFail(t, db, "UNIQUE constraint failed", `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (1, 2)`)
	mustFail(t, db, "contained in itself", `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (2, 2)`)
	mustFail(t, db, "contained in itself", `UPDATE user_container_registration SET userGearRegistrationId = 2 WHERE containerRegistrationId = 4`)
	mustFail(t, db, "FOREIGN KEY constraint failed", `INSERT INTO user_gear_registrations (gearId, userId) VALUES (4242, 1)`)
	if _, err := db.Exec(`INSERT INTO loadout_items (loadoutId, gearId, quantity) VALUES (2, 1, 4)`); err != nil {
		t.Fatalf("valid item refused: %v", err)
	}
	if _, err := db.Exec(`UPDATE loadout_items SET quantity = 7 WHERE loadoutItemId = 3`); err != nil {
		t.Fatalf("valid quantity update refused: %v", err)
	}

	// Indexes exist and the owner lookup uses one.
	idx := schemaNames(t, db, "index")
	for _, n := range []string{"idx_container_links_unique", "idx_container_links_content", "idx_user_gear_registrations_user", "idx_user_gear_registrations_gear"} {
		if !idx[n] {
			t.Errorf("index %s missing", n)
		}
	}
	for q, want := range map[string]string{
		`SELECT 1 FROM user_gear_registrations WHERE userId = 1`:                     "idx_user_gear_registrations_user",
		`SELECT 1 FROM user_gear_registrations WHERE gearId = 1`:                     "idx_user_gear_registrations_gear",
		`SELECT 1 FROM user_container_registration WHERE userContainerId = 1`:        "idx_container_links_unique",
		`SELECT 1 FROM user_container_registration WHERE userGearRegistrationId = 2`: "idx_container_links_content",
	} {
		if plan := queryPlan(t, db, q); !strings.Contains(plan, want) {
			t.Errorf("%s\n  plan %q does not use %s", q, plan, want)
		}
	}
}

func queryPlan(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	rows, err := db.Query(`EXPLAIN QUERY PLAN ` + q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, detail)
	}
	return strings.Join(parts, " | ")
}

// Down removes the constraints and keeps the data; up again restores them,
// and replaying the migration script on a migrated database changes nothing.
func TestMigration5DownUpAndReplay(t *testing.T) {
	db := openEnforced(t, legacyDB(t))
	path := migrationsPath(t)
	runMigrate(t, db, path)

	// Steps down through 000006 (trim emails) and 000005.
	stepMigrate(t, db, path, func(m *migrate.Migrate) error { return m.Steps(-2) })
	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 4 {
		t.Fatalf("version after down = %d, want 4", v)
	}
	if n := len(schemaNames(t, db, "trigger")); n != 0 {
		t.Fatalf("%d triggers left after down", n)
	}
	idx := schemaNames(t, db, "index")
	for _, n := range []string{"idx_container_links_unique", "idx_container_links_content", "idx_user_gear_registrations_user", "idx_user_gear_registrations_gear"} {
		if idx[n] {
			t.Errorf("index %s left after down", n)
		}
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM loadout_items`); n != 5 {
		t.Fatalf("down changed item rows: %d", n)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM audit_000005_clamped_loadout_item_quantity`); n != 3 {
		t.Fatalf("audit table must survive down: %d rows", n)
	}
	// Rules are gone again: a zero quantity is accepted.
	if _, err := db.Exec(`INSERT INTO loadout_items (loadoutId, gearId, quantity) VALUES (2, 1, 0)`); err != nil {
		t.Fatalf("zero quantity should be accepted after down: %v", err)
	}

	// Item 1 was clamped by the first run (original 0); break it again.
	if _, err := db.Exec(`UPDATE loadout_items SET quantity = -7 WHERE loadoutItemId = 1`); err != nil {
		t.Fatal(err)
	}

	runMigrate(t, db, path) // up again over data that violates the rule once more
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM loadout_items WHERE quantity <= 0`); n != 0 {
		t.Fatalf("re-up left %d bad quantities", n)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM audit_000005_clamped_loadout_item_quantity`); n != 5 {
		t.Fatalf("audit rows after re-up = %d, want 5 (one per run and item)", n)
	}
	if got := scalar[string](t, db, `SELECT group_concat(quantity) FROM (SELECT quantity FROM audit_000005_clamped_loadout_item_quantity WHERE loadoutItemId = 1 ORDER BY auditId)`); got != "0,-7" {
		t.Fatalf("audit values for item 1 = %q, want both runs: 0,-7", got)
	}
	if n := len(schemaNames(t, db, "trigger")); n != 4 {
		t.Fatalf("triggers after re-up = %d, want 4", n)
	}

	// Replay the script itself, as a half-applied or repeated run would.
	script, err := os.ReadFile(filepath.Join(path, "000005_integrity_constraints.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var b strings.Builder
		for _, q := range []string{
			`SELECT group_concat(loadoutItemId || ':' || quantity) FROM loadout_items`,
			`SELECT group_concat(containerRegistrationId) FROM user_container_registration`,
			`SELECT group_concat(loadoutId || ':' || IFNULL(loadoutSlug, '-') || ':' || totalWeight) FROM loadouts`,
			`SELECT group_concat(loadoutItemId || ':' || quantity) FROM audit_000005_clamped_loadout_item_quantity`,
		} {
			b.WriteString(scalar[string](t, db, q) + ";")
		}
		return b.String()
	}
	before := snapshot()
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(script)); err != nil {
			t.Fatalf("replay %d: %v", i, err)
		}
	}
	if after := snapshot(); after != before {
		t.Fatalf("replay changed data:\n before %s\n after  %s", before, after)
	}
}

// A database that is already consistent migrates without touching anything.
func TestMigration5OnCleanDatabase(t *testing.T) {
	db := openEnforced(t, filepath.Join(t.TempDir(), "clean.db"))
	runMigrate(t, db, migrationsPath(t))
	if v := scalar[int](t, db, `SELECT version FROM schema_migrations`); v != 6 {
		t.Fatalf("version = %d", v)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM audit_000005_clamped_loadout_item_quantity`); n != 0 {
		t.Fatalf("audit rows on a clean database: %d", n)
	}
	viol, err := utils.ForeignKeyViolations(context.Background(), db)
	if err != nil || len(viol) != 0 {
		t.Fatalf("violations %+v, %v", viol, err)
	}
}

// Migration 000006 trims every kind of surrounding whitespace from stored
// emails, as strings.TrimSpace does, and leaves case and inner text alone.
func TestMigration6TrimsStoredEmails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "v5.db")
	db := openEnforced(t, file)
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(5) })
	emails := []string{"a@example.com", "b@example.com\t", "\n c@Example.com \r\n", " d@example.com　", "e @example.com"}
	for i, e := range emails {
		if _, err := db.Exec(`INSERT INTO users (userId, userUsername, userPassword, userEmail) VALUES (?, ?, '', ?)`, i+1, "u"+string(rune('a'+i)), e); err != nil {
			t.Fatal(err)
		}
	}
	runMigrate(t, db, migrationsPath(t))
	want := []string{"a@example.com", "b@example.com", "c@Example.com", "d@example.com", "e @example.com"}
	for i, w := range want {
		if got := scalar[string](t, db, `SELECT userEmail FROM users WHERE userId = ?`, i+1); got != w {
			t.Errorf("user %d email = %q, want %q", i+1, got, w)
		}
	}
}
