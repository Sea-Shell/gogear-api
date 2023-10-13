package main

import (
	"database/sql"
	"flag"
	"log"

	endpoints "github.com/SeaShell/gogear-api/pkg/api"
	models "github.com/SeaShell/gogear-api/pkg/models"
	utils "github.com/SeaShell/gogear-api/pkg/utils"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	zap "go.uber.org/zap"
	zapcore "go.uber.org/zap/zapcore"

	"github.com/SeaShell/gogear-api/docs"
)

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

func setLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
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

const (
	listenPort = "8081"
	configFile = "config.yaml"
)

// @title           GoGear API
// @version         1.0
// @description     This is the API of GoGear

// @contact.name   API Support
// @contact.email  support@seashell.no

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8081
// @BasePath  /
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

	router := gin.New()
	router.Use(gin.Recovery())

	router.Use(setLogger(log))

	router.Use(databaseMiddleware(db))
	router.Use(configMiddleware(&config.General))

	// Groups
	userGroup := router.Group("/user")
	gearGroup := router.Group("/gear")
	topCategoryGroup := router.Group("/topCategory")
	categoryGroup := router.Group("/category")
	manufactureGroup := router.Group("/manufacture")

	router.GET("/health", endpoints.ReturnHealth)

	router.GET("/userGear/:user", endpoints.GetUserGear)

	// The routes
	userGroup.GET("/:user", endpoints.GetUser)
	userGroup.GET("/list", endpoints.ListUser)
	userGroup.POST("/update", endpoints.UpdateUser)
	userGroup.PUT("/insert", endpoints.InsertUser)
	userGroup.POST("/setpassword", endpoints.SetUserPassword)

	gearGroup.GET("/:gear", endpoints.GetGear)
	gearGroup.GET("/list", endpoints.ListGear)
	gearGroup.PUT("/insert", endpoints.InsertGear)
	gearGroup.POST("/update", endpoints.UpdateGear)

	topCategoryGroup.GET("/:topCategory", endpoints.GetTopCategory)
	topCategoryGroup.GET("/list", endpoints.ListTopCategory)
	topCategoryGroup.POST("/update", endpoints.UpdateTopCategory)
	topCategoryGroup.PUT("/insert", endpoints.InsertTopCategory)

	categoryGroup.GET("/:category", endpoints.GetCategory)
	categoryGroup.GET("/list", endpoints.ListCategory)
	categoryGroup.POST("/update", endpoints.UpdateCategory)
	categoryGroup.PUT("/insert", endpoints.InsertCategory)

	manufactureGroup.GET("/:manufacture", endpoints.GetManufacture)
	manufactureGroup.GET("/list", endpoints.ListManufacture)
	manufactureGroup.PUT("/insert", endpoints.InsertManufacture)
	manufactureGroup.POST("/update", endpoints.UpdateManufacture)

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.Run("0.0.0.0:" + listenPort)
}
