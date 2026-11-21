package utils

import "testing"

func TestSQLiteDSN(t *testing.T) {
	cases := map[string]string{
		"/data/gogear.db":                        "/data/gogear.db?_txlock=immediate&_busy_timeout=5000&_foreign_keys=on",
		"file:/data/g.db?cache=shared":           "file:/data/g.db?cache=shared&_txlock=immediate&_busy_timeout=5000&_foreign_keys=on",
		"g.db?_busy_timeout=100":                 "g.db?_busy_timeout=100&_txlock=immediate&_foreign_keys=on",
		"g.db?_txlock=exclusive&_busy_timeout=1": "g.db?_txlock=exclusive&_busy_timeout=1&_foreign_keys=on",
		// An empty file is not a usable DSN: the caller must reject it first
		// (main's validateConfig does). The options-only result is what the
		// driver would turn into a file named after them.
		"":           "?_txlock=immediate&_busy_timeout=5000&_foreign_keys=on",
		":memory:":   ":memory:?_txlock=immediate&_busy_timeout=5000&_foreign_keys=on",
		"g.db?_fk=0": "g.db?_fk=0&_txlock=immediate&_busy_timeout=5000",
	}
	for in, want := range cases {
		if got := SQLiteDSN(in); got != want {
			t.Errorf("SQLiteDSN(%q) = %q, want %q", in, got, want)
		}
	}
}
