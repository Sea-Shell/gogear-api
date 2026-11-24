package dbupgrade

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	sqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
)

// These tests are the required tests of ARS-002 section 8. Every one runs the
// startup routine (Run) against real files in a temporary directory.

const appV = "v4.11.0"

// latest is the version of the newest real migration. The tests start a
// volume at schema 10 (the state before milestone A) and upgrade it to latest;
// synthetic migrations take the versions after it. latestS and migratedLine
// spell the same value as the backup file names and the startup line do.
var (
	latest       = latestRealMigration()
	latestS      = strconv.Itoa(latest)
	nextS        = strconv.Itoa(latest + 1)
	migratedLine = fmt.Sprintf("gogear-api migrations: version=%d dirty=false", latest)
)

func latestRealMigration() int {
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		panic(fmt.Sprintf("real migrations not found: %v", err))
	}
	n := 0
	for _, f := range files {
		v, err := strconv.Atoi(strings.SplitN(filepath.Base(f), "_", 2)[0])
		if err != nil {
			panic(fmt.Sprintf("migration file %s has no numeric version: %v", f, err))
		}
		n = max(n, v)
	}
	return n
}

// env is one database volume: a database file, its backup directory and a
// migrations directory that starts as a copy of the real one (every real version).
type env struct {
	t          *testing.T
	dbFile     string
	backupDir  string
	migrations string
	tick       int
}

// newEnv creates a volume. seedTo is the schema version the database starts
// at, with rows 'a' and 'b' in a table "marker"; 0 means no database file.
func newEnv(t *testing.T, seedTo uint) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{t: t, dbFile: filepath.Join(root, "gogear.db"), backupDir: filepath.Join(root, "backups"), migrations: filepath.Join(root, "migrations")}
	if err := os.Mkdir(e.migrations, 0o755); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil || len(real) == 0 {
		t.Fatalf("real migrations not found: %v", err)
	}
	for _, f := range real {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(e.migrations, filepath.Base(f)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if seedTo > 0 {
		db, err := sql.Open("sqlite3", e.dbFile+"?_journal_mode=WAL")
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
		if err != nil {
			t.Fatal(err)
		}
		m, err := migrate.NewWithDatabaseInstance("file://"+e.migrations, "sqlite3", driver)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Migrate(seedTo); err != nil {
			t.Fatalf("seed to %d: %v", seedTo, err)
		}
		if _, err := db.Exec(`CREATE TABLE marker (v TEXT); INSERT INTO marker VALUES ('a'), ('b')`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		if err := os.Chmod(e.dbFile, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

// addMigration adds a synthetic migration to the image.
func (e *env) addMigration(n int, name, up string) {
	e.t.Helper()
	base := filepath.Join(e.migrations, pad6(n)+"_"+name)
	if err := os.WriteFile(base+".up.sql", []byte(up), 0o644); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(base+".down.sql", []byte("SELECT 1;"), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func pad6(n int) string { return fmt.Sprintf("%06d", n) }

type outcome struct {
	served, hold bool
	err          error
	out          string
}

// run calls the startup routine once. Time advances one second per call of
// the clock, so every backup name is distinct and ordered.
func (e *env) run(policy Policy, restore string, opts ...func(*Config)) outcome {
	e.t.Helper()
	var out bytes.Buffer
	cfg := Config{
		DatabaseFile:   e.dbFile,
		BackupDir:      e.backupDir,
		MigrationsPath: e.migrations,
		AppVersion:     appV,
		RestoreRequest: restore,
		Policy:         policy,
		Out:            &out,
		now: func() time.Time {
			e.tick++
			return time.Date(2026, 11, 22, 18, 0, 0, 0, time.UTC).Add(time.Duration(e.tick) * time.Second)
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	db, hold, err := Run(context.Background(), cfg)
	if db != nil {
		db.Close()
	}
	return outcome{served: db != nil, hold: hold, err: err, out: out.String()}
}

func (e *env) schema() schemaState {
	e.t.Helper()
	return readSchemaOf(e.t, e.dbFile)
}

func readSchemaOf(t *testing.T, file string) schemaState {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+file+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := readSchema(context.Background(), db)
	if err != nil {
		t.Fatalf("read schema of %s: %v", file, err)
	}
	return s
}

func markersOf(t *testing.T, file string) []string {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+file+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT v FROM marker ORDER BY rowid`)
	if err != nil {
		t.Fatalf("read markers of %s: %v", file, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func (e *env) addMarker(v string) {
	e.t.Helper()
	db, err := sql.Open("sqlite3", e.dbFile)
	if err != nil {
		e.t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO marker VALUES (?)`, v); err != nil {
		e.t.Fatal(err)
	}
}

// backups lists the counted files, newest first.
func (e *env) backups() []backupFile {
	e.t.Helper()
	files, err := listBackups(e.backupDir)
	if err != nil {
		e.t.Fatal(err)
	}
	return files
}

func (e *env) backupOf(kind, schema string) backupFile {
	e.t.Helper()
	for _, f := range e.backups() {
		if f.kind == kind && f.schema == schema {
			return f
		}
	}
	e.t.Fatalf("no %s backup of schema %s in %v", kind, schema, e.backups())
	return backupFile{}
}

func (e *env) names() []string {
	var out []string
	entries, _ := os.ReadDir(e.backupDir)
	for _, f := range entries {
		out = append(out, f.Name())
	}
	return out
}

func mustContain(t *testing.T, out string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(out, p) {
			t.Errorf("output lacks %q:\n%s", p, out)
		}
	}
}

func mustNotContain(t *testing.T, out string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(out, p) {
			t.Errorf("output has %q:\n%s", p, out)
		}
	}
}

// openWriter opens a second connection that stays open, the way a second
// process would hold the database.
func (e *env) openWriter() *sql.DB {
	e.t.Helper()
	db, err := sql.Open("sqlite3", e.dbFile)
	if err != nil {
		e.t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	e.t.Cleanup(func() { db.Close() })
	return db
}

// 1. The backup holds rows that exist only in the write-ahead log.
func TestBackupIsConsistentAndHoldsWALOnlyRows(t *testing.T) {
	e := newEnv(t, 10)
	w := e.openWriter()
	for _, q := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA wal_autocheckpoint=0`, `INSERT INTO marker VALUES ('wal1')`, `INSERT INTO marker VALUES ('wal2')`} {
		if _, err := w.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if info, err := os.Stat(e.dbFile + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("the rows must be only in the -wal file: %v", err)
	}

	res := e.run(DefaultPolicy(), "")
	if res.err != nil || !res.served {
		t.Fatalf("run: served=%v err=%v\n%s", res.served, res.err, res.out)
	}
	b := e.backupOf(kindPremigration, "10")
	if err := verifyBackup(context.Background(), b.path, schemaState{has: true, version: 10}); err != nil {
		t.Errorf("backup does not verify: %v", err)
	}
	if got, want := markersOf(t, b.path), []string{"a", "b", "wal1", "wal2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("backup rows = %v, want %v", got, want)
	}
	if s := e.schema(); !s.has || s.version != int64(latest) || s.dirty {
		t.Errorf("database schema = %+v, want %d clean", s, latest)
	}
	mustContain(t, res.out, "gogear-api backup: created file="+b.path+" kind=premigration schema=10 app="+appV+" integrity=ok", migratedLine)
	if info, _ := os.Stat(b.path); info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, want the database's 0600", info.Mode().Perm())
	}
	if info, _ := os.Stat(e.backupDir); info.Mode().Perm() != 0o700 {
		t.Errorf("backup directory mode = %v, want 0700", info.Mode().Perm())
	}
}

// 2. A commit by another connection between the snapshot and the second
// data_version read makes the backup a failed one.
func TestConcurrentCommitFailsTheBackup(t *testing.T) {
	e := newEnv(t, 10)
	w := e.openWriter()
	commit := func(c *Config) {
		c.hooks.afterSnapshot = func() {
			if _, err := w.Exec(`INSERT INTO marker VALUES ('late')`); err != nil {
				t.Errorf("commit from the other connection: %v", err)
			}
		}
	}

	res := e.run(DefaultPolicy(), "", commit)
	if res.err == nil || res.served {
		t.Fatalf("Refuse must fail: served=%v err=%v", res.served, res.err)
	}
	mustContain(t, res.out, "gogear-api backup: FAILED reason=", "the database changed while the backup was made", "database unchanged at schema=10")
	if s := e.schema(); s.version != 10 {
		t.Errorf("schema = %d, a migration ran", s.version)
	}
	if names := e.names(); len(names) != 0 {
		t.Errorf("files left in the backup directory: %v", names)
	}
}

// 3. Nothing pending, and an empty volume.
func TestNoBackupWhenNothingIsPendingOrOnFirstStart(t *testing.T) {
	t.Run("nothing pending", func(t *testing.T) {
		e := newEnv(t, 10)
		if res := e.run(DefaultPolicy(), ""); res.err != nil {
			t.Fatal(res.err)
		}
		before := e.names()
		res := e.run(DefaultPolicy(), "")
		if res.err != nil || !res.served {
			t.Fatalf("second run: %v", res.err)
		}
		if after := e.names(); !reflect.DeepEqual(before, after) {
			t.Errorf("files changed from %v to %v", before, after)
		}
		mustContain(t, res.out, "gogear-api backup: skipped reason=no-pending", migratedLine)
		mustNotContain(t, res.out, "created")
	})
	t.Run("empty volume", func(t *testing.T) {
		e := newEnv(t, 0)
		res := e.run(DefaultPolicy(), "")
		if res.err != nil || !res.served {
			t.Fatalf("run: %v\n%s", res.err, res.out)
		}
		if names := e.names(); len(names) != 0 {
			t.Errorf("backup made on first start: %v", names)
		}
		mustContain(t, res.out, "gogear-api backup: skipped reason=first-start", migratedLine)
	})
}

// 4. The backup cannot be made.
func TestBackupFailure(t *testing.T) {
	causes := []struct {
		name   string
		break_ func(t *testing.T, e *env)
		fix    func(t *testing.T, e *env)
	}{
		{
			name: "unwritable directory",
			break_: func(t *testing.T, e *env) {
				if os.Geteuid() == 0 {
					t.Skip("root ignores directory permissions")
				}
				if err := os.Mkdir(e.backupDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(e.backupDir, 0o500); err != nil {
					t.Fatal(err)
				}
			},
			fix: func(t *testing.T, e *env) {
				if err := os.Chmod(e.backupDir, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory path is a regular file",
			break_: func(t *testing.T, e *env) {
				if err := os.WriteFile(e.backupDir, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			fix: func(t *testing.T, e *env) {
				if err := os.Remove(e.backupDir); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, c := range causes {
		t.Run(c.name+"/Refuse", func(t *testing.T) {
			e := newEnv(t, 10)
			c.break_(t, e)
			res := e.run(Policy{OnBackupFailure: Refuse, OnMigrationFailure: Keep}, "")
			if res.err == nil || res.served {
				t.Fatalf("must refuse: served=%v err=%v", res.served, res.err)
			}
			if s := e.schema(); s.version != 10 {
				t.Errorf("schema = %d, want 10", s.version)
			}
			mustContain(t, res.out, "gogear-api backup: FAILED reason=", "no migration applied, database unchanged at schema=10")
			if c.name == "unwritable directory" {
				mustContain(t, res.out, "is not writable")
			}
			for _, f := range e.names() {
				if strings.HasSuffix(f, ".partial") || backupPattern.MatchString(f) {
					t.Errorf("file left behind: %s", f)
				}
			}

			c.fix(t, e)
			res = e.run(Policy{OnBackupFailure: Refuse, OnMigrationFailure: Keep}, "")
			if res.err != nil || !res.served {
				t.Fatalf("after the fix: %v\n%s", res.err, res.out)
			}
			e.backupOf(kindPremigration, "10")
			if s := e.schema(); s.version != int64(latest) {
				t.Errorf("schema = %d after the fix, want %d", s.version, latest)
			}
		})
		t.Run(c.name+"/MigrateWithoutBackup", func(t *testing.T) {
			e := newEnv(t, 10)
			c.break_(t, e)
			res := e.run(Policy{OnBackupFailure: MigrateWithoutBackup, OnMigrationFailure: Keep}, "")
			if res.err != nil || !res.served {
				t.Fatalf("must migrate: %v\n%s", res.err, res.out)
			}
			if s := e.schema(); s.version != int64(latest) {
				t.Errorf("schema = %d, want %d", s.version, latest)
			}
			mustContain(t, res.out, "gogear-api backup: WARNING backup failed (reason=", "migrating without a backup")
		})
	}
}

// 5. Four upgrades leave three backups; the oldest goes only after the fourth
// is verified; a file that is not ours stays.
func TestRetentionKeepsThreeAfterVerifying(t *testing.T) {
	e := newEnv(t, 10)
	if err := os.MkdirAll(e.backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := map[string]string{"notes.txt": "mine", "gogear-other.db": "mine too", "gogear-20200101T000000Z-premigration-schema1-appx.db.bak": "mine"}
	for name, content := range foreign {
		if err := os.WriteFile(filepath.Join(e.backupDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var last outcome
	for n := latest; n <= latest+3; n++ {
		if n > latest {
			e.addMigration(n, "synthetic", "CREATE TABLE synthetic"+pad6(n)+" (id INTEGER);")
		}
		last = e.run(DefaultPolicy(), "")
		if last.err != nil {
			t.Fatalf("upgrade to %d: %v\n%s", n, last.err, last.out)
		}
	}

	var schemas []string
	for _, f := range e.backups() {
		if f.kind == kindPremigration {
			schemas = append(schemas, f.schema)
		}
	}
	if want := []string{strconv.Itoa(latest + 2), strconv.Itoa(latest + 1), latestS}; !reflect.DeepEqual(schemas, want) {
		t.Fatalf("pre-migration backups = %v, want %v", schemas, want)
	}
	created := strings.Index(last.out, "created file=")
	removed := strings.Index(last.out, "removed file=")
	if created < 0 || removed < created {
		t.Errorf("the oldest backup must go after the new one is created:\n%s", last.out)
	}
	mustContain(t, last.out, "schema10-app"+appV+".db reason=retention")
	for name, content := range foreign {
		got, err := os.ReadFile(filepath.Join(e.backupDir, name))
		if err != nil || string(got) != content {
			t.Errorf("foreign file %s changed: %q, %v", name, got, err)
		}
	}
}

// upgraded returns a volume that went from 10 to the latest version once, so it
// holds one schema-10 backup, plus a row that exists only in the upgraded database.
func upgraded(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, 10)
	if res := e.run(DefaultPolicy(), ""); res.err != nil {
		t.Fatalf("upgrade: %v\n%s", res.err, res.out)
	}
	e.addMarker("after-upgrade")
	return e
}

// 6. Restore through the request value.
func TestRestore(t *testing.T) {
	e := upgraded(t)
	old := e.backupOf(kindPremigration, "10")
	oldRows := markersOf(t, old.path)

	res := e.run(DefaultPolicy(), old.name)
	if res.err != nil || !res.hold || res.served {
		t.Fatalf("restore: hold=%v served=%v err=%v\n%s", res.hold, res.served, res.err, res.out)
	}
	// Checked before the test reads the database itself, which creates them.
	for _, side := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(e.dbFile + side); err == nil {
			t.Errorf("%s left next to the restored database", side)
		}
	}
	pre := e.backupOf(kindPrerestore, latestS)
	if got := markersOf(t, pre.path); !reflect.DeepEqual(got, []string{"a", "b", "after-upgrade"}) {
		t.Errorf("pre-restore backup rows = %v", got)
	}
	if got := markersOf(t, e.dbFile); !reflect.DeepEqual(got, oldRows) {
		t.Errorf("database rows = %v, want the backup's %v", got, oldRows)
	}
	if s := e.schema(); s.version != 10 {
		t.Errorf("schema = %d, no migration may run in restore mode", s.version)
	}
	mustContain(t, res.out,
		"gogear-api backup: restored file="+old.path+" schema=10 app="+appV+"; replaced database kept as file="+pre.path,
		"gogear-api backup: restore mode schema=10 latest="+latestS+"; not migrating and not serving")
	mustNotContain(t, res.out, "gogear-api migrations:")

	t.Run("same value again", func(t *testing.T) {
		before := len(e.backups())
		res := e.run(DefaultPolicy(), old.name)
		if res.err != nil || !res.hold {
			t.Fatalf("hold=%v err=%v", res.hold, res.err)
		}
		mustContain(t, res.out, "gogear-api backup: restore already done value="+old.name)
		if len(e.backups()) != before {
			t.Errorf("a repeat of a done request made a backup")
		}
	})
	t.Run("with a tag", func(t *testing.T) {
		before := len(e.backups())
		e.addMarker("changed-in-restore-mode")
		res := e.run(DefaultPolicy(), old.name+"#2")
		if res.err != nil || !res.hold {
			t.Fatalf("hold=%v err=%v\n%s", res.hold, res.err, res.out)
		}
		mustContain(t, res.out, "gogear-api backup: restored file="+old.path)
		if got := markersOf(t, e.dbFile); !reflect.DeepEqual(got, oldRows) {
			t.Errorf("database rows = %v, want %v", got, oldRows)
		}
		if len(e.backups()) != before+1 {
			t.Errorf("a new request must keep the replaced database as a backup")
		}
	})
	for _, bad := range []struct{ name, value, reason string }{
		{"path escape", "../x.db", "reason=invalid-name"},
		{"unknown name", "gogear-20200101T000000Z-premigration-schema9-appx.db", "reason=not-found"},
	} {
		t.Run(bad.name, func(t *testing.T) {
			before := markersOf(t, e.dbFile)
			names := e.names()
			res := e.run(DefaultPolicy(), bad.value)
			if res.err == nil || res.served || res.hold {
				t.Fatalf("must fail: %+v", res)
			}
			mustContain(t, res.out, "gogear-api backup: restore FAILED "+bad.reason+"; database unchanged")
			if got := markersOf(t, e.dbFile); !reflect.DeepEqual(got, before) {
				t.Errorf("database changed: %v", got)
			}
			if !reflect.DeepEqual(names, e.names()) {
				t.Errorf("files changed: %v", e.names())
			}
		})
	}
	t.Run("backup that fails verification", func(t *testing.T) {
		name := "gogear-20200101T000000Z-premigration-schema9-appx.db"
		if err := os.WriteFile(filepath.Join(e.backupDir, name), []byte("this is not a database"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := markersOf(t, e.dbFile)
		res := e.run(DefaultPolicy(), name)
		if res.err == nil {
			t.Fatalf("must fail:\n%s", res.out)
		}
		mustContain(t, res.out, "restore FAILED reason=verify; database unchanged")
		if got := markersOf(t, e.dbFile); !reflect.DeepEqual(got, before) {
			t.Errorf("database changed: %v", got)
		}
	})
}

// 7. Restoring a backup whose schema is the image's latest serves.
func TestRestoreToTheSameSchemaServes(t *testing.T) {
	e := upgraded(t)
	if res := e.run(DefaultPolicy(), e.backupOf(kindPremigration, "10").name); !res.hold {
		t.Fatalf("setup: %+v", res)
	}
	undo := e.backupOf(kindPrerestore, latestS)

	res := e.run(DefaultPolicy(), undo.name)
	if res.err != nil || !res.served || res.hold {
		t.Fatalf("served=%v hold=%v err=%v\n%s", res.served, res.hold, res.err, res.out)
	}
	if got := markersOf(t, e.dbFile); !reflect.DeepEqual(got, []string{"a", "b", "after-upgrade"}) {
		t.Errorf("rows = %v", got)
	}
	mustContain(t, res.out, migratedLine)
	mustNotContain(t, res.out, "restore mode")
}

const failingMigration = "CREATE TABLE half_done (id INTEGER); INSERT INTO table_that_does_not_exist VALUES (1);"

func failingEnv(t *testing.T, policy Policy) (*env, outcome) {
	t.Helper()
	e := upgraded(t)
	e.addMarker("committed-before-attempt")
	e.addMigration(latest+1, "bad", failingMigration)
	res := e.run(policy, "")
	if res.err == nil || res.served || res.hold {
		t.Fatalf("the migration must fail: %+v", res)
	}
	return e, res
}

// 8. A failed migration under Keep.
func TestFailedMigrationKeep(t *testing.T) {
	e, res := failingEnv(t, Policy{OnBackupFailure: Refuse, OnMigrationFailure: Keep})
	b := e.backupOf(kindPremigration, latestS)
	mustContain(t, res.out, "gogear-api backup: migration failed from="+latestS+" target="+nextS+"; the state before the attempt is in file="+b.path)
	mustNotContain(t, res.out, "restored after failed migration")
	if got := markersOf(t, b.path); !reflect.DeepEqual(got, []string{"a", "b", "after-upgrade", "committed-before-attempt"}) {
		t.Errorf("backup rows = %v", got)
	}
	var state migrationFailed
	if found, err := (&runner{dir: e.backupDir}).readState(migrationFailedFile, &state); err != nil || !found {
		t.Fatalf("%s: found=%v err=%v", migrationFailedFile, found, err)
	}
	if state.AppVersion != appV || state.FromVersion != latestS || state.TargetVersion != nextS || state.Backup != b.name {
		t.Errorf("state = %+v", state)
	}
	if s := e.schema(); !s.dirty {
		t.Errorf("schema = %+v, want dirty after the failed attempt", s)
	}

	before := e.names()
	res = e.run(Policy{OnBackupFailure: Refuse, OnMigrationFailure: Keep}, "")
	if res.err == nil || res.served {
		t.Fatalf("a dirty database must not start: %+v", res)
	}
	mustContain(t, res.out, "gogear-api backup: dirty schema="+nextS+"; a previous migration failed. Restore with: kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP="+b.name)
	if after := e.names(); !reflect.DeepEqual(before, after) {
		t.Errorf("a dirty start changed the backup directory: %v -> %v", before, after)
	}

	// The documented way out: restore the named backup.
	res = e.run(Policy{OnBackupFailure: Refuse, OnMigrationFailure: Keep}, b.name)
	if res.err != nil || !res.hold {
		t.Fatalf("restore of a dirty database: %+v\n%s", res, res.out)
	}
	if s := e.schema(); s.version != int64(latest) || s.dirty {
		t.Errorf("schema = %+v, want %d clean", s, latest)
	}
	e.backupOf(kindPrerestore, nextS)
}

// 9. A failed migration under Restore.
func TestFailedMigrationRestore(t *testing.T) {
	policy := Policy{OnBackupFailure: Refuse, OnMigrationFailure: Restore}
	e, res := failingEnv(t, policy)
	first := e.backupOf(kindPremigration, latestS)
	mustContain(t, res.out, "migration failed from="+latestS+" target="+nextS, "gogear-api backup: restored after failed migration file="+first.path+" schema="+latestS)
	if s := e.schema(); s.version != int64(latest) || s.dirty {
		t.Fatalf("schema = %+v, want %d clean", s, latest)
	}
	if got := markersOf(t, e.dbFile); !reflect.DeepEqual(got, []string{"a", "b", "after-upgrade", "committed-before-attempt"}) {
		t.Errorf("rows after restore = %v", got)
	}
	older := e.backupOf(kindPremigration, "10")
	olderBytes, err := os.ReadFile(older.path)
	if err != nil {
		t.Fatal(err)
	}

	res = e.run(policy, "")
	if res.err == nil {
		t.Fatalf("the same migration must fail again:\n%s", res.out)
	}
	mustContain(t, res.out, "gogear-api backup: removed file="+first.path+" reason=replaced-retry", "restored after failed migration")
	var schemaLatest int
	for _, f := range e.backups() {
		if f.kind == kindPremigration && f.schema == latestS {
			schemaLatest++
		}
	}
	if schemaLatest != 1 {
		t.Errorf("%d schema-%d backups, the retry must replace the earlier one", schemaLatest, latest)
	}
	if _, err := os.Stat(first.path); err == nil {
		t.Errorf("the superseded backup is still there")
	}
	if got, err := os.ReadFile(older.path); err != nil || !bytes.Equal(got, olderBytes) {
		t.Errorf("the older backup changed: %v", err)
	}
	if s := e.schema(); s.version != int64(latest) || s.dirty {
		t.Errorf("schema = %+v after the retry, want %d clean", s, latest)
	}

	// A fixed release (same app version here) migrates, and the record goes.
	e.addMigration(latest+1, "bad", "CREATE TABLE fixed (id INTEGER);")
	res = e.run(policy, "")
	if res.err != nil || !res.served {
		t.Fatalf("fixed migration: %v\n%s", res.err, res.out)
	}
	if _, err := os.Stat(filepath.Join(e.backupDir, migrationFailedFile)); err == nil {
		t.Errorf("%s survived a successful migration", migrationFailedFile)
	}
}

// Both values of each policy switch are exercised above; this pins the
// defaults the owner's open answers select.
func TestDefaultPolicy(t *testing.T) {
	if p := DefaultPolicy(); p.OnBackupFailure != Refuse || p.OnMigrationFailure != Keep {
		t.Errorf("DefaultPolicy() = %+v, want Refuse and Keep until Q-8 and Q-9 are answered", p)
	}
}

func TestResolveBackupDir(t *testing.T) {
	for _, tc := range []struct{ file, dir, want string }{
		{"/data/gogear.db", "", "/data/backups"},
		{"/data/gogear.db?_fk=0", "  ", "/data/backups"},
		{"file:/data/gogear.db", "b", "/data/b"},
		{"/data/gogear.db", "/mnt/backups/", "/mnt/backups"},
		{"gogear.db", "", "backups"},
	} {
		if got := ResolveBackupDir(tc.file, tc.dir); got != tc.want {
			t.Errorf("ResolveBackupDir(%q, %q) = %q, want %q", tc.file, tc.dir, got, tc.want)
		}
	}
}

func TestPartialFilesAreCleanedAndForeignOnesKept(t *testing.T) {
	e := newEnv(t, uint(latest))
	if err := os.MkdirAll(e.backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	partial := "gogear-20200101T000000Z-premigration-schema9-appx.db.partial"
	other := "somebody-elses.partial"
	for _, n := range []string{partial, other} {
		if err := os.WriteFile(filepath.Join(e.backupDir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if res := e.run(DefaultPolicy(), ""); res.err != nil {
		t.Fatal(res.err)
	}
	if _, err := os.Stat(filepath.Join(e.backupDir, partial)); err == nil {
		t.Errorf("our partial file was not deleted")
	}
	if _, err := os.Stat(filepath.Join(e.backupDir, other)); err != nil {
		t.Errorf("a file that is not ours was touched: %v", err)
	}
}

// Retention never deletes the backup just made, even when its name carries an
// earlier time than three existing backups (a clock that stepped back).
func TestRetentionNeverDeletesTheNewBackup(t *testing.T) {
	e := newEnv(t, 10)
	if err := os.MkdirAll(e.backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []string{"20300101T000000Z", "20300102T000000Z", "20300103T000000Z"} {
		name := "gogear-" + stamp + "-premigration-schema9-appx.db"
		if err := os.WriteFile(filepath.Join(e.backupDir, name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res := e.run(DefaultPolicy(), "")
	if res.err != nil || !res.served {
		t.Fatalf("run: %v\n%s", res.err, res.out)
	}
	b := e.backupOf(kindPremigration, "10")
	if err := verifyBackup(context.Background(), b.path, schemaState{has: true, version: 10}); err != nil {
		t.Errorf("the new backup is gone or broken: %v", err)
	}
	var n int
	for _, f := range e.backups() {
		if f.kind == kindPremigration {
			n++
		}
	}
	if n != keepPerKind {
		t.Errorf("%d pre-migration backups, want %d", n, keepPerKind)
	}
}
