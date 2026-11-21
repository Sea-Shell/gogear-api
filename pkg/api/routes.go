package endpoints

import "github.com/gin-gonic/gin"

// RegisterUserRoutes wires the user endpoints onto the given group (normally
// /api/v1/users, behind the JWT middleware). main and the authorization test
// harness share this function so tests exercise the real route table.
func RegisterUserRoutes(g *gin.RouterGroup) {
	g.GET("/list", ListUser)
	g.GET("/:user/get", GetUser)
	g.POST("/:user/update", UpdateUser)
	g.DELETE("/:user/delete", DeleteUser)
	g.POST("/:user/admin", SetUserAdmin)
	g.PUT("/insert", InsertUser)
}
