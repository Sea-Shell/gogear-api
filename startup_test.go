package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	migrate "github.com/golang-migrate/migrate/v4"
	"go.uber.org/zap"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// startupConfig returns a configuration whose database is a new file at
// schema version 10 with data, so the first start applies every later migration.
func startupConfig(t *testing.T) *models.Config {
	t.Helper()
	t.Setenv(restoreEnv, "")
	file := filepath.Join(t.TempDir(), "gogear.db")
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		t.Fatal(err)
	}
	stepMigrate(t, db, migrationsPath(t), func(m *migrate.Migrate) error { return m.Migrate(10) })
	db.Close()
	cfg := &models.Config{}
	cfg.Database.File = file
	cfg.General.ListenPort = "0"
	return cfg
}

func schemaVersion(t *testing.T, file string) int {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+file+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	return scalar[int](t, db, `SELECT version FROM schema_migrations`)
}

// ARS-002 section 3 invariant: the listener is reached only after the startup
// routine has backed up and migrated. The stand-in for router.Run looks at the
// database and the backup directory at the moment it is called.
func TestListenerIsReachedOnlyAfterBackupAndMigration(t *testing.T) {
	cfg := startupConfig(t)
	called := false
	listen := func(router *gin.Engine) error {
		called = true
		if router == nil {
			t.Error("listen received no router")
		}
		if v, want := schemaVersion(t, cfg.Database.File), latestMigration(t); v != want {
			t.Errorf("schema = %d when the listener opened, want %d", v, want)
		}
		backups, _ := filepath.Glob(filepath.Join(filepath.Dir(cfg.Database.File), "backups", "gogear-*-premigration-schema10-app"+appVersion+".db"))
		if len(backups) != 1 {
			t.Errorf("backups = %v when the listener opened, want the verified schema-10 backup", backups)
		}
		return nil
	}
	if err := serve(context.Background(), cfg, zap.NewNop().Sugar(), listen); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if !called {
		t.Fatal("the listener was never reached")
	}
}

// A refused backup never reaches the listener.
func TestListenerIsNotReachedWhenTheBackupIsRefused(t *testing.T) {
	cfg := startupConfig(t)
	// A regular file where the backup directory must go.
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfg.Database.File), "backups"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	listen := func(*gin.Engine) error {
		t.Error("the listener was reached although the backup failed")
		return nil
	}
	if err := serve(context.Background(), cfg, zap.NewNop().Sugar(), listen); err == nil {
		t.Fatal("serve must fail")
	}
	if v := schemaVersion(t, cfg.Database.File); v != 10 {
		t.Errorf("schema = %d, want 10", v)
	}
}

// Restore mode blocks until it is told to stop and never opens the listener.
func TestRestoreModeHoldsWithoutListening(t *testing.T) {
	cfg := startupConfig(t)
	log := zap.NewNop().Sugar()
	listen := func(*gin.Engine) error { return nil }
	if err := serve(context.Background(), cfg, log, listen); err != nil {
		t.Fatalf("first start: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(cfg.Database.File), "backups", "gogear-*schema10*.db"))
	if len(matches) != 1 {
		t.Fatalf("schema-10 backup = %v", matches)
	}
	t.Setenv(restoreEnv, filepath.Base(matches[0]))

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, cfg, log, func(*gin.Engine) error {
			t.Error("the listener was opened in restore mode")
			return nil
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("serve returned (%v) instead of holding", err)
	case <-time.After(500 * time.Millisecond):
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve after stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after the stop signal")
	}
	if v := schemaVersion(t, cfg.Database.File); v != 10 {
		t.Errorf("schema = %d, want the restored 10 without a migration", v)
	}
}
