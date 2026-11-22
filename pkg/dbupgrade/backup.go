package dbupgrade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	kindPremigration = "premigration"
	kindPrerestore   = "prerestore"

	// keepPerKind is how many backups of each kind retention leaves (P-38).
	keepPerKind = 3
)

// backupPattern matches the only files the routine ever counts or deletes
// (ARS-002 section 2). Group 1 is the time, 2 the kind, 3 the schema version
// or "none", 4 the application label.
var backupPattern = regexp.MustCompile(`^gogear-(\d{8}T\d{6}Z)-(premigration|prerestore)-schema(\d+|none)-app([A-Za-z0-9._-]+)\.db$`)

// partialPattern matches the temporary name of a backup being written.
var partialPattern = regexp.MustCompile(strings.TrimSuffix(backupPattern.String(), "$") + `\.partial$`)

var unsafeLabel = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// schemaState is what golang-migrate's schema_migrations table says,
// read with plain SQL so that reading it never writes.
type schemaState struct {
	has     bool
	version int64
	dirty   bool
}

func (s schemaState) label() string {
	if !s.has {
		return "none"
	}
	return strconv.FormatInt(s.version, 10)
}

// pending reports whether a migration newer than the database's version exists.
func (s schemaState) pending(latest uint) bool {
	return !s.has || s.version < int64(latest)
}

func stateFromLabel(label string) (schemaState, error) {
	if label == "none" {
		return schemaState{}, nil
	}
	n, err := strconv.ParseInt(label, 10, 64)
	if err != nil {
		return schemaState{}, fmt.Errorf("schema label %q: %w", label, err)
	}
	return schemaState{has: true, version: n}, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readSchema reads the version and dirty flag. A missing table or an empty
// table, and golang-migrate's "no version" row (-1), mean no version.
func readSchema(ctx context.Context, q queryer) (schemaState, error) {
	var tables int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&tables); err != nil {
		return schemaState{}, fmt.Errorf("read schema table: %w", err)
	}
	if tables == 0 {
		return schemaState{}, nil
	}
	var version int64
	var dirty bool
	err := q.QueryRowContext(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return schemaState{}, nil
	}
	if err != nil {
		return schemaState{}, fmt.Errorf("read schema version: %w", err)
	}
	return schemaState{has: version >= 0, version: version, dirty: dirty}, nil
}

// backupFile is a file in the backup directory that matches backupPattern.
type backupFile struct {
	name, path, kind, stamp, schema string
	mod                             time.Time
}

// listBackups returns the matching files, newest first. A missing directory
// is an empty list.
func listBackups(dir string) ([]backupFile, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []backupFile
	for _, e := range entries {
		m := backupPattern.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, backupFile{name: e.Name(), path: filepath.Join(dir, e.Name()), kind: m[2], stamp: m[1], schema: m[3], mod: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.stamp != b.stamp {
			return a.stamp > b.stamp
		}
		if !a.mod.Equal(b.mod) {
			return a.mod.After(b.mod)
		}
		return a.name > b.name
	})
	return out, nil
}

// appLabel is the application version as it appears in a backup name.
func appLabel(version string) string {
	label := unsafeLabel.ReplaceAllString(version, "_")
	if label == "" {
		return "unknown"
	}
	return label
}

// freeName returns a backup name that exists neither as a final nor as a
// partial file: the plain name, or the name with -2, -3 and so on.
func (r *runner) freeName(kind string, src schemaState) (string, error) {
	base := fmt.Sprintf("gogear-%s-%s-schema%s-app%s", r.now().UTC().Format("20060102T150405Z"), kind, src.label(), appLabel(r.cfg.AppVersion))
	for n := 1; ; n++ {
		name := base + ".db"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.db", base, n)
		}
		taken := false
		for _, candidate := range []string{name, name + ".partial"} {
			if _, err := os.Lstat(filepath.Join(r.dir, candidate)); err == nil {
				taken = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if !taken {
			return name, nil
		}
	}
}

// snapshot makes one verified backup of the database behind pool and returns
// its path (ARS-002 section 3 step 10). src is the database's schema state,
// read by the caller. replace, when not empty, is the backup of an earlier
// failed attempt that this backup supersedes instead of normal retention.
func (r *runner) snapshot(ctx context.Context, pool *sql.DB, src schemaState, kind, replace string) (final string, err error) {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(r.dbFile); statErr == nil {
		mode = info.Mode().Perm()
	}
	name, err := r.freeName(kind, src)
	if err != nil {
		return "", err
	}
	final = filepath.Join(r.dir, name)
	partial := final + ".partial"
	defer func() {
		if err != nil {
			_ = os.Remove(partial)
		}
	}()

	conn, err := pool.Conn(ctx)
	if err != nil {
		return "", fmt.Errorf("open snapshot connection: %w", err)
	}
	defer conn.Close()

	var before, after int64
	if err := conn.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&before); err != nil {
		return "", fmt.Errorf("read data_version: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, partial); err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	if r.cfg.hooks.afterSnapshot != nil {
		r.cfg.hooks.afterSnapshot()
	}
	if err := verifyBackup(ctx, partial, src); err != nil {
		return "", err
	}
	if err := conn.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&after); err != nil {
		return "", fmt.Errorf("read data_version: %w", err)
	}
	if before != after {
		return "", errors.New("the database changed while the backup was made")
	}
	if err := os.Chmod(partial, mode); err != nil {
		return "", fmt.Errorf("set backup permissions: %w", err)
	}
	if err := syncFile(partial); err != nil {
		return "", fmt.Errorf("sync backup: %w", err)
	}
	if err := os.Rename(partial, final); err != nil {
		return "", fmt.Errorf("publish backup: %w", err)
	}
	if err := syncDir(r.dir); err != nil {
		return "", fmt.Errorf("sync backup directory: %w", err)
	}
	r.say(levelInfo, "created file=%s kind=%s schema=%s app=%s integrity=ok", final, kind, src.label(), appLabel(r.cfg.AppVersion))

	if replace != "" && replace != final {
		r.remove(replace, "replaced-retry")
	} else {
		r.applyRetention(kind)
	}
	return final, nil
}

// applyRetention keeps the newest keepPerKind backups of a kind.
func (r *runner) applyRetention(kind string) {
	files, err := listBackups(r.dir)
	if err != nil {
		r.say(levelWarn, "retention skipped (reason=%v)", err)
		return
	}
	kept := 0
	for _, f := range files {
		if f.kind != kind {
			continue
		}
		if kept++; kept > keepPerKind {
			r.remove(f.path, "retention")
		}
	}
}

func (r *runner) remove(path, reason string) {
	if err := os.Remove(path); err != nil {
		r.say(levelWarn, "could not remove file=%s reason=%v", path, err)
		return
	}
	r.say(levelInfo, "removed file=%s reason=%s", path, reason)
}

// verifyBackup opens the file read-only, with none of the service's options
// (its WAL mode would write), and checks it is a whole database whose schema
// state is want (ARS-002 section 5).
func verifyBackup(ctx context.Context, path string, want schemaState) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: abs}
	db, err := sql.Open("sqlite3", u.String()+"?mode=ro")
	if err != nil {
		return fmt.Errorf("verify: open: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("verify: integrity_check: %w", err)
	}
	var results []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return fmt.Errorf("verify: integrity_check: %w", err)
		}
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("verify: integrity_check: %w", err)
	}
	rows.Close()
	if len(results) != 1 || results[0] != "ok" {
		return fmt.Errorf("verify: integrity_check reported %q", strings.Join(results, "; "))
	}

	got, err := readSchema(ctx, db)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if got != want {
		return fmt.Errorf("verify: backup has schema=%s dirty=%t, expected schema=%s dirty=%t", got.label(), got.dirty, want.label(), want.dirty)
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func syncDir(dir string) error { return syncFile(dir) }
