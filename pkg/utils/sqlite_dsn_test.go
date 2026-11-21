package utils

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteDSN(t *testing.T) {
	const opts = "_txlock=immediate&_busy_timeout=5000&_foreign_keys=on"
	cases := map[string]string{
		"/data/gogear.db":                        "/data/gogear.db?" + opts + "&_journal_mode=WAL",
		"file:/data/g.db?cache=shared":           "file:/data/g.db?cache=shared&" + opts + "&_journal_mode=WAL",
		"g.db?_busy_timeout=100":                 "g.db?_busy_timeout=100&_txlock=immediate&_foreign_keys=on&_journal_mode=WAL",
		"g.db?_txlock=exclusive&_busy_timeout=1": "g.db?_txlock=exclusive&_busy_timeout=1&_foreign_keys=on&_journal_mode=WAL",
		"g.db?_fk=0":                             "g.db?_fk=0&_txlock=immediate&_busy_timeout=5000&_journal_mode=WAL",
		// A journal mode chosen by the operator is kept.
		"g.db?_journal_mode=DELETE": "g.db?_journal_mode=DELETE&" + opts,
		"g.db?_journal=TRUNCATE":    "g.db?_journal=TRUNCATE&" + opts,
		// In-memory databases have no WAL.
		":memory:":                        ":memory:?" + opts,
		"file::memory:?cache=shared":      "file::memory:?cache=shared&" + opts,
		"file:x?mode=memory&cache=shared": "file:x?mode=memory&cache=shared&" + opts,
		// An empty file is not a usable DSN: the caller must reject it first
		// (main's validateConfig does). The options-only result is what the
		// driver would turn into a file named after them.
		"": "?" + opts + "&_journal_mode=WAL",
	}
	for in, want := range cases {
		if got := SQLiteDSN(in); got != want {
			t.Errorf("SQLiteDSN(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every pooled connection of a file database runs in WAL mode, an operator's
// explicit journal mode wins, and an in-memory database is left alone.
func TestSQLiteDSN_JournalModeReachesTheDatabase(t *testing.T) {
	mode := func(dsn string) string {
		db, err := sql.Open("sqlite3", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var m string
		if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	dir := t.TempDir()
	if got := mode(SQLiteDSN(filepath.Join(dir, "wal.db"))); got != "wal" {
		t.Errorf("file database journal mode = %q, want wal", got)
	}
	if got := mode(SQLiteDSN(filepath.Join(dir, "del.db") + "?_journal_mode=DELETE")); got != "delete" {
		t.Errorf("explicit journal mode = %q, want delete", got)
	}
	if got := mode(SQLiteDSN(":memory:")); got != "memory" {
		t.Errorf("in-memory journal mode = %q, want memory", got)
	}
}
