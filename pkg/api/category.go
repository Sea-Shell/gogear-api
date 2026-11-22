package endpoints

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	models "github.com/Sea-Shell/gogear-api/pkg/models"
	utils "github.com/Sea-Shell/gogear-api/pkg/utils"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// categoryCapabilityJoin attaches the container settings of a sub-category (view
// category_container_setting, migration 000009, ADR-001 D-3) to a query on
// gear_category. The trailing space lets GenericGet append its WHERE clause.
const categoryCapabilityJoin = ` LEFT JOIN category_container_setting ON category_container_setting.categoryId = gear_category.categoryId `

var errInvalidCategoryContainer = errors.New(`category_container must be "container", "not_container" or "inherit"`)

// parseCategoryContainer reads the category_container field of a request body
// (ADR-001 D-2). present is false when the field is absent or JSON null: not
// sent. Otherwise value is the column value: nil for "inherit" (removes the
// explicit setting), 1 for "container", 0 for "not_container". Any other JSON
// value is errInvalidCategoryContainer.
func parseCategoryContainer(raw json.RawMessage) (value any, present bool, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return nil, false, errInvalidCategoryContainer
	}
	switch s {
	case "container":
		return int64(1), true, nil
	case "not_container":
		return int64(0), true, nil
	case "inherit":
		return nil, true, nil
	}
	return nil, false, errInvalidCategoryContainer
}

// @Summary		Get category with ID
// @Description	Get category spessific to ID. category_container is the explicit container setting (container, not_container, or inherit when the sub-category has none); category_is_container is the effective setting (BR-021).
// @Security		BearerAuth
// @Tags			Category
// @Accept			json
// @Produce		json
// @Param			category	path		int					true	"Unique ID of category you want to get"
// @Success		200			{object}	models.GearCategory	"desc"
// @Failure		default		{object}	models.Error
// @Router			/api/v1/category/{category}/get [get]
func GetCategory(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	function := "category"

	urlParameter, err := strconv.Atoi(c.Param(function))
	if err != nil {
		log.Errorf("urlParamter is of wrong type: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	results, err := utils.GenericGet[models.GearCategory]("gear_category", urlParameter, []string{categoryCapabilityJoin}, db)
	if err != nil {
		log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
	c.IndentedJSON(http.StatusOK, results)
}

// @Summary		List categories
// @Description	Get a list of category items, each with its explicit (category_container) and effective (category_is_container) container setting and its top category's (top_category_is_container).
// @Security		BearerAuth
// @Tags			Category
// @Accept			json
// @Produce		json
// @Param			page		query		int		false	"Page number"				default(1)
// @Param			limit		query		int		false	"Number of items per page"	default(30)
// @Param			category	query		[]int	false	"Top category"				collectionFormat(multi)
// @Param			topCategory	query		[]int	false	"Top gear category"			collectionFormat(multi)
// @Success		200			{object}	models.ResponsePayload{items=[]models.GearCategoryListItem}
// @Failure		default		{object}	models.Error
// @Router			/api/v1/category/list [get]
func ListCategory(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	topCategories := c.QueryArray("topCategory")
	categories := c.QueryArray("category")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	log.Debugf("Request parameters: %#v", c.Request.URL.Query())

	if limit == "" {
		limit = "30"
	}

	if page == "" || page == "0" {
		page = "1"
	}

	pageInt, err := strconv.Atoi(page)
	if err != nil {
		log.Errorf("Error setting page to int: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		log.Errorf("Error setting limit to int: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if pageInt <= 0 {
		log.Errorf("Error page is less than 0: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid page number"})
		return
	}

	if limitInt <= 0 {
		log.Errorf("Error limit is less than 0: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid limit number"})
		return
	}

	conditions := []string{}
	args := []interface{}{}

	for _, topCategory := range topCategories {
		conditions = append(conditions, "gear_category.categoryTopCategoryId = ?")
		args = append(args, topCategory)
	}

	for _, category := range categories {
		conditions = append(conditions, "gear_category.categoryId = ?")
		args = append(args, category)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " OR ")
	}

	baseCountQuery := "SELECT COUNT(*) FROM gear_category"
	countQuery := baseCountQuery + whereClause

	var totalCount int
	err = db.QueryRow(countQuery, args...).Scan(&totalCount)
	if err != nil {
		log.Errorf("Error getting GearCount database: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	start := strconv.Itoa((pageInt - 1) * limitInt)
	totalPages := int(math.Ceil(float64(totalCount) / float64(limitInt)))

	startInt, err := strconv.Atoi(start)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var paramCategory models.GearCategoryListItem
	fields := utils.GetDBFieldNames(reflect.TypeOf(paramCategory))

	baseQuery := fmt.Sprintf(`SELECT %s FROM gear_category
    LEFT JOIN gear_top_category ON gear_category.categoryTopCategoryId = gear_top_category.topCategoryId
    LEFT JOIN category_container_setting ON category_container_setting.categoryId = gear_category.categoryId`,
		strings.Join(fields, ", "))

	queryLimit := fmt.Sprintf(" LIMIT %v, %v", startInt, limitInt)

	query := baseQuery + whereClause + queryLimit

	log.Debugf("Query: %s", query)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Errorf("Query error: %#v", err.Error())
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	defer rows.Close()

	dest, err := utils.GetScanFields(paramCategory)
	if err != nil {
		log.Errorf("Error getting destination arguments: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var gearCategoryList []models.GearCategoryListItem

	for rows.Next() {
		err = rows.Scan(dest...)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Errorf("No gear found")
				c.IndentedJSON(http.StatusNotFound, models.Error{Error: "No results"})
				return
			}
			log.Errorf("Scan error: %#v", err)
			c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
			return
		}

		for i := 0; i < reflect.TypeOf(paramCategory).NumField(); i++ {
			reflect.ValueOf(&paramCategory).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		gearCategoryList = append(gearCategoryList, paramCategory)
	}

	if err := rows.Err(); err != nil {
		log.Errorf("Row iteration error: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	payload := models.ResponsePayload{
		TotalItemCount: totalCount,
		CurrentPage:    pageInt,
		ItemLimit:      limitInt,
		TotalPages:     totalPages,
		Items:          gearCategoryList,
	}

	if pageInt < totalPages {
		currentQueryParameters.Set("page", strconv.Itoa(pageInt+1))
		nextPage := url.URL{
			Path:     c.Request.URL.Path,
			RawQuery: currentQueryParameters.Encode(),
		}
		payload.NextPage = new(string)
		*payload.NextPage = nextPage.String()
	}

	if pageInt > 1 {
		currentQueryParameters.Set("page", strconv.Itoa(pageInt-1))
		prevPage := url.URL{
			Path:     c.Request.URL.Path,
			RawQuery: currentQueryParameters.Encode(),
		}
		payload.PrevPage = new(string)
		*payload.PrevPage = prevPage.String()
	}

	c.IndentedJSON(http.StatusOK, payload)
}

// @Summary		Update category with ID
// @Description	Update category identified by ID. Requires a JWT issued with the admin audience. category_container sets the container setting: "container" or "not_container" sets it, "inherit" removes it so the top category's setting applies; absent or null leaves it unchanged; any other value is 400. category_is_container is read-only and ignored.
// @Security		BearerAuth
// @Tags			Category
// @Accept			json
// @Produce		json
// @Param			categoryID	path		int					true	"Unique ID of category you want to update"
// @Param			request		body		models.GearCategoryInput	true	"Request body"
// @Success		200			{object}	models.Status		"status: success when all goes well"
// @Failure		default		{object}	models.Error
// @Failure		400		{object}	models.Error
// @Failure		403		{object}	models.Error	"admin privileges required"
// @Router			/api/v1/category/{category}/update [post]
func UpdateCategory(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	id, ok := catalogID(c, "category")
	if !ok {
		return
	}
	var body struct {
		ID            *int64          `json:"category_id"`
		TopCategoryID *int64          `json:"category_top_category_id"`
		Name          *string         `json:"category_name"`
		Container     json.RawMessage `json:"category_container"`
	}
	if !readCatalogBody(c, &body) {
		return
	}
	container, hasContainer, err := parseCategoryContainer(body.Container)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	u := catalogUpdate{table: "gear_category", idColumn: "categoryId"}
	if hasContainer {
		u.add("categoryIsContainer", container)
	}
	if body.TopCategoryID != nil {
		u.add("categoryTopCategoryId", *body.TopCategoryID)
	}
	if body.Name != nil {
		u.add("categoryName", *body.Name)
	}
	u.apply(c, id, body.ID)
}

// @Summary		Insert new category
// @Description	Insert new category with corresponding values. Requires a JWT issued with the admin audience. Without category_container (or with null or "inherit") the new sub-category has no explicit container setting and follows its top category.
// @Security		BearerAuth
// @Tags			Category
// @Accept			json
// @Produce		json
// @Param			request	body		models.GearCategoryInput	true	"Request body"
// @Success		200		{object}	models.GearCategory	"the created category with its container settings"
// @Failure		default	{object}	models.Error
// @Failure		400		{object}	models.Error
// @Failure		403		{object}	models.Error	"admin privileges required"
// @Router			/api/v1/category/insert [put]
func InsertCategory(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	var body struct {
		TopCategoryID int64           `json:"category_top_category_id"`
		Name          string          `json:"category_name"`
		Container     json.RawMessage `json:"category_container"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid JSON body: " + err.Error()})
		return
	}
	container, _, err := parseCategoryContainer(body.Container)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	res, err := db.Exec(`INSERT INTO gear_category (categoryTopCategoryId, categoryName, categoryIsContainer) VALUES (?, ?, ?)`,
		body.TopCategoryID, body.Name, container)
	if err != nil {
		catalogInsertFailed(c, log, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		catalogInsertFailed(c, log, err)
		return
	}
	createdObject, err := utils.GenericGet[models.GearCategory]("gear_category", int(id), []string{categoryCapabilityJoin}, db)
	if err != nil {
		catalogInsertFailed(c, log, err)
		return
	}

	c.JSON(http.StatusOK, createdObject)
}

// @Summary		Delete category with ID
// @Description	Delete category with corresponding ID value. Requires a JWT issued with the admin audience.
// @Security		BearerAuth
// @Tags			Category
// @Accept			json
// @Produce		json
// @Param			category	path		int				true	"Unique ID of category you want to update"
// @Success		200			{object}	models.Status	"status: success when all goes well"
// @Failure		default		{object}	models.Error
// @Failure		409		{object}	models.Error
// @Router			/api/v1/category/{category}/delete [delete]
func DeleteCategory(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	deleteCatalogRow(c, "gear_category", "categoryId", "categoryName", "category", "Category")
}
