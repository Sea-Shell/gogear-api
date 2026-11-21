package utils

import "testing"

func TestSQLiteDSN(t *testing.T) {
	cases := map[string]string{
		"/data/gogear.db":                        "/data/gogear.db?_txlock=immediate&_busy_timeout=5000",
		"file:/data/g.db?cache=shared":           "file:/data/g.db?cache=shared&_txlock=immediate&_busy_timeout=5000",
		"g.db?_busy_timeout=100":                 "g.db?_busy_timeout=100&_txlock=immediate",
		"g.db?_txlock=exclusive&_busy_timeout=1": "g.db?_txlock=exclusive&_busy_timeout=1",
	}
	for in, want := range cases {
		if got := SQLiteDSN(in); got != want {
			t.Errorf("SQLiteDSN(%q) = %q, want %q", in, got, want)
		}
	}
}
