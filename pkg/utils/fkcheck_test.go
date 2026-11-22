package utils

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

const fkSchema = `
CREATE TABLE parent (id INTEGER PRIMARY KEY);
CREATE TABLE other (id INTEGER PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parentId INTEGER NOT NULL, otherId INTEGER,
	FOREIGN KEY (parentId) REFERENCES parent(id), FOREIGN KEY (otherId) REFERENCES other(id));`

// Foreign-key enforcement is per connection. Every connection of a pool that
// SQLiteDSN opened must have it on, and must refuse an orphan row.
func TestSQLiteDSN_ForeignKeysOnEveryPooledConnection(t *testing.T) {
	db, err := sql.Open("sqlite3", SQLiteDSN(filepath.Join(t.TempDir(), "fk.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(fkSchema); err != nil {
		t.Fatal(err)
	}

	const n = 6
	db.SetMaxOpenConns(n)
	ctx := context.Background()
	conns := make([]*sql.Conn, 0, n)
	for i := 0; i < n; i++ { // hold them all so each is a distinct connection
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		conns = append(conns, c)
	}
	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var on int
			if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&on); err != nil || on != 1 {
				t.Errorf("connection %d: foreign_keys = %d, err %v; want 1", i, on, err)
			}
			if _, err := c.ExecContext(ctx, `INSERT INTO child (parentId) VALUES (999)`); err == nil {
				t.Errorf("connection %d accepted an orphan row", i)
			}
		}()
	}
	wg.Wait()
}

// The check reports legacy orphans per table and parent, writes nothing, and
// is empty for consistent data.
func TestForeignKeyViolations(t *testing.T) {
	file := filepath.Join(t.TempDir(), "legacy.db")
	// Legacy data was written with enforcement off.
	legacy, err := sql.Open("sqlite3", file+"?_foreign_keys=off")
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec(fkSchema + `
INSERT INTO parent (id) VALUES (1);
INSERT INTO child (parentId, otherId) VALUES (1, NULL);
INSERT INTO child (parentId, otherId) VALUES (7, NULL);
INSERT INTO child (parentId, otherId) VALUES (8, NULL);
INSERT INTO child (parentId, otherId) VALUES (1, 5);`); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", SQLiteDSN(file))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	got, err := ForeignKeyViolations(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	want := []FKViolation{{"child", "other", 1}, {"child", "parent", 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %+v, want %+v", got, want)
	}
	var rowsLeft int
	if err := db.QueryRow(`SELECT COUNT(*) FROM child`).Scan(&rowsLeft); err != nil || rowsLeft != 4 {
		t.Fatalf("check must not change data: %d rows, err %v", rowsLeft, err)
	}

	// An existing orphan does not block opening, but a new one is refused.
	if _, err := db.Exec(`INSERT INTO child (parentId) VALUES (99)`); err == nil {
		t.Fatal("new orphan accepted")
	}

	if _, err := db.Exec(`DELETE FROM child WHERE parentId <> 1 OR otherId IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	got, err = ForeignKeyViolations(context.Background(), db)
	if err != nil || len(got) != 0 {
		t.Fatalf("consistent data: violations = %+v, err %v", got, err)
	}
}
