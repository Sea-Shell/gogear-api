// Package dbupgrade owns what happens to the database between process start
// and the first request: restoring a requested backup, backing the database up
// before pending schema migrations (BR-038), migrating, and reporting each
// step on stdout. Exact behavior: ARS-002 and ADR-006.
package dbupgrade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	sqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"

	"github.com/Sea-Shell/gogear-api/pkg/utils"
)

const linePrefix = "gogear-api backup: "

// Config is everything Run needs.
type Config struct {
	// DatabaseFile is database.file as configured, DSN options included.
	DatabaseFile string
	// BackupDir is database.backup-dir as configured; see ResolveBackupDir.
	BackupDir string
	// MigrationsPath is the directory with the golang-migrate files.
	MigrationsPath string
	// AppVersion is the build's version; it is part of every backup name.
	AppVersion string
	// RestoreRequest is the value of GOGEAR_RESTORE_BACKUP, empty if unset.
	RestoreRequest string
	Policy         Policy
	// Out receives the announcement lines; nil means os.Stdout. Deploy checks
	// parse these lines, so they never go through the log level filter.
	Out io.Writer
	Log *zap.SugaredLogger

	// Test seams.
	now   func() time.Time
	hooks struct{ afterSnapshot func() }
}

// DatabasePath returns the file path inside a configured database.file value.
func DatabasePath(file string) string {
	base, _, _ := strings.Cut(file, "?")
	return strings.TrimPrefix(base, "file:")
}

// ResolveBackupDir returns the backup directory for a configuration: the
// directory "backups" next to the database file when none is set, and a
// relative value resolved against the database file's directory.
func ResolveBackupDir(databaseFile, backupDir string) string {
	dbDir := filepath.Dir(DatabasePath(databaseFile))
	switch dir := strings.TrimSpace(backupDir); {
	case dir == "":
		return filepath.Join(dbDir, "backups")
	case filepath.IsAbs(dir):
		return filepath.Clean(dir)
	default:
		return filepath.Join(dbDir, dir)
	}
}

type logLevel int

const (
	levelInfo logLevel = iota
	levelWarn
	levelError
)

type runner struct {
	cfg    Config
	dbFile string
	dir    string
	out    io.Writer
	log    *zap.SugaredLogger
	now    func() time.Time
}

func (r *runner) say(level logLevel, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.out, linePrefix+msg)
	switch level {
	case levelError:
		r.log.Error(linePrefix + msg)
	case levelWarn:
		r.log.Warn(linePrefix + msg)
	default:
		r.log.Info(linePrefix + msg)
	}
}

// Run is the startup routine of ARS-002 section 3. It returns the open
// database pool when the service may start serving, and hold=true (with no
// pool) when a requested restore left an older schema and the service must
// neither migrate nor serve. Any error means the process must exit.
func Run(ctx context.Context, cfg Config) (db *sql.DB, hold bool, err error) {
	r := &runner{
		cfg:    cfg,
		dbFile: DatabasePath(cfg.DatabaseFile),
		dir:    ResolveBackupDir(cfg.DatabaseFile, cfg.BackupDir),
		out:    cfg.Out,
		log:    cfg.Log,
		now:    cfg.now,
	}
	if r.out == nil {
		r.out = os.Stdout
	}
	if r.log == nil {
		r.log = zap.NewNop().Sugar()
	}
	if r.now == nil {
		r.now = time.Now
	}
	if cfg.Policy.OnBackupFailure == 0 || cfg.Policy.OnMigrationFailure == 0 {
		return nil, false, errors.New("dbupgrade: policy is not set; use DefaultPolicy")
	}
	return r.run(ctx)
}

func (r *runner) run(ctx context.Context) (db *sql.DB, hold bool, err error) {
	// 1 and 2: housekeeping. It is advisory: an unreadable backup directory
	// must reach the backup step, where the failure policy decides.
	r.cleanPartials()
	r.listKept()

	// 3.
	existed := hasContent(r.dbFile)

	// 4.
	if request := r.cfg.RestoreRequest; request != "" {
		var done restoreDone
		if _, err := r.readState(restoreDoneFile, &done); err != nil {
			return nil, false, fmt.Errorf("read %s: %w", restoreDoneFile, err)
		}
		if requestDone(done, request) {
			r.say(levelInfo, "restore already done value=%s; remove GOGEAR_RESTORE_BACKUP or add #<tag> to restore again", request)
		} else {
			if err := r.restoreRequested(ctx, request, existed); err != nil {
				return nil, false, err
			}
			existed = true
		}
	}

	// 5.
	pool, err := sql.Open("sqlite3", utils.SQLiteDSN(r.cfg.DatabaseFile))
	if err != nil {
		return nil, false, fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err != nil || hold {
			pool.Close()
		}
	}()

	// 6.
	state, err := readSchema(ctx, pool)
	if err != nil {
		return nil, false, err
	}
	latest, err := latestVersion(r.cfg.MigrationsPath)
	if err != nil {
		return nil, false, err
	}

	// 7.
	if state.dirty {
		r.say(levelError, "dirty schema=%s; a previous migration failed. Restore with: kubectl -n seashell set env deployment/gogear-api GOGEAR_RESTORE_BACKUP=%s", state.label(), r.newestPremigrationName())
		return nil, false, fmt.Errorf("database is dirty at schema %s: a previous migration failed", state.label())
	}

	// 8.
	pending := state.pending(latest)
	if r.cfg.RestoreRequest != "" && pending {
		r.say(levelInfo, "restore mode schema=%s latest=%d; not migrating and not serving. Deploy the release that matches schema %s (app=<version> of the backup), or remove GOGEAR_RESTORE_BACKUP to migrate again", state.label(), latest, state.label())
		return nil, true, nil
	}

	// 9 to 11.
	switch {
	case !pending:
		r.say(levelInfo, "skipped reason=no-pending")
	default:
		backup := ""
		if !existed {
			r.say(levelInfo, "skipped reason=first-start")
		} else {
			var berr error
			backup, berr = r.snapshot(ctx, pool, state, kindPremigration, r.supersededBackup(state, latest))
			if berr != nil {
				backup = ""
				if err := r.handleBackupFailure(berr, state); err != nil {
					return nil, false, err
				}
			}
		}
		if err := r.migrate(ctx, pool, state, latest, backup); err != nil {
			return nil, false, err
		}
	}

	// 12: the line the deploy check parses (ADR-002 D-8).
	final, err := readSchema(ctx, pool)
	if err != nil {
		return nil, false, err
	}
	if !final.has {
		return nil, false, errors.New("migration version: database has no schema version")
	}
	fmt.Fprintf(r.out, "gogear-api migrations: version=%d dirty=%t\n", final.version, final.dirty)
	return pool, false, nil
}

// handleBackupFailure applies Policy.OnBackupFailure. It returns nil when the
// migration may go on without a backup.
func (r *runner) handleBackupFailure(cause error, state schemaState) error {
	reason := oneLine(cause)
	if r.cfg.Policy.OnBackupFailure == MigrateWithoutBackup {
		r.say(levelWarn, "WARNING backup failed (reason=%s); migrating without a backup", reason)
		return nil
	}
	r.say(levelError, "FAILED reason=%s; no migration applied, database unchanged at schema=%s", reason, state.label())
	return fmt.Errorf("pre-migration backup failed: %w", cause)
}

// migrate runs golang-migrate (ARS-002 section 3 step 11). backup is the
// verified backup of this attempt, or empty if none was made.
func (r *runner) migrate(ctx context.Context, pool *sql.DB, from schemaState, latest uint, backup string) error {
	driver, err := sqlite3.WithInstance(pool, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("migration driver init: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+r.cfg.MigrationsPath, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("migration instance init: %w", err)
	}

	upErr := m.Up()
	if upErr == nil || errors.Is(upErr, migrate.ErrNoChange) {
		if err := os.Remove(r.statePath(migrationFailedFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
			r.say(levelWarn, "could not remove %s reason=%v", migrationFailedFile, err)
		}
		r.log.Infoln("Database migrations applied successfully")
		return nil
	}

	failure := fmt.Errorf("migration up: %w", upErr)
	shown := "none"
	if backup != "" {
		shown = backup
	}
	if err := r.recordMigrationFailure(from, latest, backup); err != nil {
		failure = errors.Join(failure, fmt.Errorf("record %s: %w", migrationFailedFile, err))
	}
	r.say(levelError, "migration failed from=%s target=%d; the state before the attempt is in file=%s", from.label(), latest, shown)
	if r.cfg.Policy.OnMigrationFailure == Restore && backup != "" {
		_, _ = m.Close() // closes the pool: the files are replaced next
		if err := r.restoreAfterFailedMigration(ctx, backup); err != nil {
			r.say(levelError, "restore FAILED reason=swap; the database stays as the failed migration left it")
			failure = errors.Join(failure, err)
		}
	}
	return failure
}

// cleanPartials deletes backups that were never finished.
func (r *runner) cleanPartials() {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if partialPattern.MatchString(e.Name()) && e.Type().IsRegular() {
			_ = os.Remove(filepath.Join(r.dir, e.Name()))
		}
	}
}

func (r *runner) listKept() {
	files, err := listBackups(r.dir)
	if err != nil {
		r.say(levelWarn, "could not list %s (reason=%v)", r.dir, err)
		return
	}
	for _, f := range files {
		m := backupPattern.FindStringSubmatch(f.name)
		r.say(levelInfo, "kept file=%s kind=%s schema=%s app=%s", f.path, f.kind, f.schema, m[4])
	}
}

func (r *runner) newestPremigrationName() string {
	var prev migrationFailed
	if found, err := r.readState(migrationFailedFile, &prev); err == nil && found && prev.Backup != "" {
		return prev.Backup
	}
	files, _ := listBackups(r.dir)
	for _, f := range files {
		if f.kind == kindPremigration {
			return f.name
		}
	}
	return "<no backup found>"
}

func requestDone(done restoreDone, value string) bool {
	for _, req := range done.Requests {
		if req.Value == value {
			return true
		}
	}
	return false
}

func hasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// latestVersion is the highest migration version in the image.
func latestVersion(path string) (uint, error) {
	src, err := source.Open("file://" + path)
	if err != nil {
		return 0, fmt.Errorf("open migrations: %w", err)
	}
	defer src.Close()
	v, err := src.First()
	if err != nil {
		return 0, fmt.Errorf("read migrations: %w", err)
	}
	for {
		next, err := src.Next(v)
		if errors.Is(err, os.ErrNotExist) {
			return v, nil
		}
		if err != nil {
			return 0, fmt.Errorf("read migrations after %s: %w", strconv.FormatUint(uint64(v), 10), err)
		}
		v = next
	}
}

// oneLine reduces an error to one line for the announcement lines.
func oneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
