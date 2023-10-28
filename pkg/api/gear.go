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

//	@Summary		List gear
//	@Description	Get a list of gear items
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			page			query		int			false	"Page number"				default(1)
//	@Param			limit			query		int			false	"Number of items per page"	default(30)
//	@Param			category		query		string		false	"Gear category"
//	@Param			topCategory		query		string		false	"Top gear category"
//	@Param			manufacturer	query		string		false	"Gear manufacturer"
//	@Param			collection		query		[]string	false	"string collection"	collectionFormat(multi)
//	@Success		200				{object}	models.ResponsePayload{items=[]models.GearListItem}
//	@Failure		default		{object}	models.Error
//	@Router			/gear/list [get]
func ListGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    currentQueryParameters := c.Request.URL.Query()

    page := c.Query("page")
    limit := c.Query("limit")
    category := c.Query("category")
    topCategory := c.Query("topCategory")
    manufacturer := c.Query("manufacturer")
    

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
    if topCategory != "" {
        topcat := fmt.Sprintf("gear.gearTopCategoryId = %s", topCategory)
        conditions = append(conditions, topcat)
    }
    if category != "" {
        cat := fmt.Sprintf("gear.gearCategoryId = %s", category)
        conditions = append(conditions, cat)
    }
    if manufacturer != "" {
        cat := fmt.Sprintf("gear.GearManufactureId = %s", manufacturer)
        conditions = append(conditions, cat)
    }

    whereClause := ""
    if len(conditions) > 0 {
        whereClause = " WHERE " + strings.Join(conditions, " AND ")
    }

    baseCountQuery := "SELECT COUNT(*) FROM gear"
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

    var param_gear models.GearListItem
    fields := utils.GetDBFieldNames(reflect.TypeOf(param_gear))

    baseQuery := fmt.Sprintf(`SELECT %s FROM gear
        LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId
        LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId
        LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId`, 
        strings.Join(fields, ", "))

    queryLimit := fmt.Sprintf(" LIMIT %v, %v", start_int, limit_int)

    query := baseQuery + whereClause + queryLimit

    log.Debugf("Query: %s", query)

    rows, err := db.Query(query)
    if err != nil {
        log.Errorf("Query error: %#v", err.Error())
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    dest, err := utils.GetScanFields(param_gear)
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

        for i := 0; i < reflect.TypeOf(param_gear).NumField(); i++ {
            reflect.ValueOf(&param_gear).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
        }

        gearList = append(gearList, param_gear)
    }

    payload := models.ResponsePayload{
        TotalItemCount: totalCount,
        CurrentPage:    page_int,
        ItemLimit:      limit_int,
        TotalPages:     totalPages,
        Items:          gearList,
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

    log.Infof("successfully fetched gear with id: %s, gearName: %s", param_gear.GearId, param_gear.GearName)
    c.IndentedJSON(http.StatusOK, payload)
}

//	@Summary		Get gear with ID
//	@Description	Get gear spessific to ID
//	@Tags			Gear
//	@Accept			json 
//	@Produce		json
//	@Param			gear	path		int				true	"Unique ID of Gear you want to get"
//	@Success		200		{object}	models.FullGear	"desc"
//	@Failure		default		{object}	models.Error
//	@Router			/gear/{gear}/get [get]
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
    
    var extraSQl []string
    extraSQl = append(extraSQl, " LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId ")
    extraSQl = append(extraSQl, " LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId ")
    extraSQl = append(extraSQl, "  LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")
    
    results, err := utils.GenericGet[models.FullGear]("gear", urlParameter, extraSQl, db)
    if err != nil {
        log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
    c.IndentedJSON(http.StatusOK, results)
}

//	@Summary		Insert new gear
//	@Description	Insert new gear with corresponding values
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.GearNoId	true	"query params"	test
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/gear/insert [put]
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

    err = utils.GenericInsert[models.Gear]("gear", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

//	@Summary		Update gear with ID
//	@Description	Update gear identified by ID
//	@Tags			Gear
//	@Accept			json
//	@Produce		json
//	@Param			gear	path		int				true	"Unique ID of Gear you want to get"
//	@Param			request	body		models.Gear		true	"query params"	test
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/gear/{gear}/update [post]
func UpdateGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericUpdate[models.Gear]("gear", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// @Summary		Delete gear with ID
// @Description	Delete gear with corresponding ID value
// @Security	BearerAuth
// @Tags		Gear
// @Accept		json
// @Produce		json
// @Param		gear		path		int					true	"Unique ID of gear you want to delete"
// @Success		200				{object}	models.Status	"status: success when all goes well"
// @Failure		default			{object}	models.Error
// @Router		/gear/{gear}/delete [delete]
func DeleteGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "gear"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    result, err := utils.GenericDelete[models.Gear]("gear", urlParameter, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    log.Infof("success! Gear with gear_id %v and gear_name %s was deleted", result.GearId, result.GearName)
    c.JSON(http.StatusOK, map[string]string{
        "status": fmt.Sprintf("success! Gear with gear_id %v and gear_name %s was deleted", result.GearId, result.GearName),
    })
}
