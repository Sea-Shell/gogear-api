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

// ListUserGear list the users registered gear
//
// @Summary		List users gear
// @Description	Get a list a users gear
// @Security		BearerAuth
// @Tags			User gear
// @Accept			json
// @Produce		json
// @Param			user		path		int		true	"Unique ID of user you want to get the Gear of"
// @Param			page		query		int		false	"Page number"				default(1)
// @Param			limit		query		int		false	"Number of items per page"	default(30)
// @Param			topCategory	query		[]int	false	"top categories"			collectionFormat(multi)
// @Param			category	query		[]int	false	"sub categories"			collectionFormat(multi)
// @Param			manufacture	query		[]int	false	"manufacturers"				collectionFormat(multi)
// @Param			container			query		string		false		"show container gear only. valid values are true, false, all"	default(all)
// @Success		200			{object}	models.ResponsePayload{items=[]models.UserGear}
// @Failure		default		{object}	models.Error
// @Router			/api/v1/usergear/{user}/list [get]
func ListUserGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	userID := c.Param("user")
	topCategories := c.QueryArray("topCategory")
	categories := c.QueryArray("category")
	manufacturers := c.QueryArray("manufacture")
	container, containerBool := c.GetQuery("container")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	log.Debugf("Request parameters: %#v", c.Request.URL.Query())

	if userID == "" {
		log.Errorf("Error userID was not supplied")
		c.IndentedJSON(http.StatusNoContent, models.Error{Error: "userId supplied was not valid"})
		return
	}

	if limit == "" {
		limit = "30"
	}

	if page == "" || page == "0" {
		page = "1"
	}

	if !containerBool {
		container = "all"
	}

	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		log.Errorf("Error setting userID to int: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	subjectAny, hasSubject := c.Get("user_id")
	if !hasSubject {
		log.Warn("JWT subject missing from context")
		c.IndentedJSON(http.StatusUnauthorized, models.Error{Error: "authentication context missing"})
		return
	}

	subject, _ := subjectAny.(string)
	subject = strings.TrimSpace(subject)

	isAdmin := false
	if adminAny, ok := c.Get("user_is_admin"); ok {
		if adminFlag, ok := adminAny.(bool); ok {
			isAdmin = adminFlag
		}
	}

	if !isAdmin {
		subjectID, parseErr := strconv.Atoi(subject)
		if parseErr != nil || subjectID != userIDInt {
			log.Warnw("non-admin attempted to view other user's gear", "requested_user", userIDInt, "subject", subject)
			c.IndentedJSON(http.StatusForbidden, models.Error{Error: "not allowed to view registrations for this user"})
			return
		}
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

	if userID != "" {
		userIDQ := fmt.Sprintf("user_gear_registrations.userId = %d", userIDInt)
		conditions = append(conditions, userIDQ)
	}

	for _, topCategory := range topCategories {
		topCategoryInt, err := strconv.Atoi(topCategory)
		if err != nil {
			continue
		}
		topcat := fmt.Sprintf("gear.gearTopCategoryId = %d", topCategoryInt)
		conditions = append(conditions, topcat)
	}

	for _, category := range categories {
		categoryInt, err := strconv.Atoi(category)
		if err != nil {
			continue
		}
		cat := fmt.Sprintf("gear.gearCategoryId = %d", categoryInt)
		conditions = append(conditions, cat)
	}

	for _, manufacture := range manufacturers {
		manufactureInt, err := strconv.Atoi(manufacture)
		if err != nil {
			continue
		}
		manufac := fmt.Sprintf("gear.gearManufactureId = %d", manufactureInt)
		conditions = append(conditions, manufac)
	}

	switch container {
	case "true":
		containerCond := "gear.gearIsContainer = 1"
		conditions = append(conditions, containerCond)
	case "false":
		containerCond := "gear.gearIsContainer = 0"
		conditions = append(conditions, containerCond)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	var extra []string
	extra = append(extra, " LEFT JOIN user_container_registration ON user_container_registration.userGearRegistrationId = user_gear_registrations.userGearRegistrationId")
	extra = append(extra, " LEFT JOIN gear ON user_gear_registrations.gearId = gear.gearId")
	extra = append(extra, "LEFT JOIN users ON user_gear_registrations.userId = users.userId")
	extra = append(extra, "LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId")
	extra = append(extra, "LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId")
	extra = append(extra, "LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")
	extraSQL := strings.Join(extra, " ")

	baseCountQuery := "SELECT COUNT(*) FROM user_gear_registrations"
	countQuery := baseCountQuery + extraSQL + whereClause

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

	var paramTopCategory models.UserGear
	fields := utils.GetDBFieldNames(reflect.TypeOf(paramTopCategory))

	baseQuery := fmt.Sprintf(`SELECT %s FROM user_gear_registrations`, strings.Join(fields, ", "))

	queryLimit := fmt.Sprintf(" LIMIT %v, %v", startInt, limitInt)

	query := baseQuery + extraSQL + whereClause + queryLimit

	log.Debugf("Query: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		log.Errorf("Query error: %#v", err.Error())
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	defer rows.Close()

	dest, err := utils.GetScanFields(paramTopCategory)
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

		for i := 0; i < reflect.TypeOf(paramTopCategory).NumField(); i++ {
			reflect.ValueOf(&paramTopCategory).Elem().Field(i).Set(reflect.ValueOf(dest[i]).Elem())
		}

		gearTopCategoryList = append(gearTopCategoryList, paramTopCategory)
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
		Items:          gearTopCategoryList,
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

// GetUserGear retrives the users full gear list
//
// @Summary		Get user registered gear with ID
// @Description	Get user registeredgear spessific to ID
// @Security		BearerAuth
// @Tags			User gear
// @Accept			json
// @Produce		json
// @Param			usergear	path		int				true	"Unique ID of user registered gear you want to get"
// @Success		200			{object}	models.UserGear	"desc"
// @Router			/api/v1/usergear/registration/{usergear}/get [get]
func GetUserGear(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	function := "usergear"

	topCategories := c.QueryArray("topCategory")
	categories := c.QueryArray("category")

	urlParameter, err := strconv.Atoi(c.Param(function))
	if err != nil {
		log.Errorf("urlParamter is of wrong type: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
	}

	conditions := []string{}

	for _, topCategory := range topCategories {
		topcat := fmt.Sprintf("gear_category.categoryTopCategoryId = %s", topCategory)
		conditions = append(conditions, topcat)
	}

	for _, category := range categories {
		topcat := fmt.Sprintf("gear_category.categoryId = %s", category)
		conditions = append(conditions, topcat)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " OR ")
	}

	var extra []string
	extra = append(extra, " LEFT JOIN user_container_registration ON user_container_registration.userGearRegistrationId = user_gear_registrations.userGearRegistrationId")
	extra = append(extra, " LEFT JOIN gear ON user_gear_registrations.gearId = gear.gearId")
	extra = append(extra, "LEFT JOIN users ON user_gear_registrations.userId = users.userId")
	extra = append(extra, "LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId")
	extra = append(extra, "LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId")
	extra = append(extra, "LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")

	extra = append(extra, whereClause)

	results, err := utils.GenericGet[models.UserGear]("user_gear_registrations", urlParameter, extra, db)
	if err != nil {
		log.Errorf("Unable to get %s with id: %s. Error: %#v", function, urlParameter, err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	log.Infof("Successfully fetched %s with ID %s", function, urlParameter)
	c.IndentedJSON(http.StatusOK, results)
}

// InsertUserGear puts gear on the users gear list
//
// @Summary		Insert user registered gear
// @Description	Insert user registered gear with corresponding values
// @Security		BearerAuth
// @Tags			User gear
// @Accept			json
// @Produce		json
// @Param			request	body		models.UserGearLinkNoID	true	"query params"
// @Success		200		{object}	models.Status			"status: success when all goes well"
// @Router			/api/v1/usergear/insert [put]
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

	_, err = utils.GenericInsert[models.UserGearLink]("user_gear_registrations", data, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// UpdateUserGear updates a record of user registered gear
//
// @Summary		Update user registered gear with ID
// @Description	Update user registered gear identified by ID
// @Security		BearerAuth
// @Tags			User gear
// @Accept			json
// @Produce		json
// @Param			usergear	path		int					true	"Unique ID of user registered gear you want to get"
// @Param			request		body		models.UserGearLink	true	"query params"
// @Success		200			{object}	models.Status		"status: success when all goes well"
// @Router			/api/v1/usergear/registration/{usergear}/update [post]
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

// DeleteUserGearRegistration deletes a registered gear item from users list
//
// @Summary		Delete userGear with ID
// @Description	Delete userGear with corresponding ID value
// @Security		BearerAuth
// @Tags			User gear
// @Accept			json
// @Produce		json
// @Param			userGear	path		int				true	"Unique ID of userGear you want to update"
// @Success		200			{object}	models.Status	"status: success when all goes well"
// @Failure		default		{object}	models.Error
// @Router			/api/v1/usergear/registration/{usergear}/delete [delete]
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

	const detailQuery = `SELECT gear.gearName, users.userUsername, users.userId
		FROM user_gear_registrations
		LEFT JOIN gear ON user_gear_registrations.gearId = gear.gearId
		LEFT JOIN users ON user_gear_registrations.userId = users.userId
		WHERE user_gear_registrations.userGearRegistrationId = ?
		LIMIT 1`

	var gearName sql.NullString
	var userUsername sql.NullString
	var userID sql.NullInt64

	if err := db.QueryRow(detailQuery, urlParameter).Scan(&gearName, &userUsername, &userID); err != nil {
		if err == sql.ErrNoRows {
			log.Warnw("user gear registration not found", "registration_id", urlParameter)
			c.IndentedJSON(http.StatusNotFound, models.Error{Error: "registration not found"})
			return
		}
		log.Errorw("failed to inspect registration before deletion", "error", err, "registration_id", urlParameter)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	_, err = db.Exec("DELETE FROM user_container_registration WHERE userGearRegistrationId = ? OR userContainerId = ?", urlParameter, urlParameter)
	if err != nil {
		log.Errorw("failed to clear container links before deleting registration", "error", err, "registration_id", urlParameter)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	result, err := db.Exec("DELETE FROM user_gear_registrations WHERE userGearRegistrationId = ?", urlParameter)
	if err != nil {
		log.Errorw("failed to delete user gear registration", "error", err, "registration_id", urlParameter)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Errorw("unable to determine rows affected during deletion", "error", err, "registration_id", urlParameter)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if rowsAffected == 0 {
		log.Warnw("delete attempted but no rows removed", "registration_id", urlParameter)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "registration not found"})
		return
	}

	gearLabel := gearName.String
	if gearLabel == "" {
		gearLabel = fmt.Sprintf("registration #%d", urlParameter)
	}

	userLabel := userUsername.String
	if userLabel == "" {
		if userID.Valid {
			userLabel = fmt.Sprintf("#%d", userID.Int64)
		} else {
			userLabel = "unknown user"
		}
	}

	statusMessage := fmt.Sprintf("success! User gear registration linking %s to user %s was deleted", gearLabel, userLabel)
	log.Infow("user gear registration deleted", "registration_id", urlParameter, "gear", gearLabel, "user", userLabel)
	c.JSON(http.StatusOK, map[string]string{"status": statusMessage})
}
