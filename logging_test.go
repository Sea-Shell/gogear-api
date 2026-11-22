package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	zap "go.uber.org/zap"
	zapcore "go.uber.org/zap/zapcore"
	observer "go.uber.org/zap/zaptest/observer"
)

// SEC-11: request logging must report the final status and latency, use the
// route template, and never record query values (OAuth codes, tokens).

func newObservedRouter() (*gin.Engine, *observer.ObservedLogs) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.DebugLevel)
	r := gin.New()
	r.Use(LogRequestsMiddleware(zap.New(core).Sugar()))
	r.GET("/api/v1/gear/:gear/get", func(c *gin.Context) { c.Status(http.StatusForbidden) })
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	return r, logs
}

func TestRequestLog_FinalStatusLatencyRouteAndNoQueryValues(t *testing.T) {
	r, logs := newObservedRouter()

	secrets := []string{"code=AUTHCODE-1", "id_token=IDTOKEN-2", "credential=CRED-3", "state=STATE-4",
		"token=TOK-5", "access_token=ACCESS-6", "refresh_token=REFRESH-7", "page=PAGE-8", "email=bob@example.com"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gear/42/get?"+strings.Join(secrets, "&"), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want exactly one request log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()

	if got := fields["status"]; got != int64(http.StatusForbidden) {
		t.Errorf("logged status = %v, want %d (the status after the handler ran)", got, http.StatusForbidden)
	}
	if _, ok := fields["latency_ms"]; !ok {
		t.Errorf("no latency logged: %v", fields)
	}
	if got := fields["route"]; got != "/api/v1/gear/:gear/get" {
		t.Errorf("route = %v, want the route template", got)
	}

	dump, _ := json.Marshal(fields)
	all := entries[0].Message + string(dump)
	for _, leak := range []string{"AUTHCODE-1", "IDTOKEN-2", "CRED-3", "STATE-4", "TOK-5", "ACCESS-6", "REFRESH-7", "PAGE-8", "bob@example.com", "/42/"} {
		if strings.Contains(all, leak) {
			t.Errorf("request log leaks %q: %s", leak, all)
		}
	}
}

func TestRequestLog_ServerErrorIsLoggedAtErrorLevel(t *testing.T) {
	r, logs := newObservedRouter()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))
	e := logs.All()
	if len(e) != 1 || e[0].Level != zapcore.ErrorLevel || e[0].ContextMap()["status"] != int64(500) {
		t.Fatalf("5xx must log status 500 at error level, got %+v", e)
	}
}

func TestRequestLog_UnmatchedRouteDoesNotLogRawPath(t *testing.T) {
	r, logs := newObservedRouter()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope/SECRET-PATH?code=X", nil))
	e := logs.All()
	if len(e) != 1 {
		t.Fatalf("want one entry, got %d", len(e))
	}
	dump, _ := json.Marshal(e[0].ContextMap())
	if strings.Contains(string(dump), "SECRET-PATH") || strings.Contains(string(dump), "code") {
		t.Errorf("unmatched request log leaks raw path/query: %s", dump)
	}
	if e[0].ContextMap()["status"] != int64(404) {
		t.Errorf("status = %v, want 404", e[0].ContextMap()["status"])
	}
}

func TestConfigSummaryHasNoSecrets(t *testing.T) {
	cfg := &models.Config{
		Database: models.Database{File: "gear.db", Password: "DB-PASS-SECRET"},
		General:  models.General{Hostname: "api.example.com", ListenPort: "8080", LogLevel: "debug"},
		Auth: models.Auth{
			JWTSecret:          "JWT-SECRET-VALUE",
			GoogleClientID:     "client-id.apps.googleusercontent.com",
			GoogleClientSecret: "GOOGLE-SECRET-VALUE",
			GoogleRedirectURL:  "https://api.example.com/auth/google/callback?x=REDIRECT-SECRET",
		},
	}
	// The old call logged the whole struct: prove the fixture would have leaked.
	if raw := fmt.Sprintf("%#v", cfg); !strings.Contains(raw, "JWT-SECRET-VALUE") {
		t.Fatal("fixture invalid: the old Go-syntax log would expose the secret")
	}

	summary := fmt.Sprint(configSummary(cfg)...)
	for _, leak := range []string{"JWT-SECRET-VALUE", "GOOGLE-SECRET-VALUE", "DB-PASS-SECRET", "REDIRECT-SECRET"} {
		if strings.Contains(summary, leak) {
			t.Errorf("config summary leaks %q: %s", leak, summary)
		}
	}
	for _, want := range []string{"8080", "api.example.com", "true"} {
		if !strings.Contains(summary, want) {
			t.Errorf("config summary lacks %q: %s", want, summary)
		}
	}
}

// wiredRouter builds the real router through newRouter, as main does, against
// a migrated database, with logs captured.
func wiredRouter(t *testing.T) (*gin.Engine, *observer.ObservedLogs, *models.Config) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := tempDB(t)
	runMigrate(t, db, migrationsPath(t))
	core, logs := observer.New(zapcore.DebugLevel)
	cfg := &models.Config{
		General: models.General{Hostname: "api.example.com", ListenPort: "8080", LogLevel: "debug"},
		Auth: models.Auth{
			JWTSecret: "JWT-SECRET-VALUE", JWTAudience: "client", JWTAdminAudience: "admin",
			GoogleClientID: "cid", GoogleClientSecret: "GOOGLE-SECRET-VALUE",
			GoogleRedirectURL: "https://api.example.com/cb",
		},
	}
	return newRouter(cfg, db, zap.New(core).Sugar()), logs, cfg
}

func dumpLogs(logs *observer.ObservedLogs) string {
	var sb strings.Builder
	for _, e := range logs.All() {
		dump, _ := json.Marshal(e.ContextMap())
		sb.WriteString(e.Message + " " + string(dump) + "\n")
	}
	return sb.String()
}

// SEC-11 through the real wiring: an OAuth callback code and the startup
// config line must not reach the logs.
func TestNewRouter_LogsCarryNoSecrets(t *testing.T) {
	r, logs, _ := wiredRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=SECRET-OAUTH-CODE&state=SECRET-STATE", nil))

	out := dumpLogs(logs)
	if !strings.Contains(out, "Handled request") || !strings.Contains(out, "Loaded configuration") {
		t.Fatalf("expected request and startup config log lines, got:\n%s", out)
	}
	for _, leak := range []string{"SECRET-OAUTH-CODE", "SECRET-STATE", "JWT-SECRET-VALUE", "GOOGLE-SECRET-VALUE"} {
		if strings.Contains(out, leak) {
			t.Errorf("logs leak %q:\n%s", leak, out)
		}
	}
}

// A handler panic is a logged 500 and must not dump the query to stderr.
func TestNewRouter_PanicIsLoggedAs500WithoutQueryDump(t *testing.T) {
	r, logs, _ := wiredRouter(t)
	r.GET("/panic", func(c *gin.Context) { panic("kaboom") })

	// Capture what gin's default recovery would write.
	var stderr strings.Builder
	prev := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &stderr
	defer func() { gin.DefaultErrorWriter = prev }()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic?code=SECRET-PANIC-CODE", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	var handled, panicked bool
	for _, e := range logs.All() {
		switch e.Message {
		case "Handled request":
			handled = e.ContextMap()["status"] == int64(500) && e.Level == zapcore.ErrorLevel
		case "Panic recovered":
			panicked = true
		}
	}
	if !handled || !panicked {
		t.Fatalf("want a logged 500 request and a panic entry, got:\n%s", dumpLogs(logs))
	}
	if strings.Contains(stderr.String(), "SECRET-PANIC-CODE") || strings.Contains(dumpLogs(logs), "SECRET-PANIC-CODE") {
		t.Errorf("panic handling leaks the query:\nstderr=%s\nlogs=%s", stderr.String(), dumpLogs(logs))
	}
}

// Startup goes through loadValidatedConfig, which must refuse a bad config.
func TestLoadValidatedConfig_RefusesInvalidConfig(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("auth:\n  jwt-audience: same\n  jwt-admin-audience: same\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadValidatedConfig(file); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("error = %v, want audience validation failure", err)
	}
}
