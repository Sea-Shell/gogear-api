package endpoints

import (
	"database/sql"
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

// gearCapabilityJoin attaches the derived container status (view
// gear_container_capability, migration 000009, ADR-001 D-3) to a query on gear.
// Container status is read only through this view, in the reported value, the
// container filter and its count, so they cannot disagree (BR-021 AC-3).
const gearCapabilityJoin = ` LEFT JOIN gear_container_capability ON gear_container_capability.gearId = gear.gearId`

// ListGear lists gear in the database
//
//	@Summary		List gear
//	@Description	Get a list of gear items
//	@Security		BearerAuth
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			page			query		int			false	"Page number"				default(1)
//	@Param			limit			query		int			false	"Number of items per page"	default(30)
//	@Param			category		query		string		false	"Gear category"
//	@Param			topCategory		query		string		false	"Top gear category"
//	@Param			manufacturer	query		string		false	"Gear manufacturer"
//	@Param			collection		query		[]string	false	"string collection"	collectionFormat(multi)
//	@Param			container		query		bool		false	"string collection"	default(false)
//	@Success		200				{object}	models.ResponsePayload{items=[]models.GearListItem}
//	@Failure		default			{object}	models.Error
//	@Router			/api/v1/gear/list [get]
func ListGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	category := c.Query("category")
	topCategory := c.Query("topCategory")
	manufacturer := c.Query("manufacturer")
	container, hasContainer := c.GetQuery("container")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	//cache := c.MustGet("cache").(*cache.BigCache)
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
	queryParams := []interface{}{}
	if topCategory != "" {
		conditions = append(conditions, "gear.gearTopCategoryId = ?")
		queryParams = append(queryParams, topCategory)
	}
	if category != "" {
		conditions = append(conditions, "gear.gearCategoryId = ?")
		queryParams = append(queryParams, category)
	}
	if manufacturer != "" {
		conditions = append(conditions, "gear.GearManufactureId = ?")
		queryParams = append(queryParams, manufacturer)
	}
	if hasContainer {
		containerBool, err := strconv.ParseBool(container)
		if err != nil {
			c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "container must be true or false"})
			return
		}
		if containerBool {
			conditions = append(conditions, "gear_container_capability.isContainer = 1")
		} else {
			conditions = append(conditions, "gear_container_capability.isContainer = 0")
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	baseCountQuery := "SELECT COUNT(*) FROM gear" + gearCapabilityJoin
	countQuery := baseCountQuery + whereClause

	var totalCount int
	err = db.QueryRow(countQuery, queryParams...).Scan(&totalCount)
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

	var paramGear models.GearListItem
	fields := utils.GetDBFieldNames(reflect.TypeOf(paramGear))

	baseQuery := fmt.Sprintf(`SELECT %s FROM gear
        LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId
        LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId
        LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId`+gearCapabilityJoin,
		strings.Join(fields, ", "))

	queryLimit := fmt.Sprintf(" LIMIT %v, %v", startInt, limitInt)

	query := baseQuery + whereClause + queryLimit

	log.Debugf("Query: %s", query)

	rows, err := db.Query(query, queryParams...)
	if err != nil {
		log.Errorf("Query error: %#v", err.Error())
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	defer rows.Close()

	dest, err := utils.GetScanFields(paramGear)
	if err != nil {
		log.Errorf("Error getting destination arguments: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var gearList []models.GearListItem

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

		for i := 0; i < reflect.TypeOf(paramGear).NumField(); i++ {
			reflect.ValueOf(&paramGear).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		gearList = append(gearList, paramGear)
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
		Items:          gearList,
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

	log.Infof("successfully fetched gear with id: %s, gearName: %s", paramGear.GearID, paramGear.GearName)
	c.IndentedJSON(http.StatusOK, payload)
}

// SearchGear is a function to search for gear items
//
//	@Summary		Search for gear
//	@Description	Get a list of gear items based on search patterns
//	@Security		BearerAuth
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			page			query		int		false	"Page number"				default(1)
//	@Param			limit			query		int		false	"Number of items per page"	default(30)
//	@Param			searchString	query		string	true	"String to search for"
//	@Param			searchType		query		string	true	"Type of search method. valid choices are: startswith, contains, endswith"
//	@Success		200				{object}	models.ResponsePayload{items=[]models.GearListItem}
//	@Failure		default			{object}	models.Error
//	@Router			/api/v1/gear/search [get]
func SearchGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	searchString, searchStringPressent := c.GetQuery("searchString")
	searchType, searchTypePressent := c.GetQuery("searchType")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	//cache := c.MustGet("cache").(*cache.BigCache)
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
	queryArgs := []interface{}{}

	if searchStringPressent && strings.TrimSpace(searchString) != "" {
		normalizedSearchType := strings.TrimSpace(strings.ToLower(searchType))
		if !searchTypePressent || normalizedSearchType == "" {
			normalizedSearchType = "contains"
		}

		var likePattern string
		switch normalizedSearchType {
		case "contains":
			likePattern = "%" + searchString + "%"
		case "startswith":
			likePattern = searchString + "%"
		case "endswith":
			likePattern = "%" + searchString
		default:
			c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid searchType. Use contains, startswith, or endswith."})
			return
		}

		conditions = append(conditions, "gear.gearName LIKE ?")
		queryArgs = append(queryArgs, likePattern)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " OR ")
	}

	baseCountQuery := "SELECT COUNT(*) FROM gear"
	countQuery := baseCountQuery + whereClause

	var totalCount int
	err = db.QueryRow(countQuery, queryArgs...).Scan(&totalCount)
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

	var paramGear models.GearListItem
	fields := utils.GetDBFieldNames(reflect.TypeOf(paramGear))

	baseQuery := fmt.Sprintf(`SELECT %s FROM gear
        LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId
        LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId
        LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId`+gearCapabilityJoin,
		strings.Join(fields, ", "))

	queryLimit := fmt.Sprintf(" LIMIT %v, %v", startInt, limitInt)

	query := baseQuery + whereClause + queryLimit

	log.Debugf("Query: %s", query)

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		log.Errorf("Query error: %#v", err.Error())
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	defer rows.Close()

	dest, err := utils.GetScanFields(paramGear)
	if err != nil {
		log.Errorf("Error getting destination arguments: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var gearList []models.GearListItem

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

		for i := 0; i < reflect.TypeOf(paramGear).NumField(); i++ {
			reflect.ValueOf(&paramGear).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		gearList = append(gearList, paramGear)
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
		Items:          gearList,
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

	log.Infof("successfully fetched gear with id: %s, gearName: %s", paramGear.GearID, paramGear.GearName)
	c.IndentedJSON(http.StatusOK, payload)
}

// GetGear gets spessific gear based on ID
//
//	@Summary		Get gear with ID
//	@Description	Get gear spessific to ID
//	@Security		BearerAuth
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			gear	path		int				true	"Unique ID of Gear you want to get"
//	@Success		200		{object}	models.FullGear	"desc"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/gear/{gear}/get [get]
func GetGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	function := "gear"

	urlParameter, err := strconv.Atoi(c.Param(function))
	if err != nil {
		log.Errorf("urlParamter is of wrong type: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
	}

	var extraSQL []string
	extraSQL = append(extraSQL, " LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId ")
	extraSQL = append(extraSQL, " LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId ")
	extraSQL = append(extraSQL, "  LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")
	extraSQL = append(extraSQL, gearCapabilityJoin+" ")

	results, err := utils.GenericGet[models.FullGear]("gear", urlParameter, extraSQL, db)
	if err != nil {
		log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
	c.IndentedJSON(http.StatusOK, results)
}

// InsertGear insert new gear according to spessification
//
//	@Summary		Insert new gear
//	@Description	Insert new gear with corresponding values. gear_is_container is read-only: it is ignored when sent, and the response reports the status derived from the gear's sub-category.
//	@Security		BearerAuth
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.GearInput	true	"query params"	test
//	@Success		200		{object}	models.Gear			"the created gear"
//	@Failure		default	{object}	models.Error
//	@Failure		400		{object}	models.Error
//	@Router			/api/v1/gear/insert [put]
func InsertGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	inserted, err := utils.GenericInsert[models.GearInput]("gear", data, db)
	if err != nil {
		catalogInsertFailed(c, log, err)
		return
	}
	// The response carries the derived container status, so it is read back
	// through the same view as every other gear response.
	createdObject, err := utils.GenericGet[models.Gear]("gear", int(*inserted.GearID), []string{gearCapabilityJoin + " "}, db)
	if err != nil {
		catalogInsertFailed(c, log, err)
		return
	}

	c.JSON(http.StatusOK, createdObject)
}

// UpdateGear updates existing gear
//
//	@Summary		Update gear with ID
//	@Description	Update gear identified by ID
//	@Security		BearerAuth
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			gear	path		int				true	"Unique ID of Gear you want to get"
//	@Param			request	body		models.GearInput	true	"query params"	test
//	@Success		200		{object}	models.Status		"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Failure		400		{object}	models.Error
//	@Router			/api/v1/gear/{gear}/update [post]
func UpdateGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	id, ok := catalogID(c, "gear")
	if !ok {
		return
	}
	var body struct {
		ID             *int64  `json:"gear_id"`
		TopCategoryID  *int64  `json:"gear_top_category_id"`
		CategoryID     *int64  `json:"gear_category_id"`
		ManufactureID  *int64  `json:"gear_manufacture_id"`
		Name           *string `json:"gear_name"`
		SizeDefinition *string `json:"gear_size_definition"`
		Weight         *int32  `json:"gear_weight"`
		Height         *int32  `json:"gear_height"`
		Length         *int32  `json:"gear_length"`
		Width          *int32  `json:"gear_width"`
		Status         *bool   `json:"gear_status"`
	}
	if !readCatalogBody(c, &body) {
		return
	}

	u := catalogUpdate{table: "gear", idColumn: "gearId"}
	for _, f := range []struct {
		column string
		value  any
		set    bool
	}{
		{"gearTopCategoryId", body.TopCategoryID, body.TopCategoryID != nil},
		{"gearCategoryId", body.CategoryID, body.CategoryID != nil},
		{"gearManufactureId", body.ManufactureID, body.ManufactureID != nil},
		{"gearName", body.Name, body.Name != nil},
		{"gearSizeDefinition", body.SizeDefinition, body.SizeDefinition != nil},
		{"gearWeight", body.Weight, body.Weight != nil},
		{"gearHeight", body.Height, body.Height != nil},
		{"gearLength", body.Length, body.Length != nil},
		{"gearWidth", body.Width, body.Width != nil},
		{"gearStatus", body.Status, body.Status != nil},
	} {
		if f.set {
			u.add(f.column, f.value)
		}
	}
	u.apply(c, id, body.ID)
}

// DeleteGear delets gear based on ID
//
//	@Summary		Delete gear with ID
//	@Description	Delete gear with corresponding ID value
//	@Security		BearerAuth
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			gear	path		int				true	"Unique ID of gear you want to delete"
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Failure		409		{object}	models.Error
//	@Router			/api/v1/gear/{gear}/delete [delete]
func DeleteGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	deleteCatalogRow(c, "gear", "gearId", "gearName", "gear", "Gear")
}
