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

// @Summary        List users gear
// @Description    Get a list a users gear
// @Security       BearerAuth
// @Tags           User gear
// @Accept         json
// @Produce        json
// @Param          user               path         int          true     "Unique ID of user you want to get the Gear of"
// @Param          page               query        int          false    "Page number"                 default(1)
// @Param          limit              query        int          false    "Number of items per page"    default(30)
// @Param          topCategory        query        []int        false    "top categories"              collectionFormat(multi)
// @Param          category           query        []int        false    "sub categories"              collectionFormat(multi)
// @Param          manufacture        query        []int        false    "manufacturers"               collectionFormat(multi)
// @Success        200                {object}    models.ResponsePayload{items=[]models.UserGear}
// @Failure        default            {object}    models.Error
// @Router         /usergear/{user}/list [get]
func ListUserGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    currentQueryParameters := c.Request.URL.Query()

    page := c.Query("page")
    limit := c.Query("limit")
    userId := c.Param("user")
    topCategories := c.QueryArray("topCategory")
    categories := c.QueryArray("category")
    manufacturers := c.QueryArray("manufacture")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    log.Debugf("Request parameters: %#v", c.Request.URL.Query())

    if userId == "" {
        log.Errorf("Error userId was not supplied")
        c.IndentedJSON(http.StatusNoContent, models.Error{Error: "userId supplied was not valid"})
        return
    }

    if limit == "" {
        limit = "30"
    }

    if page == "" || page == "0" {
        page = "1"
    }

    userId_int, err := strconv.Atoi(userId)
    if err != nil {
        log.Errorf("Error setting userId to int: %#v", err)
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
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

    if userId != "" {
        userIdQ := fmt.Sprintf("user_gear_registrations.userId = %d", userId_int)
        conditions = append(conditions, userIdQ)
    }

    for _, topCategory := range topCategories {
        topCategory_int, err := strconv.Atoi(topCategory)
        if err != nil {
            continue
        }
        topcat := fmt.Sprintf("gear.gearTopCategoryId = %d", topCategory_int)
        conditions = append(conditions, topcat)
    }

    for _, category := range categories {
        category_int, err := strconv.Atoi(category)
        if err != nil {
            continue
        }
        cat := fmt.Sprintf("gear.gearCategoryId = %d", category_int)
        conditions = append(conditions, cat)
    }

    for _, manufacture := range manufacturers {
        manufacture_int, err := strconv.Atoi(manufacture)
        if err != nil {
            continue
        }
        manufac := fmt.Sprintf("gear.gearManufactureId = %d", manufacture_int)
        conditions = append(conditions, manufac)
    }

    whereClause := ""
    if len(conditions) > 0 {
        whereClause = " WHERE " + strings.Join(conditions, " AND ")
    }

    var extra []string
    extra = append(extra, " LEFT JOIN gear ON user_gear_registrations.gearId = gear.gearId")
    extra = append(extra, "LEFT JOIN users ON user_gear_registrations.userId = users.userId")
    extra = append(extra, "LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId")
    extra = append(extra, "LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId")
    extra = append(extra, "LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")
    extraSQl := strings.Join(extra, " ")

    baseCountQuery := "SELECT COUNT(*) FROM user_gear_registrations"
    countQuery := baseCountQuery + extraSQl + whereClause

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

    var param_topCategory models.UserGear
    fields := utils.GetDBFieldNames(reflect.TypeOf(param_topCategory))

    baseQuery := fmt.Sprintf(`SELECT %s FROM user_gear_registrations`, strings.Join(fields, ", "))

    queryLimit := fmt.Sprintf(" LIMIT %v, %v", start_int, limit_int)

    query := baseQuery + extraSQl + whereClause + queryLimit

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

    var gearTopCategoryList []models.UserGear

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

// @Summary        Get user registered gear with ID
// @Description    Get user registeredgear spessific to ID
// @Security       BearerAuth
// @Tags           User gear
// @Accept         json
// @Produce        json
// @Param          usergear    path        int                true    "Unique ID of user registered gear you want to get"
// @Success        200         {object}    models.UserGear    "desc"
// @Router         /usergear/registration/{usergear}/get [get]
func GetUserGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "usergear"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
    }

    var extra []string
    extra = append(extra, " LEFT JOIN gear ON user_gear_registrations.gearId = gear.gearId")
    extra = append(extra, "LEFT JOIN users ON user_gear_registrations.gearId = users.userId")
    extra = append(extra, "LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId")
    extra = append(extra, "LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId")
    extra = append(extra, "LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")

    results, err := utils.GenericGet[models.UserGear]("user_gear_registrations", urlParameter, extra, db)
    if err != nil {
        log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
    c.IndentedJSON(http.StatusOK, results)
}

// @Summary        Insert user registered gear
// @Description    Insert user registered gear with corresponding values
// @Security       BearerAuth
// @Tags           User gear
// @Accept         json
// @Produce        json
// @Param          request    body        models.UserGearLinkNoId    true    "query params"
// @Success        200        {object}    models.Status              "status: success when all goes well"
// @Router         /usergear/insert [put]
func InsertUserGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericInsert[models.UserGearLink]("user_gear_registrations", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// @Summary        Update user registered gear with ID
// @Description    Update user registered gear identified by ID
// @Security       BearerAuth
// @Tags           User gear
// @Accept         json
// @Produce        json
// @Param          usergear    path        int                  true    "Unique ID of user registered gear you want to get"
// @Param          request     body        models.UserGearLink  true    "query params"
// @Success        200         {object}    models.Status        "status: success when all goes well"
// @Router         /usergear/registration/{usergear}/update [post]
func UpdateUserGear(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericUpdate[models.UserGearLink]("user_gear_registrations", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// @Summary      Delete userGear with ID
// @Description  Delete userGear with corresponding ID value
// @Security     BearerAuth
// @Tags         User gear
// @Accept       json
// @Produce      json
// @Param        userGear       path        int              true    "Unique ID of userGear you want to update"
// @Success      200            {object}    models.Status    "status: success when all goes well"
// @Failure      default        {object}    models.Error
// @Router       /usergear/registration/{usergear}/delete [delete]
func DeleteUserGearRegistration(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "usergear"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    result, err := utils.GenericDelete[models.UserGear]("user_gear_registration", urlParameter, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    log.Infof("success! User gear registration linking %s to user %s was deleted", result.GearName, result.UserGearUserId)
    c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! User gear registration linking %s to user %s was deleted", result.GearName, result.UserUsername)})
}
