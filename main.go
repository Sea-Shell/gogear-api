// Some comment
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	endpoints "github.com/Sea-Shell/gogear-api/pkg/api"
	docs "github.com/Sea-Shell/gogear-api/pkg/docs"
	models "github.com/Sea-Shell/gogear-api/pkg/models"
	utils "github.com/Sea-Shell/gogear-api/pkg/utils"

	gin "github.com/gin-gonic/gin"
	migrate "github.com/golang-migrate/migrate/v4"
	sqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	zap "go.uber.org/zap"
	zapcore "go.uber.org/zap/zapcore"
	oauth2 "golang.org/x/oauth2"
	googleOAuth "golang.org/x/oauth2/google"
)

const (
	configFile = "config.yaml"
)

var googleConf *oauth2.Config

func makeLogger(loglevel zapcore.Level) *zap.SugaredLogger {
	customCallerEncoder := func(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(caller.TrimmedPath())
	}
	cfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(loglevel),
		Encoding:    "json",
		OutputPaths: []string{"stdout"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:      "time",
			EncodeLevel:  zapcore.LowercaseLevelEncoder,
			EncodeTime:   zapcore.ISO8601TimeEncoder,
			LevelKey:     "level",
			MessageKey:   "message",
			CallerKey:    "caller",
			EncodeCaller: customCallerEncoder,
		},
	}

	return zap.Must(cfg.Build()).Sugar()
}

// LogRequestsMiddleware is a Gin middleware that logs one line per request
// after the handler has run, so the status is the one the client received. It
// logs the route template (not the raw path, which carries IDs and, on
// unmatched routes, arbitrary client text) and only the number of query
// parameters: query values carry OAuth codes, ID tokens and similar secrets
// (SEC-11), so neither names nor values are recorded.
func LogRequestsMiddleware(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Set("logger", logger)

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		fields := []any{
			"method", c.Request.Method,
			"route", route,
			"status", status,
			"latency_ms", float64(time.Since(start).Microseconds()) / 1000,
			"query_params", len(c.Request.URL.Query()),
		}
		if status >= http.StatusInternalServerError {
			logger.Errorw("Handled request", fields...)
			return
		}
		logger.Infow("Handled request", fields...)
	}
}

// configSummary describes the loaded configuration for debug logging without
// secrets: secret values appear only as set/unset flags.
func configSummary(cfg *models.Config) []any {
	return []any{
		"hostname", cfg.General.Hostname,
		"schemes", cfg.General.Schemes,
		"listen_port", cfg.General.ListenPort,
		"log_level", cfg.General.LogLevel,
		"database_file", cfg.Database.File,
		"jwt_issuer", cfg.Auth.JWTIssuer,
		"jwt_secret_set", strings.TrimSpace(cfg.Auth.JWTSecret) != "",
		"jwt_audience_set", cfg.Auth.JWTAudience != "",
		"jwt_admin_audience_set", cfg.Auth.JWTAdminAudience != "",
		"jwt_expiry_minutes", cfg.Auth.JWTExpiryMinutes,
		"google_client_id_set", cfg.Auth.GoogleClientID != "",
		"google_client_secret_set", cfg.Auth.GoogleClientSecret != "",
		"google_redirect_url_set", cfg.Auth.GoogleRedirectURL != "",
	}
}

func databaseMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	}
}

func configMiddleware(config *models.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("config", config)
		c.Set("conf", &config.General)
		c.Set("auth", &config.Auth)
		c.Next()
	}
}

func oauthConfigMiddleware(oauth *oauth2.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("oauth", oauth)
		c.Next()
	}
}

// validateConfig checks the loaded configuration before the service starts.
// main refuses to start when it fails (BR-006 AC-4).
func validateConfig(config *models.Config) error {
	if err := utils.ValidateAuthAudiences(&config.Auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	// idtoken.Validate skips the audience check for an empty client ID, which
	// would accept tokens issued for any Google client.
	if strings.TrimSpace(config.Auth.GoogleClientID) == "" {
		return errors.New("auth: google-client-id must not be empty")
	}
	// An empty path would make the SQLite driver open a file named after its
	// options in the working directory (see utils.SQLiteDSN).
	if strings.TrimSpace(config.Database.File) == "" {
		return errors.New("database: file must not be empty")
	}
	return nil
}

// loadValidatedConfig loads the config file and refuses a configuration that
// fails validateConfig. main starts only from its result.
func loadValidatedConfig(file string) (*models.Config, error) {
	config, err := utils.LoadConfig[models.Config](file)
	if err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return config, nil
}

// runMigrations applies pending golang-migrate migrations from the given directory.
// Uses the already-open *sql.DB so the same SQLite connection is used.
func runMigrations(db *sql.DB, migrationsPath string, log *zap.SugaredLogger) error {
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("migration driver init: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"sqlite3",
		driver,
	)
	if err != nil {
		return fmt.Errorf("migration instance init: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up: %w", err)
	}

	log.Infoln("Database migrations applied successfully")
	return nil
}

// reportForeignKeyViolations logs one warning per (table, parent) with orphan
// rows. It is a startup audit of data written while enforcement was off: it
// never repairs or deletes, and a database with orphans still starts, because
// enforcement only blocks new invalid writes. It returns the number of
// orphan rows found.
func reportForeignKeyViolations(ctx context.Context, db *sql.DB, log *zap.SugaredLogger) (int, error) {
	violations, err := utils.ForeignKeyViolations(ctx, db)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, v := range violations {
		total += v.Count
		log.Warnw("Database contains rows that reference a missing parent row; they are left unchanged",
			"table", v.Table, "parent_table", v.Parent, "orphan_rows", v.Count)
	}
	return total, nil
}

// warnIfForeignKeysOff logs a warning when foreign-key enforcement is off on
// the opened database. SQLiteDSN honours _fk=0 / _foreign_keys=off from the
// configured file name, and with enforcement off the referential guarantees
// the service relies on silently stop holding. It returns whether enforcement
// is on.
func warnIfForeignKeysOff(ctx context.Context, db *sql.DB, log *zap.SugaredLogger) bool {
	var on int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&on); err != nil {
		log.Warnf("Could not read PRAGMA foreign_keys: %v", err)
		return false
	}
	if on == 0 {
		log.Warn("Foreign-key enforcement is OFF (PRAGMA foreign_keys = 0); check the database.file options for _fk=0 or _foreign_keys=off. Rows can now reference missing parents")
		return false
	}
	return true
}

// recoveryMiddleware turns a handler panic into a logged 500. It replaces
// gin.Recovery, which writes the request line, query string included, to
// stderr: query values carry OAuth codes and tokens (SEC-11).
func recoveryMiddleware(logger *zap.SugaredLogger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		logger.Errorw("Panic recovered", "panic", fmt.Sprint(err), "stack", string(debug.Stack()))
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

// newRouter builds the service router: middleware, routes and swagger. main
// serves it; tests exercise it directly so wiring regressions are caught.
func newRouter(config *models.Config, db *sql.DB, log *zap.SugaredLogger) *gin.Engine {
	log.Debugw("Loaded configuration", configSummary(config)...)

	router := gin.New()
	// Logging comes first so a request that panics is still logged, with the
	// 500 that recovery wrote.
	router.Use(LogRequestsMiddleware(log))
	router.Use(recoveryMiddleware(log))
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
		c.Header("Access-Control-Expose-Headers", "Authorization, Content-Type")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
	})

	router.Use(databaseMiddleware(db))
	googleConf = &oauth2.Config{
		ClientID:     config.Auth.GoogleClientID,
		ClientSecret: config.Auth.GoogleClientSecret,
		RedirectURL:  config.Auth.GoogleRedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     googleOAuth.Endpoint,
	}

	if strings.TrimSpace(config.Auth.JWTSecret) == "" {
		log.Warn("JWT secret is empty; authentication will fail until configured")
	}
	if googleConf.ClientID == "" || googleConf.RedirectURL == "" {
		log.Warnw("Google OAuth configuration incomplete", "client_id_set", googleConf.ClientID != "", "redirect_url_set", googleConf.RedirectURL != "")
	}

	router.Use(configMiddleware(config))
	router.Use(oauthConfigMiddleware(googleConf))

	// API v1
	swagger := router.Group("/swagger")
	v1 := router.Group("/api/v1")
	v1.Use(utils.JWTMiddleware())
	// v1.Use(utils.CheckApiKey())

	// API Groups
	userGroup := v1.Group("/users")
	userGearGroup := v1.Group("/usergear")
	containerGroup := v1.Group("/container")

	// The routes
	router.GET("/health", endpoints.ReturnHealth)

	authGroup := router.Group("/auth")
	authGroup.GET("/google/callback", endpoints.GoogleAuthCallback)
	authGroup.POST("/google/callback", endpoints.GoogleAuthCallback)
	authGroup.POST("/refresh", utils.JWTMiddleware(), endpoints.RefreshToken)

	// Public loadout routes (no JWT required)
	endpoints.RegisterPublicLoadoutRoutes(router.Group("/api/v1/public"))

	// User endpoints
	endpoints.RegisterUserRoutes(userGroup)

	// Catalog endpoints (gear, top category, category, manufacture): reads for
	// every authenticated user, writes admin-only
	endpoints.RegisterCatalogRoutes(v1)

	// User Gear endpoints
	endpoints.RegisterUserGearRoutes(userGearGroup)

	// Container endpoints
	endpoints.RegisterContainerRoutes(containerGroup)

	// Loadout and loadout item endpoints (protected)
	endpoints.RegisterLoadoutRoutes(v1.Group("/loadout"))

	// Swagger API documentation
	swagger.GET("/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return router
}

// @title						GoGear API
// @version					1.0
// @description				This is the API of GoGear
// @contact.name				API Support
// @contact.email				support@sea-shell.no
// @license.name				Apache 2.0
// @license.url				http://www.apache.org/licenses/LICENSE-2.0.html
//
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Include a server-issued JWT as `Bearer <token>`. Endpoints may require either the client or admin audience.
func main() {
	configFile := flag.String("config", configFile, "Config file")

	flag.Parse()

	config, err := loadValidatedConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	logLevel := utils.GetLogLevel(config.General.LogLevel)
	log := makeLogger(logLevel)
	defer log.Sync()

	db, err := sql.Open("sqlite3", utils.SQLiteDSN(config.Database.File))
	if err != nil {
		log.Error(err)
	}

	warnIfForeignKeysOff(context.Background(), db, log)

	if err := runMigrations(db, "migrations", log); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}

	if _, err := reportForeignKeyViolations(context.Background(), db, log); err != nil {
		log.Warnf("Could not audit foreign keys: %v", err)
	}

	// Legacy bootstrap: ensure columns that migrations do not manage.
	// Keep for backward compatibility with databases created before golang-migrate.
	if err := utils.EnsureColumn(db, "user_gear_registrations", "maxContainerWeight", "INTEGER"); err != nil {
		log.Fatalf("Failed to ensure user_gear_registrations.maxContainerWeight exists: %v", err)
	}

	log.Infoln("Connected to database")
	defer db.Close()

	docs.SwaggerInfo.Title = "GoGear API"
	docs.SwaggerInfo.Description = "This is the API of GoGear."
	docs.SwaggerInfo.Host = config.General.Hostname
	docs.SwaggerInfo.Schemes = config.General.Schemes
	docs.SwaggerInfo.BasePath = "/"

	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := newRouter(config, db, log)

	// Listen to all addresses and port defined
	if err := router.Run("0.0.0.0:" + config.General.ListenPort); err != nil {
		log.Fatal(err)
	}
}
