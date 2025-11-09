// Some comment
package main

import (
	"database/sql"
	"flag"
	"log"
	"net/http"
	"strings"

	endpoints "github.com/Sea-Shell/gogear-api/pkg/api"
	docs "github.com/Sea-Shell/gogear-api/pkg/docs"
	models "github.com/Sea-Shell/gogear-api/pkg/models"
	utils "github.com/Sea-Shell/gogear-api/pkg/utils"

	gin "github.com/gin-gonic/gin"
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

var GoogleConf *oauth2.Config

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

func LogRequestsMiddleware(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Log the request details
		logger.Infow("Received request",
			"method", c.Request.Method,
			"status", c.Writer.Status(),
			"url", c.FullPath(),
			"url-params", c.Request.URL.Query(),
			"Authorization", c.Request.Header.Get("Authorization"),
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

// @title									GoGear API
// @version								1.0
// @description							This is the API of GoGear
// @contact.name							API Support
// @contact.email							support@sea-shell.no
// @license.name							Apache 2.0
// @license.url							http://www.apache.org/licenses/LICENSE-2.0.html
//
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description					Include a server-issued JWT as `Bearer <token>`. Endpoints may require either the client or admin audience.
func main() {
	configFile := flag.String("config", configFile, "Config file")

	flag.Parse()

	config, err := utils.LoadConfig[models.Config](*configFile)
	if err != nil {
		log.Fatalf("Failed to load config file: %v", err)
	}

	logLevel := utils.GetLogLevel(config.General.LogLevel)
	log := makeLogger(logLevel)
	defer log.Sync()

	log.Debugf("%#v", config)

	db, err := sql.Open("sqlite3", config.Database.File)
	if err != nil {
		log.Error(err)
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
	GoogleConf = &oauth2.Config{
		ClientID:     config.Auth.GoogleClientID,
		ClientSecret: config.Auth.GoogleClientSecret,
		RedirectURL:  config.Auth.GoogleRedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     googleOAuth.Endpoint,
	}

	if strings.TrimSpace(config.Auth.JWTSecret) == "" {
		log.Warn("JWT secret is empty; authentication will fail until configured")
	}
	if GoogleConf.ClientID == "" || GoogleConf.RedirectURL == "" {
		log.Warnw("Google OAuth configuration incomplete", "client_id_set", GoogleConf.ClientID != "", "redirect_url", GoogleConf.RedirectURL)
	}

	router.Use(configMiddleware(config))
	router.Use(oauthConfigMiddleware(GoogleConf))

	// API v1
	swagger := router.Group("/swagger")
	v1 := router.Group("/api/v1")
	v1.Use(utils.JWTMiddleware())
	// v1.Use(utils.CheckApiKey())

	// API Groups
	userGroup := v1.Group("/users")
	gearGroup := v1.Group("/gear")
	topCategoryGroup := v1.Group("/topCategory")
	categoryGroup := v1.Group("/category")
	manufactureGroup := v1.Group("/manufacture")
	userGearGroup := v1.Group("/usergear")
	containerGroup := v1.Group("/container")

	// The routes
	router.GET("/health", endpoints.ReturnHealth)

	authGroup := router.Group("/auth")
	authGroup.GET("/google/callback", endpoints.GoogleAuthCallback)
	authGroup.POST("/google/callback", endpoints.GoogleAuthCallback)
	authGroup.POST("/refresh", utils.JWTMiddleware(), endpoints.RefreshToken)

	// User endpoints
	userGroup.GET("/list", endpoints.ListUser)
	userGroup.GET("/:user/get", endpoints.GetUser)
	userGroup.POST("/:user/update", endpoints.UpdateUser)
	userGroup.DELETE("/:user/delete", endpoints.DeleteUser)
	userGroup.PUT("/insert", endpoints.InsertUser)
	// userGroup.POST("/setpassword", endpoints.SetUserPassword)

	// Gear endpoints
	gearGroup.GET("/list", endpoints.ListGear)
	gearGroup.GET("/search", utils.JWTMiddleware(), endpoints.SearchGear)
	gearGroup.GET("/:gear/get", utils.JWTMiddleware(), endpoints.GetGear)
	gearGroup.POST("/:gear/update", endpoints.UpdateGear)
	gearGroup.DELETE("/:gear/delete", endpoints.DeleteGear)
	gearGroup.PUT("/insert", endpoints.InsertGear)

	// User Gear endpoints
	userGearGroup.GET("/:user/list", endpoints.ListUserGear)
	userGearGroup.GET("/registration/:usergear/get", endpoints.GetUserGear)
	userGearGroup.POST("/registration/:usergear/update", endpoints.UpdateUserGear)
	userGearGroup.DELETE("/registration/:usergear/delete", endpoints.DeleteUserGearRegistration)
	userGearGroup.PUT("/insert", endpoints.InsertUserGear)

	// Container endpoints
	containerGroup.GET("/:container/list", endpoints.ListUserGearInContainer)
	containerGroup.PUT("/insert", endpoints.InsertContainer)
	containerGroup.DELETE("/:container/delete", endpoints.DeleteContainerRegistration)

	// Top Category endpoints
	topCategoryGroup.GET("/list", endpoints.ListTopCategory)
	topCategoryGroup.GET("/:topCategory/get", endpoints.GetTopCategory)
	topCategoryGroup.POST("/:topCategory/update", endpoints.UpdateTopCategory)
	topCategoryGroup.DELETE("/:topCategory/delete", endpoints.DeleteTopCategory)
	topCategoryGroup.PUT("/insert", endpoints.InsertTopCategory)

	// Category endpoints
	categoryGroup.GET("/list", endpoints.ListCategory)
	categoryGroup.GET("/:category/get", endpoints.GetCategory)
	categoryGroup.POST("/:category/update", endpoints.UpdateCategory)
	categoryGroup.DELETE("/:category/delete", endpoints.DeleteCategory)
	categoryGroup.PUT("/insert", endpoints.InsertCategory)

	// Manufacture endpoints
	manufactureGroup.GET("/list", endpoints.ListManufacture)
	manufactureGroup.GET("/:manufacture/get", endpoints.GetManufacture)
	manufactureGroup.POST("/:manufacture/update", endpoints.UpdateManufacture)
	manufactureGroup.DELETE("/:manufacture/delete", endpoints.DeleteManufature)
	manufactureGroup.PUT("/insert", endpoints.InsertManufacture)

	// Swagger API documentation
	swagger.GET("/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Listen to all addresses and port defined
	if err := router.Run("0.0.0.0:" + config.General.ListenPort); err != nil {
		log.Fatal(err)
	}
}
