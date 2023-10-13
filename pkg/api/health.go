package endpoints

import (
	"net/http"
	"time"

	"github.com/SeaShell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
)

func ReturnHealth(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, models.Health{
		Status:        "ok",
		Name:          "GoGear-api",
		Updated:       time.Now().Format("02.01.2006 15:04:05"),
		Documentation: "https://github.com/SeaShell/gogear-api",
	})
}
