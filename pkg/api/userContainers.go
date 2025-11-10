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

// ListUserGearInContainer list all gear items registered to a user inside a container
//
// @Summary List all items inside a container
// @Description List all items inside a container registered to a user
// @Security 	BearerAuth
// @Tags			User container
// @Accept 		json
// @Produce 	json
// @Param			page										query		int		false	"Page number"				default(1)
// @Param			limit										query		int		false	"Number of items per page"	default(30)
// @Param			container	path		int				true	"Unique ID of userGear you want to update"
// @Success		200			{object}	models.ResponsePayload{items=[]models.FullGear}
// @Router /api/v1/container/{container}/list [get]
func ListUserGearInContainer(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	currentQueryParameters := c.Request.URL.Query()

	page := c.Query("page")
	limit := c.Query("limit")
	container := c.Param("container")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	log.Debugf("Request parameters: %#v", c.Request.URL.Query())

	if container == "" {
		log.Errorf("Error container was not supplied")
		c.IndentedJSON(http.StatusNoContent, models.Error{Error: "container supplied was not valid"})
		return
	}

	if limit == "" {
		limit = "30"
	}

	if page == "" || page == "0" {
		page = "1"
	}

	containerInt, err := strconv.Atoi(container)
	if err != nil {
		log.Errorf("Error setting container to int: %#v", err)
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
	log.Debugf("subject: %s", subject)

	// isAdmin := false
	// if adminAny, ok := c.Get("user_is_admin"); ok {
	// 	if adminFlag, ok := adminAny.(bool); ok {
	// 		isAdmin = adminFlag
	// 	}
	// }

	// if !isAdmin {
	// 	subjectID, parseErr := strconv.Atoi(subject)
	// 	if parseErr != nil || subjectID != containerInt {
	// 		log.Warnw("non-admin attempted to view other user's gear", "requested_user", containerInt, "subject", subject)
	// 		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "not allowed to view registrations for this user"})
	// 		return
	// 	}
	// }

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

	if container != "" {
		userIDQ := fmt.Sprintf("user_container_registration.userContainerId = %d", containerInt)
		conditions = append(conditions, userIDQ)
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

	baseCountQuery := "SELECT COUNT(*) FROM user_container_registration"
	countQuery := baseCountQuery + extraSQL + whereClause
	log.Debugf("countQuery: %s", countQuery)

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

	baseQuery := fmt.Sprintf(`SELECT %s FROM user_container_registration`, strings.Join(fields, ", "))

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

// InsertContainer puts gear on the users gear list
//
// @Summary		Insert user registered gear
// @Description	Insert user registered gear with corresponding values
// @Security		BearerAuth
// @Tags			User container
// @Accept			json
// @Produce		json
// @Param			request	body		models.UserContainerNoID	true	"query params"
// @Success		200		{object}	models.Status			"status: success when all goes well"
// @Router			/api/v1/container/insert [put]
func InsertContainer(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	_, err = utils.GenericInsert[models.UserContainer]("user_container_registration", data, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// DeleteContainerRegistration deletes a registered container gear item from users container gear
//
// @Summary		Delete user container content with ID
// @Description	Delete user container content with corresponding ID value
// @Security		BearerAuth
// @Tags			User container
// @Accept			json
// @Produce		json
// @Param			container	path		int				true	"Unique ID of userGear you want to update"
// @Success		200			{object}	models.Status	"status: success when all goes well"
// @Failure		default		{object}	models.Error
// @Router			/api/v1/container/{container}/delete [delete]
func DeleteContainerRegistration(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	function := "container"

	urlParameter, err := strconv.Atoi(c.Param(function))
	if err != nil {
		log.Errorf("urlParamter is of wrong type: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	_, err = utils.GenericDelete[models.UserContainer]("user_container_registration", urlParameter, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	log.Infof("success! User container gear registration linking was deleted")
	c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! User gear registration linking was deleted")})
}
