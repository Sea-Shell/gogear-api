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

	models "github.com/SeaShell/gogear-api/pkg/models"
	utils "github.com/SeaShell/gogear-api/pkg/utils"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

//	@Summary		Get manufacture by ID
//	@Description	Get manufacture spessific to ID
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			manufacture	path		int					true	"Unique ID of Gear you want to get"
//	@Success		200			{object}	models.Manufacture	"desc"
//	@Router			/manufacture/{manufacture} [get]
func GetManufacture(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	function := "manufacture"

	manufacture := c.Param(function)

	var param_Manufacturer models.Manufacture
	fields := utils.GetDBFieldNames(reflect.TypeOf(param_Manufacturer))

	baseQuery := fmt.Sprintf("SELECT %s FROM manufacture WHERE manufactureId = ? LIMIT 1", strings.Join(fields, ", "))

	row := db.QueryRow(baseQuery, manufacture)

	dest, err := utils.GetScanFields(param_Manufacturer)
	if err != nil {
		log.Errorf("Error getting destination arguments: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	err = row.Scan(dest...)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Errorf("gear with id: %v not found", manufacture)
			c.IndentedJSON(http.StatusNotFound, map[string]string{"gearId": manufacture, "error": "No results"})
			return
		}
		log.Errorf("Scan error: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	for i := 0; i < reflect.TypeOf(param_Manufacturer).NumField(); i++ {
		reflect.ValueOf(&param_Manufacturer).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
	}

	log.Infof("successfully fetched ManufactureId: %s, ManufactureName: %s", param_Manufacturer.ManufactureId, param_Manufacturer.ManufactureName)
	c.IndentedJSON(http.StatusOK, param_Manufacturer)
}

//	@Summary		List manufacture
//	@Description	Get a list of manufacturers
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			page			query		int		false	"Page number"				default(1)
//	@Param			limit			query		int		false	"Number of items per page"	default(30)
//	@Param			manufacture		query		string	false	"search by manufacturename (this is case insensitive and wildcard)"
//	@Param			manufacturename	query		string	false	"search by manufactures full name (this is case insensitive and wildcard)"
//	@Success		200				{object}	models.ResponsePayload{items=[]models.Manufacture}
//	@Failure		default			{object}	models.Error
//	@Router			/manufacture/list [get]
func ListManufacture(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	manufacturers := c.QueryArray("manufacture")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	log.Debugf("Request parameters: %#v", c.Request.URL.Query())

	if limit == "" {
		limit = "30"
	}
	
	if page == "" || page == "0" {
		page = "1"
	}

	page_int, err := strconv.Atoi(page)
	if err != nil {
		log.Errorf("Error setting page to int: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	limit_int, err := strconv.Atoi(limit)
	if err != nil {
		log.Errorf("Error setting limit to int: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if page_int <= 0 {
		log.Errorf("Error page is less than 0: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid page number"})
		return
	}

	if limit_int <= 0 {
		log.Errorf("Error limit is less than 0: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid limit number"})
		return
	}

	conditions := []string{}

	for _, manufacture := range manufacturers {
		topcat := fmt.Sprintf("manufacture.manufactureId = %s", manufacture)
		conditions = append(conditions, topcat)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " OR ")
	}

	baseCountQuery := "SELECT COUNT(*) FROM manufacture"
	countQuery := baseCountQuery + whereClause

	var totalCount int
	err = db.QueryRow(countQuery).Scan(&totalCount)
	if err != nil {
		log.Errorf("Error getting GearCount database: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	start := strconv.Itoa((page_int-1)*limit_int)
	totalPages := int(math.Ceil(float64(totalCount) / float64(limit_int)))

	start_int, err := strconv.Atoi(start)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var param_Manufacturer models.Manufacture
	fields := utils.GetDBFieldNames(reflect.TypeOf(param_Manufacturer))

	baseQuery := fmt.Sprintf(`SELECT %s FROM manufacture`, strings.Join(fields, ", "))

	queryLimit := fmt.Sprintf(" LIMIT %v, %v", start_int, limit_int)

	query := baseQuery + whereClause + queryLimit

	log.Debugf("Query: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		log.Errorf("Query error: %#v", err.Error())
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	dest, err := utils.GetScanFields(param_Manufacturer)
	if err != nil {
		log.Errorf("Error getting destination arguments: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var manufactureList []models.Manufacture

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

		for i := 0; i < reflect.TypeOf(param_Manufacturer).NumField(); i++ {
			reflect.ValueOf(&param_Manufacturer).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		manufactureList = append(manufactureList, param_Manufacturer)
	}

	payload := models.ResponsePayload{
		TotalItemCount: totalCount,
		CurrentPage:    page_int,
		ItemLimit:      limit_int,
		TotalPages:     totalPages,
		Items:          manufactureList,
	}

	if page_int < totalPages {
		currentQueryParameters.Set("page", strconv.Itoa(page_int+1))
		nextPage := url.URL{
			Path:     c.Request.URL.Path,
			RawQuery: currentQueryParameters.Encode(),
		}
		payload.NextPage = new(string)
		*payload.NextPage = nextPage.String()
	}

	if page_int > 1 {
		currentQueryParameters.Set("page", strconv.Itoa(page_int-1))
		prevPage := url.URL{
			Path:     c.Request.URL.Path,
			RawQuery: currentQueryParameters.Encode(),
		}
		payload.PrevPage = new(string) 
		*payload.PrevPage = prevPage.String()
	}

	c.IndentedJSON(http.StatusOK, payload)
}

//	@Summary		Update manufacture with ID
//	@Description	Update manufacture identified by ID
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.Manufacture	true	"query params"	test
//	@Success		200		{object}	models.Status		"status: success when all goes well"
//	@Router			/manufacture/update [post]
func UpdateManufacture(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericUpdate[models.Manufacture]("manufacture", data, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
	}

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

//	@Summary		Insert new manufacture
//	@Description	Insert new manufacture with corresponding values
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.Manufacture	true	"query params"	test
//	@Success		200		{object}	models.Status		"status: success when all goes well"
//	@Router			/manufacture/insert [put]
func InsertManufacture(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	err = utils.GenericInsert[models.Manufacture]("manufacture", data, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}