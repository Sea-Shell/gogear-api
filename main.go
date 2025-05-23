// Some comment
package main

import (
    "database/sql"
    "flag"
    "log"

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
)

const (
    listenPort     = "8081"
    configFile     = "config.yaml"
    googleCredFile = "google-creds.json"
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
            "X-API-Key", c.Request.Header.Get("X-API-Key"),
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

func configMiddleware(config *models.General) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Set("conf", config)
        c.Next()
    }
}

//	@title									GoGear API
//	@version								1.0
//	@description							This is the API of GoGear
//	@contact.name							API Support
//	@contact.email							support@sea-shell.no
//	@license.name							Apache 2.0
//	@license.url							http://www.apache.org/licenses/LICENSE-2.0.html
//
//	@securityDefinitions.apikey				APIKey
//	@in										header
//	@name									X-API-Key
//
//	@securitydefinitions.oauth2.password	OAuth2Application
//	@description							OAuth protects our entity endpoints
//	@tokenUrl								https://oauth2.googleapis.com/token
//	@authorizationurl						https://accounts.google.com/o/oauth2/auth
//	@scope.write							Grants read and write access
//	@scope.admin							Grants read and write access to administrative information
//	@scope.read								Grants read access
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
    })

    router.Use(LogRequestsMiddleware(log))
    router.Use(databaseMiddleware(db))
    router.Use(configMiddleware(&config.General))

    // API v1
    swagger := router.Group("/swagger")
    v1 := router.Group("/api/v1")
    // v1.Use(utils.CheckApiKey())

    // API Groups
    userGroup := v1.Group("/users")
    gearGroup := v1.Group("/gear")
    topCategoryGroup := v1.Group("/topCategory")
    categoryGroup := v1.Group("/category")
    manufactureGroup := v1.Group("/manufacture")
    userGearGroup := v1.Group("/usergear")

    // The routes
    router.GET("/health", endpoints.ReturnHealth)

    // User endpoints
    userGroup.GET("/list", endpoints.ListUser)
    userGroup.GET("/:user/get", endpoints.GetUser)
    userGroup.POST("/:user/update", endpoints.UpdateUser)
    userGroup.DELETE("/:user/delete", endpoints.DeleteUser)
    userGroup.PUT("/insert", endpoints.InsertUser)
    // userGroup.POST("/setpassword", endpoints.SetUserPassword)

    // Gear endpoints
    gearGroup.GET("/list", endpoints.ListGear)
    gearGroup.GET("/:gear/get", endpoints.GetGear)
    gearGroup.POST("/:gear/update", endpoints.UpdateGear)
    gearGroup.DELETE("/:gear/delete", endpoints.DeleteGear)
    gearGroup.PUT("/insert", endpoints.InsertGear)

    // User Gear endpoints
    userGearGroup.GET("/:user/list", endpoints.ListUserGear)
    userGearGroup.GET("/registration/:usergear/get", endpoints.GetUserGear)
    userGearGroup.POST("/registration/:usergear/update", endpoints.UpdateUserGear)
    userGearGroup.DELETE("/registration/:usergear/delete", endpoints.DeleteUserGearRegistration)
    userGearGroup.PUT("/insert", endpoints.InsertUserGear)

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
