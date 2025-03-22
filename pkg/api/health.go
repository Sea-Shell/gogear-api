package endpoints

import (
	"net/http"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
)

//	@Summary		Get application health
//	@Description	Get health status of application
//	@Tags			Health
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.Health	"desc"
//	@Router			/health [get]
func ReturnHealth(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, models.Health{
		Status:        "ok",
		Name:          "GoGear-api",
		Updated:       time.Now().Format("02.01.2006 15:04:05"),
		Documentation: "https://github.com/Sea-Shell/gogear-api",
	})
}
