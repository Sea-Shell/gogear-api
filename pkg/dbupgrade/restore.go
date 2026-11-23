package dbupgrade

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/utils"
)

const (
	migrationFailedFile = "migration-failed.json"
	restoreDoneFile     = "restore-done.json"
	restoreDoneKeep     = 20
)

// migrationFailed is migration-failed.json: which backup holds the state from
// before a failed attempt (BR-038 AC-10).
type migrationFailed struct {
	AppVersion    string `json:"app_version"`
	FromVersion   string `json:"from_version"`
	TargetVersion string `json:"target_version"`
	Backup        string `json:"backup"`
	At            string `json:"at"`
}

// restoreDone is restore-done.json: the requests already performed, so that a
// pod restart does not restore again.
type restoreDone struct {
	Requests []restoreRequest `json:"requests"`
}

type restoreRequest struct {
	Value  string `json:"value"`
	Backup string `json:"backup"`
	At     string `json:"at"`
}

func (r *runner) statePath(name string) string { return filepath.Join(r.dir, name) }

func (r *runner) readState(name string, into any) (found bool, err error) {
	data, err := os.ReadFile(r.statePath(name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return true, nil
}

// writeState writes a state file through a temporary file and a rename.
func (r *runner) writeState(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	tmp := r.statePath(name) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.statePath(name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (r *runner) timestamp() string { return r.now().UTC().Format(time.RFC3339) }

// restoreRequested performs a restore asked for through GOGEAR_RESTORE_BACKUP
// (ARS-002 section 4). existed says whether the database has content now.
func (r *runner) restoreRequested(ctx context.Context, value string, existed bool) error {
	fail := func(reason string, err error) error {
		r.say(levelError, "restore FAILED reason=%s; database unchanged", reason)
		return fmt.Errorf("restore %q: %s: %w", value, reason, err)
	}

	name, _, _ := strings.Cut(value, "#")
	m := backupPattern.FindStringSubmatch(name)
	if m == nil {
		return fail("invalid-name", errors.New("not the name of a backup file"))
	}
	path := filepath.Join(r.dir, name)
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return fail("not-found", errors.New("no such backup in the backup directory"))
	}
	want, err := stateFromLabel(m[3])
	if err != nil {
		return fail("invalid-name", err)
	}
	if err := verifyBackup(ctx, path, want); err != nil {
		return fail("verify", err)
	}

	replaced := "none"
	if existed {
		pool, err := sql.Open("sqlite3", utils.SQLiteDSN(r.cfg.DatabaseFile))
		if err != nil {
			return fail("prerestore-backup", err)
		}
		src, err := readSchema(ctx, pool)
		if err == nil {
			replaced, err = r.snapshot(ctx, pool, src, kindPrerestore, "")
		}
		if err == nil {
			_, err = pool.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
		}
		pool.Close()
		if err != nil {
			return fail("prerestore-backup", err)
		}
	}

	if err := r.swapIn(path); err != nil {
		return fail("swap", err)
	}

	var done restoreDone
	if _, err := r.readState(restoreDoneFile, &done); err != nil {
		return fmt.Errorf("record restore: %w", err)
	}
	done.Requests = append(done.Requests, restoreRequest{Value: value, Backup: name, At: r.timestamp()})
	if len(done.Requests) > restoreDoneKeep {
		done.Requests = done.Requests[len(done.Requests)-restoreDoneKeep:]
	}
	if err := r.writeState(restoreDoneFile, done); err != nil {
		return fmt.Errorf("record restore: %w", err)
	}
	r.say(levelInfo, "restored file=%s schema=%s app=%s; replaced database kept as file=%s", path, m[3], m[4], replaced)
	return nil
}

// restoreAfterFailedMigration puts the backup made for this attempt back. The
// caller has closed its pool. No pre-restore backup is made: the failed
// attempt changed only golang-migrate's dirty flag, because each SQLite
// migration runs in one transaction (ADR-006 D-7).
func (r *runner) restoreAfterFailedMigration(ctx context.Context, backup string) error {
	m := backupPattern.FindStringSubmatch(filepath.Base(backup))
	if m == nil {
		return fmt.Errorf("restore after failed migration: %q is not a backup name", backup)
	}
	want, err := stateFromLabel(m[3])
	if err != nil {
		return err
	}
	if err := verifyBackup(ctx, backup, want); err != nil {
		return err
	}
	if err := r.checkpointAndClose(ctx); err != nil {
		return err
	}
	if err := r.swapIn(backup); err != nil {
		return err
	}
	r.say(levelInfo, "restored after failed migration file=%s schema=%s", backup, m[3])
	return nil
}

// checkpointAndClose opens the database once more and folds the write-ahead
// log into the main file, so the files replaced next hold no commit.
func (r *runner) checkpointAndClose(ctx context.Context) error {
	if info, err := os.Stat(r.dbFile); err != nil || info.Size() == 0 {
		return nil
	}
	pool, err := sql.Open("sqlite3", utils.SQLiteDSN(r.cfg.DatabaseFile))
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// swapIn copies the backup next to the database file and renames it over the
// database, after removing the database's -wal and -shm files. A crash leaves
// either the old file or the new one, never a mix (ARS-002 section 4 step 5).
func (r *runner) swapIn(backup string) (err error) {
	tmp := r.dbFile + ".restore-partial"
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	src, err := os.Open(backup)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	for _, side := range []string{"-wal", "-shm"} {
		if err := os.Remove(r.dbFile + side); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(tmp, r.dbFile); err != nil {
		return err
	}
	return syncDir(filepath.Dir(r.dbFile))
}

// recordMigrationFailure writes migration-failed.json.
func (r *runner) recordMigrationFailure(from schemaState, latest uint, backup string) error {
	return r.writeState(migrationFailedFile, migrationFailed{
		AppVersion:    r.cfg.AppVersion,
		FromVersion:   from.label(),
		TargetVersion: strconv.FormatUint(uint64(latest), 10),
		Backup:        filepath.Base(backup),
		At:            r.timestamp(),
	})
}

// supersededBackup returns the backup an earlier failed attempt of this same
// upgrade left, when the new backup is to replace it (ADR-006 D-8). Only the
// Restore policy replaces: under Keep the database is dirty and never gets here.
func (r *runner) supersededBackup(from schemaState, latest uint) string {
	if r.cfg.Policy.OnMigrationFailure != Restore {
		return ""
	}
	var prev migrationFailed
	found, err := r.readState(migrationFailedFile, &prev)
	if err != nil || !found {
		return ""
	}
	if prev.AppVersion != r.cfg.AppVersion || prev.FromVersion != from.label() || prev.TargetVersion != strconv.FormatUint(uint64(latest), 10) {
		return ""
	}
	m := backupPattern.FindStringSubmatch(prev.Backup)
	if m == nil || m[2] != kindPremigration || prev.Backup != filepath.Base(prev.Backup) {
		return ""
	}
	path := filepath.Join(r.dir, prev.Backup)
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}
