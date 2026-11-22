package utils

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// FKViolation summarizes the rows of one table that name a parent row that
// does not exist.
type FKViolation struct {
	Table  string // child table holding the orphan rows
	Parent string // table the foreign key points at
	Count  int
}

// ForeignKeyViolations runs PRAGMA foreign_key_check and summarizes the
// result per (table, parent), ordered by table then parent. It reads only: it
// never repairs or deletes rows. An empty result means the data is consistent.
func ForeignKeyViolations(ctx context.Context, db *sql.DB) ([]FKViolation, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()

	type key struct{ table, parent string }
	counts := map[key]int{}
	for rows.Next() {
		var table, parent string
		var rowid sql.NullInt64 // NULL for WITHOUT ROWID tables
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return nil, fmt.Errorf("foreign_key_check scan: %w", err)
		}
		counts[key{table, parent}]++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("foreign_key_check rows: %w", err)
	}

	out := make([]FKViolation, 0, len(counts))
	for k, n := range counts {
		out = append(out, FKViolation{Table: k.table, Parent: k.parent, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return out[i].Parent < out[j].Parent
	})
	return out, nil
}
