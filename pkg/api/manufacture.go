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

//	@Summary		Get manufacture by ID
//	@Description	Get manufacture spessific to ID
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			manufacture	path		int					true	"Unique ID of manufacture you want to get"
//	@Success		200			{object}	models.Manufacture	"desc"
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/manufacture/{manufacture}/get [get]
func GetManufacture(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "manufacture"

    manufacture := c.Param(function)

    var paramManufacturer models.Manufacture
    fields := utils.GetDBFieldNames(reflect.TypeOf(paramManufacturer))

    baseQuery := fmt.Sprintf("SELECT %s FROM manufacture WHERE manufactureID = ? LIMIT 1", strings.Join(fields, ", "))

    row := db.QueryRow(baseQuery, manufacture)

    dest, err := utils.GetScanFields(paramManufacturer)
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

    for i := 0; i < reflect.TypeOf(paramManufacturer).NumField(); i++ {
        reflect.ValueOf(&paramManufacturer).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
    }

    log.Infof("successfully fetched ManufactureID: %s, ManufactureName: %s", paramManufacturer.ManufactureID, paramManufacturer.ManufactureName)
    c.IndentedJSON(http.StatusOK, paramManufacturer)
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
//	@Router			/api/v1/manufacture/list [get]
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

    start := strconv.Itoa((pageInt - 1) * limitInt)
    totalPages := int(math.Ceil(float64(totalCount) / float64(limitInt)))

    startInt, err := strconv.Atoi(start)
    if err != nil {
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    var paramManufacturer models.Manufacture
    fields := utils.GetDBFieldNames(reflect.TypeOf(paramManufacturer))

    baseQuery := fmt.Sprintf(`SELECT %s FROM manufacture`, strings.Join(fields, ", "))

    queryLimit := fmt.Sprintf(" LIMIT %v, %v", startInt, limitInt)

    query := baseQuery + whereClause + queryLimit

    log.Debugf("Query: %s", query)

    rows, err := db.Query(query)
    if err != nil {
        log.Errorf("Query error: %#v", err.Error())
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    dest, err := utils.GetScanFields(paramManufacturer)
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

        for i := 0; i < reflect.TypeOf(paramManufacturer).NumField(); i++ {
            reflect.ValueOf(&paramManufacturer).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
        }

        manufactureList = append(manufactureList, paramManufacturer)
    }

    payload := models.ResponsePayload{
        TotalItemCount: totalCount,
        CurrentPage:    pageInt,
        ItemLimit:      limitInt,
        TotalPages:     totalPages,
        Items:          manufactureList,
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

//	@Summary		Update manufacture with ID
//	@Description	Update manufacture identified by ID
//	@Security		OAuth2Application[write]
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			manufacture	path		int					true	"Unique ID of manufacture you want to update"
//	@Param			request		body		models.Manufacture	true	"query params"	test
//	@Success		200			{object}	models.Status		"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/manufacture/{manufacture}/update [post]
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
//	@Security		OAuth2Application[write]
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.Manufacture	true	"query params"	test
//	@Success		200		{object}	models.Manufacture	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/manufacture/insert [put]
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

    createdObject, err := utils.GenericInsert[models.Manufacture]("manufacture", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, createdObject)
}

//	@Summary		Delete manufacture with ID
//	@Description	Delete manufacture with corresponding ID value
//	@Security		OAuth2Application[write]
//	@Tags			Manufacture
//	@Accept			json
//	@Produce		json
//	@Param			manufacture	path		int				true	"Unique ID of manufacture you want to update"
//	@Success		200			{object}	models.Status	"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/manufacture/{manufacture}/delete [delete]
func DeleteManufature(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "manufacture"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    result, err := utils.GenericDelete[models.Manufacture]("manufacture", urlParameter, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    log.Infof("success! Manufacturer with manufacture_id %v and manufacture_name %s was deleted", result.ManufactureID, result.ManufactureName)
    c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! Manufacturer with manufacture_id %v and manufacture_name %s was deleted", result.ManufactureID, result.ManufactureName)})
}
