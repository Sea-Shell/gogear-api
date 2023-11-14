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

// @Summary        Get top category with ID
// @Description    Get top category spessific to ID
// @Tags           Top Category
// @Accept         json
// @Produce        json
// @Param          topCategoryId      path        int                     true    "Unique ID of top category you want to get"
// @Success        200                {object}    models.GearTopCategory
// @Failure        default            {object}    models.Error
// @Router         /topCategory/{topCategory}/get [get]
func GetTopCategory(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "topCategory"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
    }

    var extraSQl []string
    // extraSQl = append(extraSQl, " LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId ")
    // extraSQl = append(extraSQl, " LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId ")
    // extraSQl = append(extraSQl, "  LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")

    results, err := utils.GenericGet[models.GearTopCategory]("gear_top_category", urlParameter, extraSQl, db)
    if err != nil {
        log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
    c.IndentedJSON(http.StatusOK, results)
}

// @Summary        List top categories
// @Description    Get a list of top category items
// @Tags           Top Category
// @Accept         json
// @Produce        json
// @Param          page               query        int            false    "Page number"                default(1)
// @Param          limit              query        int            false    "Number of items per page"   default(30)
// @Param          topCategory        query        []int          false    "top categories"             collectionFormat(multi)
// @Success        200                {object}     models.ResponsePayload{items=[]models.GearTopCategory}
// @Failure        default            {object}     models.Error
// @Router         /topCategory/list [get]
func ListTopCategory(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    currentQueryParameters := c.Request.URL.Query()

    page := c.Query("page")
    limit := c.Query("limit")
    topCategories := c.QueryArray("topCategory")

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

    for _, category := range topCategories {
        topcat := fmt.Sprintf("gear_top_category.topCategoryId = %s", category)
        conditions = append(conditions, topcat)
    }

    whereClause := ""
    if len(conditions) > 0 {
        whereClause = " WHERE " + strings.Join(conditions, " OR ")
    }

    baseCountQuery := "SELECT COUNT(*) FROM gear_top_category"
    countQuery := baseCountQuery + whereClause

    var totalCount int
    err = db.QueryRow(countQuery).Scan(&totalCount)
    if err != nil {
        log.Errorf("Error getting GearCount database: %#v", err)
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    start := strconv.Itoa((page_int - 1) * limit_int)
    totalPages := int(math.Ceil(float64(totalCount) / float64(limit_int)))

    start_int, err := strconv.Atoi(start)
    if err != nil {
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    var param_topCategory models.GearTopCategory
    fields := utils.GetDBFieldNames(reflect.TypeOf(param_topCategory))

    baseQuery := fmt.Sprintf(`SELECT %s FROM gear_top_category`, strings.Join(fields, ", "))

    queryLimit := fmt.Sprintf(" LIMIT %v, %v", start_int, limit_int)

    query := baseQuery + whereClause + queryLimit

    log.Debugf("Query: %s", query)

    rows, err := db.Query(query)
    if err != nil {
        log.Errorf("Query error: %#v", err.Error())
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    dest, err := utils.GetScanFields(param_topCategory)
    if err != nil {
        log.Errorf("Error getting destination arguments: %#v", err)
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    var gearTopCategoryList []models.GearTopCategory

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

        for i := 0; i < reflect.TypeOf(param_topCategory).NumField(); i++ {
            reflect.ValueOf(&param_topCategory).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
        }

        gearTopCategoryList = append(gearTopCategoryList, param_topCategory)
    }

    payload := models.ResponsePayload{
        TotalItemCount: totalCount,
        CurrentPage:    page_int,
        ItemLimit:      limit_int,
        TotalPages:     totalPages,
        Items:          gearTopCategoryList,
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

// @Summary        Update top category with ID
// @Description    Update top category identified by ID
// @Security       OAuth2Application[write]
// @Tags           Top Category
// @Accept         json
// @Produce        json
// @Param          topCategoryId      path        int                      true        "Unique ID of top category you want to update"
// @Param          request            body        models.GearTopCategory   true        "Request body"
// @Success        200                {object}    models.Status            "status: success when all goes well"
// @Failure        default            {object}    models.Error
// @Router         /topCategory/{topCategory}/update [post]
func UpdateTopCategory(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericUpdate[models.GearTopCategory]("gear_top_category", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// @Summary        Insert new top category
// @Description    Insert new top category with corresponding values
// @Security       OAuth2Application[write]
// @Tags           Top Category
// @Accept         json
// @Produce        json
// @Param          request            body        models.GearTopCategory    true    "Request body"
// @Success        200                {object}    models.Status             "status: success when all goes well"
// @Failure        default            {object}    models.Error
// @Router         /topCategory/insert [put]
func InsertTopCategory(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericInsert[models.GearTopCategory]("gear_top_category", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// @Summary        Delete topCategory with ID
// @Description    Delete topCategory with corresponding ID value
// @Security       OAuth2Application[write]
// @Tags           Top Category
// @Accept         json
// @Produce        json
// @Param          topCategory        path        int                    true    "Unique ID of topCategory you want to update"
// @Success        200                {object}    models.Status          "status: success when all goes well"
// @Failure        default            {object}    models.Error
// @Router         /topCategory/{topCategory}/delete [delete]
func DeleteTopCategory(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "topCategory"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    result, err := utils.GenericDelete[models.GearTopCategory]("gear_top_category", urlParameter, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    log.Infof("success! Top category with top_category_id %v and top_category_name %s was deleted", result.TopCategoryId, result.TopCategoryName)
    c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! Top category with top_category_id %v and top_category_name %s was deleted", result.TopCategoryId, result.TopCategoryName)})
}
