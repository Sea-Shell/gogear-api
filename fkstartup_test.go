package main

import (
	"context"
	"testing"

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
