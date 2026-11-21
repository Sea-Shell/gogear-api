package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// The startup audit warns once per (table, parent) with the orphan count,
// changes no data, and does not stop startup.
func TestReportForeignKeyViolations_WarnsPerTableAndLeavesData(t *testing.T) {
	db := openEnforced(t, legacyDB(t))
	runMigrate(t, db, migrationsPath(t))

	core, logs := observer.New(zapcore.DebugLevel)
	total, err := reportForeignKeyViolations(context.Background(), db, zap.New(core).Sugar())
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]int64{}
	for _, e := range logs.All() {
		if e.Level != zapcore.WarnLevel {
			t.Errorf("unexpected %v log: %s", e.Level, e.Message)
		}
		f := e.ContextMap()
		got[f["table"].(string)+"->"+f["parent_table"].(string)] = f["orphan_rows"].(int64)
	}
	want := map[string]int64{
		"loadout_items->gear": 2, // items 4, 5
		"loadouts->users":     1,
		"user_container_registration->user_gear_registrations": 1,
		"user_gear_registrations->gear":                        1,
		"user_gear_registrations->users":                       1,
	}
	if len(got) != len(want) {
		t.Fatalf("warnings = %v, want %v", got, want)
	}
	var sum int64
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: %d orphans, want %d (all: %v)", k, got[k], n, got)
		}
		sum += n
	}
	if int64(total) != sum {
		t.Errorf("total = %d, want %d", total, sum)
	}
	if n := scalar[int](t, db, `SELECT COUNT(*) FROM user_gear_registrations`); n != 4 {
		t.Errorf("audit deleted rows: %d registrations left", n)
	}
}

func TestReportForeignKeyViolations_SilentWhenConsistent(t *testing.T) {
	db := openEnforced(t, t.TempDir()+"/ok.db")
	runMigrate(t, db, migrationsPath(t))
	core, logs := observer.New(zapcore.DebugLevel)
	total, err := reportForeignKeyViolations(context.Background(), db, zap.New(core).Sugar())
	if err != nil || total != 0 || logs.Len() != 0 {
		t.Fatalf("total %d, err %v, %d log entries", total, err, logs.Len())
	}
}

func openWithDSN(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// A database file name that switches enforcement off through the DSN options
// is warned about at startup; the default configuration is not.
func TestWarnIfForeignKeysOff(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		file     string
		wantOn   bool
		wantWarn int
	}{
		{"default", filepath.Join(dir, "a.db"), true, 0},
		{"_fk=0", filepath.Join(dir, "b.db") + "?_fk=0", false, 1},
		{"_foreign_keys=off", filepath.Join(dir, "c.db") + "?_foreign_keys=off", false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openWithDSN(t, utils.SQLiteDSN(tc.file))
			core, logs := observer.New(zapcore.DebugLevel)
			if got := warnIfForeignKeysOff(context.Background(), db, zap.New(core).Sugar()); got != tc.wantOn {
				t.Errorf("enforced = %v, want %v", got, tc.wantOn)
			}
			if n := logs.FilterLevelExact(zapcore.WarnLevel).Len(); n != tc.wantWarn {
				t.Errorf("warnings = %d, want %d: %v", n, tc.wantWarn, logs.All())
			}
		})
	}
}
