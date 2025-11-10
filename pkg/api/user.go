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

// ListUser lists users based on query
//
// @Summary		List user
// @Description	Get a list of user items
// @Security		BearerAuth
// @Tags			User
// @Accept			json
// @Produce		json
// @Param			page		  query		  int		false	"Page number"				default(1)
// @Param			limit		  query		  int		false	"Number of items per page"	default(30)
// @Param			user		  query		  string	false	"search by user's username (this is case insensitive and wildcard)"
// @Param			username	query		  string	false	"search by users full name (this is case insensitive and wildcard)"
// @Param			email  	  query		  string	false	"search by users email (this is case insensitive and wildcard)"
// @Success		200			  {object}	models.ResponsePayload{items=[]models.User}
// @Failure		default		{object}	models.Error
// @Router			/api/v1/users/list [get]
func ListUser(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	qUserUsername := c.QueryArray("user")
	qUserName := c.QueryArray("username")
	qUserEmail := c.QueryArray("email")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

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

	params := []interface{}{}
	for _, username := range qUserUsername {
		conditions = append(conditions, "userUsername LIKE ?")
		params = append(params, "%"+username+"%")
	}

	for _, name := range qUserName {
		conditions = append(conditions, "userName LIKE ?")
		params = append(params, "%"+name+"%")
	}

	for _, email := range qUserEmail {
		conditions = append(conditions, "userEmail LIKE ?")
		params = append(params, "%"+email+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " OR ")
	}

	baseCountQuery := "SELECT COUNT(*) FROM users"
	countQuery := baseCountQuery + whereClause

	var totalCount int
	err = db.QueryRow(countQuery, params...).Scan(&totalCount)
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

	var paramUser models.User
	fields := utils.GetDBFieldNames(reflect.TypeOf(paramUser))

	baseQuery := fmt.Sprintf(`SELECT %s FROM users`, strings.Join(fields, ", "))

	queryLimit := fmt.Sprintf(" LIMIT %v, %v", startInt, limitInt) // validated beforehand

	query := baseQuery + whereClause + queryLimit

	log.Debugf("Query: %s Params: %#v", query, params)

	rows, err := db.Query(query, params...)
	if err != nil {
		log.Errorf("Query error: %#v", err.Error())
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	defer rows.Close()

	dest, err := utils.GetScanFields(paramUser)
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

		for i := 0; i < reflect.TypeOf(paramUser).NumField(); i++ {
			reflect.ValueOf(&paramUser).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		users = append(users, paramUser)
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
		Items:          users,
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

// GetUser gets spessific user based on ID
//
// @Summary		Get user with ID
// @Description	Get user spessific to ID
// @Security		BearerAuth
// @Tags			User
// @Accept			json
// @Produce		json
// @Param			user	path		int			true	"Unique ID of user you want to get"
// @Success		200		{object}	models.User	"desc"
// @Failure		default	{object}	models.Error
// @Router			/api/v1/users/{user}/get [get]
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

	var extraSQL []string
	// extraSQL = append(extraSQL, " LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId ")
	// extraSQL = append(extraSQL, " LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId ")
	// extraSQL = append(extraSQL, "  LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")

	results, err := utils.GenericGet[models.User]("users", urlParameter, extraSQL, db)
	if err != nil {
		log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
	c.IndentedJSON(http.StatusOK, results)
}

// InsertUser creates new user in the database
//
// @Summary		Insert new user
// @Description	Insert new user with corresponding values
// @Security		BearerAuth
// @Tags			User
// @Accept			json
// @Produce		json
// @Param			request	body		models.UserWithPass	true	"query params"	test
// @Success		200		{object}	models.Status		"status: success when all goes well"
// @Failure		default	{object}	models.Error
// @Router			/api/v1/users/insert [put]
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

	_, err = utils.GenericInsert[models.User]("users", data, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// UpdateUser updates user in database
//
// @Summary		Update user with ID
// @Description	Update user identified by ID
// @Security		BearerAuth
// @Tags			User
// @Accept			json
// @Produce		json
// @Param			user	path		int				true	"Unique ID of user you want to update"
// @Param			request	body		models.User		true	"query params"	test
// @Success		200		{object}	models.Status	"status: success when all goes well"
// @Failure		default	{object}	models.Error
// @Router			/api/v1/users/{user}/update [post]
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

// DeleteUser delets user from database
//
// @Summary		Delete user with ID
// @Description	Delete user with corresponding ID value
// @Security		BearerAuth
// @Tags			User
// @Accept			json
// @Produce		json
// @Param			user	path		int				true	"Unique ID of user you want to update"
// @Success		200		{object}	models.Status	"status: success when all goes well"
// @Failure		default	{object}	models.Error
// @Router			/api/v1/users/{user}/delete [delete]
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
		_, err := utils.GenericDelete[models.UserGearLink]("user_gear_registrations", int(*userRegistration.UserGearRegistrationID), db)
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

	log.Infof("success! User %s (ID: %v) and all the users gear association was deleted", result.UserUsername, *result.UserID)
	c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! User %s (ID: %v) and all the users gear association was deleted", result.UserUsername, *result.UserID)})
}
