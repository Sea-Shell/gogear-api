package endpoints

import (
	"database/sql"
	"encoding/json"
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

//	@Summary		List user
//	@Description	Get a list of user items
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			page		query		int		false	"Page number"				default(1)
//	@Param			limit		query		int		false	"Number of items per page"	default(30)
//	@Param			user		query		string	false	"search by username (this is case insensitive and wildcard)"
//	@Param			username	query		string	false	"search by users full name (this is case insensitive and wildcard)"
//	@Success		200			{object}	models.ResponsePayload{items=[]models.User}
//	@Failure		default		{object}	models.Error
//	@Router			/users/list [get]
func ListUser(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    currentQueryParameters := c.Request.URL.Query()

    page := c.Query("page")
    limit := c.Query("limit")
    qUser := c.QueryArray("user")
    qUserUsername := c.QueryArray("username")
    qUserName := c.QueryArray("name")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

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

    for _, user := range qUser {
        topcat := fmt.Sprintf("userId = %s", user)
        conditions = append(conditions, topcat)
    }

    for _, username := range qUserUsername {
        topcat := fmt.Sprintf("userUsername LIKE '%s'", "%"+username+"%")
        conditions = append(conditions, topcat)
    }

    for _, name := range qUserName {
        topcat := fmt.Sprintf("userName LIKE '%s'", "%"+name+"%")
        conditions = append(conditions, topcat)
    }

    whereClause := ""
    if len(conditions) > 0 {
        whereClause = " WHERE " + strings.Join(conditions, " OR ")
    }

    baseCountQuery := "SELECT COUNT(*) FROM users"
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

    var param_user models.User
    fields := utils.GetDBFieldNames(reflect.TypeOf(param_user))

    baseQuery := fmt.Sprintf(`SELECT %s FROM users`, strings.Join(fields, ", "))

    queryLimit := fmt.Sprintf(" LIMIT %v, %v", start_int, limit_int)

    query := baseQuery + whereClause + queryLimit

    log.Debugf("Query: %s", query)

    rows, err := db.Query(query)
    if err != nil {
        log.Errorf("Query error: %#v", err.Error())
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    dest, err := utils.GetScanFields(param_user)
    if err != nil {
        log.Errorf("Error getting destination arguments: %#v", err)
        c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    var users []models.User

    for rows.Next() {
        err = rows.Scan(dest...)
        if err != nil {
            if err == sql.ErrNoRows {
                log.Errorf("No user found")
                c.IndentedJSON(http.StatusNotFound, models.Error{Error: "No results"})
                return
            }
            log.Errorf("Scan error: %#v", err)
            c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
            return
        }

        for i := 0; i < reflect.TypeOf(param_user).NumField(); i++ {
            reflect.ValueOf(&param_user).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
        }

        users = append(users, param_user)
    }

    payload := models.ResponsePayload{
        TotalItemCount: totalCount,
        CurrentPage:    page_int,
        ItemLimit:      limit_int,
        TotalPages:     totalPages,
        Items:          users,
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

//	@Summary		Get user with ID
//	@Description	Get user spessific to ID
//	@Tags			User
//	@Accept			json 
//	@Produce		json
//	@Param			user	path		int			true	"Unique ID of user you want to get"
//	@Success		200		{object}	models.User	"desc"
//	@Failure		default		{object}	models.Error
//	@Router			/users/{user}/get [get]
func GetUser(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "user"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
    }

    var extraSQl []string
    // extraSQl = append(extraSQl, " LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId ")
    // extraSQl = append(extraSQl, " LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId ")
    // extraSQl = append(extraSQl, "  LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")

    results, err := utils.GenericGet[models.User]("users", urlParameter, extraSQl, db)
    if err != nil {
        log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
    c.IndentedJSON(http.StatusOK, results)
}

func SetUserPassword(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    user := c.Param("user")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    var body map[string]interface{}
    err = json.Unmarshal(data, &body)
    if err != nil {
        c.IndentedJSON(http.StatusNotModified, gin.H{"error": err.Error()})
        log.Error(err.Error())
        return
    }

    log.Debugln(user, body)

    if !utils.IsPasswordStrong(body["password"].(string)) {
        c.JSON(http.StatusNotModified, models.Error{Error: "Password is to weak"})
        log.Error("password supplied is to weak, aborting!")
        return
    }

    var password string
    password, err = utils.HashPassword(body["password"].(string), 10)
    if err != nil {
        c.JSON(http.StatusNotModified, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    log.Debugln(password, user)

    _, err = db.Exec("UPDATE users SET password = ? WHERE userName == ?", password, user)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

//	@Summary		Insert new user
//	@Description	Insert new user with corresponding values
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.UserWithPass	true	"query params"	test
//	@Success		200		{object}	models.Status		"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/users/insert [put]
func InsertUser(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericInsert[models.User]("users", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

//	@Summary		Update user with ID
//	@Description	Update user identified by ID
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			user	path		int				true	"Unique ID of user you want to update"
//	@Param			request	body		models.User		true	"query params"	test
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/users/{user}/update [post]
func UpdateUser(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)

    data, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    err = utils.GenericUpdate[models.User]("users", data, db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        log.Error(err.Error())
        return
    }

    c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// @Summary		Delete user with ID
// @Description	Delete user with corresponding ID value
// @Security	BearerAuth
// @Tags		User
// @Accept		json
// @Produce		json
// @Param		user			path		int				true	"Unique ID of user you want to update"
// @Success		200				{object}	models.Status	"status: success when all goes well"
// @Failure		default			{object}	models.Error
// @Router		/users/{user}/delete [delete]
func DeleteUser(c *gin.Context) {
    c.Header("Content-Type", "application/json")

    log := c.MustGet("logger").(*zap.SugaredLogger)
    db := c.MustGet("db").(*sql.DB)
    function := "user"

    urlParameter, err := strconv.Atoi(c.Param(function))
    if err != nil {
        log.Errorf("urlParamter is of wrong type: %#v", err)
        c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
        return
    }

    userRegistrations, err := utils.GenericList[models.UserGearLink]("user_gear_registrations", "userId", urlParameter, db)
    if err != nil {
        log.Error(err.Error())
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    for _, userRegistration := range *userRegistrations {
        _, err := utils.GenericDelete[models.UserGearLink]("user_gear_registrations", int(*userRegistration.UserGearRegistrationId), db)
        if err != nil {
            log.Error(err.Error())
            c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
            return
        }
    }

    result, err := utils.GenericDelete[models.User]("users", urlParameter, db)
    if err != nil {
        log.Error(err.Error())
        c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
        return
    }

    log.Infof("success! User %s (ID: %v) and all the users gear association was deleted", result.UserUsername, *result.UserId)
    c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! User %s (ID: %v) and all the users gear association was deleted", result.UserUsername, *result.UserId)})
}