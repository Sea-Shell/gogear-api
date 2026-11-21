// Some comment
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

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

// LogRequestsMiddleware is a Gin middleware that logs incoming HTTP requests using the provided logger.
func LogRequestsMiddleware(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Log the request details
		logger.Infow("Received request",
			"method", c.Request.Method,
			"status", c.Writer.Status(),
			"url", c.FullPath(),
			"url-params", c.Request.URL.Query(),
		)

		// Continue processing the request
		c.Set("logger", logger)
		c.Next()
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

	config, err := utils.LoadConfig[models.Config](*configFile)
	if err != nil {
		log.Fatalf("Failed to load config file: %v", err)
	}

	if err := utils.ValidateAuthAudiences(&config.Auth); err != nil {
		log.Fatalf("Invalid auth configuration: %v", err)
	}

	logLevel := utils.GetLogLevel(config.General.LogLevel)
	log := makeLogger(logLevel)
	defer log.Sync()

	log.Debugf("%#v", config)

	db, err := sql.Open("sqlite3", config.Database.File)
	if err != nil {
		log.Error(err)
	}

	if err := runMigrations(db, "migrations", log); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
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

	router := gin.New()
	router.Use(gin.Recovery())
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

	router.Use(LogRequestsMiddleware(log))
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
		log.Warnw("Google OAuth configuration incomplete", "client_id_set", googleConf.ClientID != "", "redirect_url", googleConf.RedirectURL)
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

	// Listen to all addresses and port defined
	if err := router.Run("0.0.0.0:" + config.General.ListenPort); err != nil {
		log.Fatal(err)
	}
}
