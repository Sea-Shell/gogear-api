package endpoints

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	zap "go.uber.org/zap"
)

// RequireAdmin refuses the request with 403 unless the real JWT middleware
// marked the caller as admin (user_is_admin). It runs before the handler, so a
// handler behind it never sees a non-admin and cannot forget the check.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isAdmin, _ := c.Get("user_is_admin"); isAdmin != true {
			if logger, ok := c.Get("logger"); ok {
				if l, ok := logger.(*zap.SugaredLogger); ok {
					l.Warnw("catalog change refused: admin privileges required", "method", c.Request.Method, "route", c.FullPath())
				}
			}
			c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "admin privileges required"})
			return
		}
		c.Next()
	}
}

// RegisterCatalogRoutes wires the gear, manufacture, top category and category
// endpoints onto the given group (normally /api/v1, behind the JWT middleware).
// Reads are open to every authenticated user; create, update and delete go
// through RequireAdmin (BR-001). main and the authorization tests share this.
func RegisterCatalogRoutes(v1 *gin.RouterGroup) {
	gear := v1.Group("/gear")
	gear.GET("/list", ListGear)
	gear.GET("/search", SearchGear)
	gear.GET("/:gear/get", GetGear)
	gearAdmin := gear.Group("", RequireAdmin())
	gearAdmin.POST("/:gear/update", UpdateGear)
	gearAdmin.DELETE("/:gear/delete", DeleteGear)
	gearAdmin.PUT("/insert", InsertGear)

	top := v1.Group("/topCategory")
	top.GET("/list", ListTopCategory)
	top.GET("/:topCategory/get", GetTopCategory)
	topAdmin := top.Group("", RequireAdmin())
	topAdmin.POST("/:topCategory/update", UpdateTopCategory)
	topAdmin.DELETE("/:topCategory/delete", DeleteTopCategory)
	topAdmin.PUT("/insert", InsertTopCategory)

	cat := v1.Group("/category")
	cat.GET("/list", ListCategory)
	cat.GET("/:category/get", GetCategory)
	catAdmin := cat.Group("", RequireAdmin())
	catAdmin.POST("/:category/update", UpdateCategory)
	catAdmin.DELETE("/:category/delete", DeleteCategory)
	catAdmin.PUT("/insert", InsertCategory)

	man := v1.Group("/manufacture")
	man.GET("/list", ListManufacture)
	man.GET("/:manufacture/get", GetManufacture)
	manAdmin := man.Group("", RequireAdmin())
	manAdmin.POST("/:manufacture/update", UpdateManufacture)
	manAdmin.DELETE("/:manufacture/delete", DeleteManufature)
	manAdmin.PUT("/insert", InsertManufacture)
}

// catalogUpdate is one explicit partial UPDATE of a catalog row. The target is
// always the path ID; the body can never name it (BR-017). Only the columns in
// set are written.
type catalogUpdate struct {
	table, idColumn string
	set             []string // column names
	values          []any
}

func (u *catalogUpdate) add(column string, value any) {
	u.set = append(u.set, column+" = ?")
	u.values = append(u.values, value)
}

// readCatalogBody reads the request body and decodes it into dst. On failure
// it writes the response and returns false.
func readCatalogBody(c *gin.Context, dst any) bool {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Error(err.Error())
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return false
	}
	if err := json.Unmarshal(data, dst); err != nil {
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

// catalogID parses the path parameter as a positive ID, or answers 400.
func catalogID(c *gin.Context, param string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid " + param + " ID"})
		return 0, false
	}
	return id, true
}

// apply runs the update and answers: 404 when no row has the path ID (checked
// first, as for every other resource), then 400 when the body ID conflicts
// with the path or the body names no field, 500 on database errors, else 200.
func (u *catalogUpdate) apply(c *gin.Context, id int64, bodyID *int64) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	var exists bool
	if err := db.QueryRow(fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE %s = ?)", u.table, u.idColumn), id).Scan(&exists); err != nil {
		log.Error(err.Error())
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, models.Error{Error: "not found"})
		return
	}

	// Provisional default P-2 (BR-017 AC-2): a body ID that differs from the
	// path is refused; a matching or absent one has no effect of its own.
	if bodyID != nil && *bodyID != id {
		c.JSON(http.StatusBadRequest, models.Error{Error: "ID in body does not match ID in path"})
		return
	}
	if len(u.set) == 0 {
		c.JSON(http.StatusBadRequest, models.Error{Error: "no updatable fields in request body"})
		return
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", u.table, strings.Join(u.set, ", "), u.idColumn)
	res, err := db.Exec(query, append(u.values, id)...)
	if err != nil {
		log.Error(err.Error())
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		log.Error(err.Error())
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	if n == 0 {
		c.JSON(http.StatusNotFound, models.Error{Error: "not found"})
		return
	}
	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// deleteCatalogRow deletes one row by path ID in a transaction and reports its
// name. It reads only the name, so NULL in other columns cannot break it, and
// answers 404 when no row has the ID.
func deleteCatalogRow(c *gin.Context, table, idColumn, nameColumn, param, label string) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	id, ok := catalogID(c, param)
	if !ok {
		return
	}

	fail := func(err error) {
		log.Error(err.Error())
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
	}

	tx, err := db.Begin()
	if err != nil {
		fail(err)
		return
	}
	defer tx.Rollback()

	var name string
	err = tx.QueryRow(fmt.Sprintf("SELECT %s FROM %s WHERE %s = ?", nameColumn, table, idColumn), id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, models.Error{Error: "not found"})
		return
	}
	if err != nil {
		fail(err)
		return
	}
	res, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", table, idColumn), id)
	if err != nil {
		fail(err)
		return
	}
	if n, err := res.RowsAffected(); err != nil {
		fail(err)
		return
	} else if n == 0 {
		c.JSON(http.StatusNotFound, models.Error{Error: "not found"})
		return
	}
	if err := tx.Commit(); err != nil {
		fail(err)
		return
	}

	msg := fmt.Sprintf("success! %s with ID %d and name %s was deleted", label, id, name)
	log.Info(msg)
	c.JSON(http.StatusOK, map[string]string{"status": msg})
}
