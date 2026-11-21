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

// RegisterLoadoutRoutes wires the authenticated loadout and loadout-item
// endpoints onto the given group (normally /api/v1/loadout, behind the JWT
// middleware).
func RegisterLoadoutRoutes(g *gin.RouterGroup) {
	g.PUT("/insert", InsertLoadout)
	g.GET("/list", ListLoadouts)
	g.GET("/:loadout/get", GetLoadout)
	g.POST("/:loadout/update", UpdateLoadout)
	g.DELETE("/:loadout/delete", DeleteLoadout)
	g.POST("/:loadout/import", ImportLoadout)

	g.PUT("/:loadout/item/insert", InsertLoadoutItem)
	g.GET("/:loadout/item/list", ListLoadoutItems)
	g.POST("/:loadout/item/:item/update", UpdateLoadoutItem)
	g.DELETE("/:loadout/item/:item/delete", DeleteLoadoutItem)
}

// RegisterPublicLoadoutRoutes wires the unauthenticated, read-only public
// loadout endpoints onto the given group (normally /api/v1/public, without the
// JWT middleware).
func RegisterPublicLoadoutRoutes(g *gin.RouterGroup) {
	g.GET("/loadout/:slug", GetPublicLoadout)
	g.GET("/loadout/:slug/items", GetPublicLoadoutItems)
}
