package endpoints

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// Gear registrations (inventory). Every operation is scoped to the
// authenticated user, taken from the verified token and never from the body or
// a query parameter, and the owner is bound in the same SQL statement that
// reads or changes the data (BR-012, OC-001). A registration that is unknown or
// owned by someone else gives one identical 404 (BR-018). A conflicting owner or
// registration ID in a body is 400 (BR-017). Admins get no exception (P-3).

// ListUserGear list the users registered gear
//
//	@Summary		List users gear
//	@Description	Get a list a users gear
//	@Security		BearerAuth
//	@Tags			User gear
//	@Accept			json
//	@Produce		json
//	@Param			user		path		int		true	"Unique ID of user you want to get the Gear of"
//	@Param			page		query		int		false	"Page number"													default(1)
//	@Param			limit		query		int		false	"Number of items per page"										default(30)
//	@Param			topCategory	query		[]int	false	"top categories"												collectionFormat(multi)
//	@Param			category	query		[]int	false	"sub categories"												collectionFormat(multi)
//	@Param			manufacture	query		[]int	false	"manufacturers"													collectionFormat(multi)
//	@Param			container	query		string	false	"show container gear only. valid values are true, false, all"	default(all)
//	@Success		200			{object}	models.ResponsePayload{items=[]models.UserGear}
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/usergear/{user}/list [get]
func ListUserGear(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	requested, err := strconv.ParseInt(c.Param("user"), 10, 64)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "invalid user ID"})
		return
	}
	if requested != callerID {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "not allowed to view registrations for this user"})
		return
	}
	page, limit, ok := parsePaging(c)
	if !ok {
		return
	}

	// Filters are parsed as integers and bound; a value that is not an integer
	// is ignored, so it can never change the meaning of the query.
	where := []string{"ugr.userId = ?"}
	args := []any{callerID}
	addIntFilters := func(column string, values []string) {
		for _, v := range values {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				where = append(where, column+" = ?")
				args = append(args, n)
			}
		}
	}
	addIntFilters("g.gearTopCategoryId", c.QueryArray("topCategory"))
	addIntFilters("g.gearCategoryId", c.QueryArray("category"))
	addIntFilters("g.gearManufactureId", c.QueryArray("manufacture"))
	switch c.Query("container") {
	case "true":
		where = append(where, "g.gearIsContainer = 1")
	case "false":
		where = append(where, "g.gearIsContainer = 0")
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	ctx := c.Request.Context()
	var total int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_gear_registrations ugr LEFT JOIN gear g ON g.gearId = ugr.gearId`+whereSQL, args...).Scan(&total); err != nil {
		respondInternal(c, log, "count registrations", err)
		return
	}

	rows, err := db.QueryContext(ctx,
		`SELECT `+userGearColumns+` FROM user_gear_registrations ugr`+userGearCatalogJoins+userGearOwnLinkJoin+
			whereSQL+` ORDER BY ugr.userGearRegistrationId LIMIT ? OFFSET ?`,
		append(args, limit, int64(page-1)*int64(limit))...)
	if err != nil {
		respondInternal(c, log, "list registrations", err)
		return
	}
	items, err := scanUserGearRows(rows)
	if err != nil {
		respondInternal(c, log, "list registrations", err)
		return
	}

	c.IndentedJSON(http.StatusOK, pagePayload(c, items, total, page, limit))
}

// GetUserGear retrives the users full gear list
//
//	@Summary		Get user registered gear with ID
//	@Description	Get user registeredgear spessific to ID
//	@Security		BearerAuth
//	@Tags			User gear
//	@Accept			json
//	@Produce		json
//	@Param			usergear	path		int				true	"Unique ID of user registered gear you want to get"
//	@Success		200			{object}	models.UserGear	"desc"
//	@Router			/api/v1/usergear/registration/{usergear}/get [get]
func GetUserGear(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	id, ok := parseInt64Param(c, "usergear")
	if !ok {
		return
	}

	// No query-string filtering on a single record: the record is selected by
	// path ID and owner only.
	reg, err := scanUserGear(db.QueryRowContext(c.Request.Context(),
		`SELECT `+userGearColumns+` FROM user_gear_registrations ugr`+userGearCatalogJoins+userGearOwnLinkJoin+
			` WHERE ugr.userGearRegistrationId = ? AND ugr.userId = ?`, id, callerID))
	if isNoRows(err) {
		c.IndentedJSON(errRegistrationNotFound.status, models.Error{Error: errRegistrationNotFound.msg})
		return
	}
	if err != nil {
		respondInternal(c, log, "get registration", err)
		return
	}

	c.IndentedJSON(http.StatusOK, reg)
}

// InsertUserGear puts gear on the users gear list
//
//	@Summary		Insert user registered gear
//	@Description	Insert user registered gear with corresponding values
//	@Security		BearerAuth
//	@Tags			User gear
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.UserGearLinkNoID	true	"query params"
//	@Success		200		{object}	models.Status			"status: success when all goes well"
//	@Router			/api/v1/usergear/insert [put]
func InsertUserGear(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondInternal(c, log, "read body", err)
		return
	}
	var body struct {
		GearID             int64  `json:"usergear_gear_id"`
		UserID             *int64 `json:"usergear_user_id"`
		MaxContainerWeight *int32 `json:"max_container_weight"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	if body.UserID != nil && *body.UserID != callerID {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "usergear_user_id does not match the authenticated user"})
		return
	}

	// The owner is the caller. A registration ID in the body is not read.
	if _, err := db.ExecContext(c.Request.Context(),
		`INSERT INTO user_gear_registrations (gearId, userId, maxContainerWeight) VALUES (?, ?, ?)`,
		body.GearID, callerID, body.MaxContainerWeight); err != nil {
		respondInternal(c, log, "insert registration", err)
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// UpdateUserGear updates a record of user registered gear
//
//	@Summary		Update user registered gear with ID
//	@Description	Update user registered gear identified by ID
//	@Security		BearerAuth
//	@Tags			User gear
//	@Accept			json
//	@Produce		json
//	@Param			usergear	path		int					true	"Unique ID of user registered gear you want to get"
//	@Param			request		body		models.UserGearLink	true	"query params"
//	@Success		200			{object}	models.Status		"status: success when all goes well"
//	@Router			/api/v1/usergear/registration/{usergear}/update [post]
func UpdateUserGear(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	id, ok := parseInt64Param(c, "usergear")
	if !ok {
		return
	}
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondInternal(c, log, "read body", err)
		return
	}
	var body struct {
		RegistrationID     *int64 `json:"usergear_registration_id"`
		UserID             *int64 `json:"usergear_user_id"`
		MaxContainerWeight *int32 `json:"max_container_weight"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	_, setWeight := raw["max_container_weight"]

	// Only the maximum container weight is updatable. The gear and the owner
	// of a registration never change, whatever the body says.
	err = inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		ctx := c.Request.Context()
		// A foreign or unknown target is 404 before any body conflict is reported.
		owned, err := registrationOwned(ctx, tx, id, callerID)
		if err != nil {
			return err
		}
		if !owned {
			return errRegistrationNotFound
		}
		if body.RegistrationID != nil && *body.RegistrationID != id {
			return badRequest("request body registration ID does not match route parameter")
		}
		if body.UserID != nil && *body.UserID != callerID {
			return badRequest("usergear_user_id does not match the authenticated user")
		}
		if !setWeight {
			return nil
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE user_gear_registrations SET maxContainerWeight = ? WHERE userGearRegistrationId = ? AND userId = ?`,
			body.MaxContainerWeight, id, callerID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return errRegistrationNotFound
		}
		return nil
	})
	if err != nil {
		respondUserGearError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// DeleteUserGearRegistration deletes a registered gear item from users list
//
//	@Summary		Delete userGear with ID
//	@Description	Delete userGear with corresponding ID value
//	@Security		BearerAuth
//	@Tags			User gear
//	@Accept			json
//	@Produce		json
//	@Param			userGear	path		int				true	"Unique ID of userGear you want to update"
//	@Success		200			{object}	models.Status	"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/usergear/registration/{usergear}/delete [delete]
func DeleteUserGearRegistration(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	id, ok := parseInt64Param(c, "usergear")
	if !ok {
		return
	}

	var gearName, username sql.NullString
	err := inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		ctx := c.Request.Context()
		err := tx.QueryRowContext(ctx,
			`SELECT g.gearName, u.userUsername
			 FROM user_gear_registrations ugr
			 LEFT JOIN gear g ON g.gearId = ugr.gearId
			 LEFT JOIN users u ON u.userId = ugr.userId
			 WHERE ugr.userGearRegistrationId = ? AND ugr.userId = ?`, id, callerID).Scan(&gearName, &username)
		if isNoRows(err) {
			return errRegistrationNotFound
		}
		if err != nil {
			return err
		}
		// The registration goes first, bound to its owner; if nothing was
		// deleted no container link is touched. The links of the deleted
		// registration follow in the same transaction (foreign keys are not
		// enforced).
		res, err := tx.ExecContext(ctx,
			`DELETE FROM user_gear_registrations WHERE userGearRegistrationId = ? AND userId = ?`, id, callerID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return errRegistrationNotFound
		}
		_, err = tx.ExecContext(ctx,
			`DELETE FROM user_container_registration WHERE userGearRegistrationId = ? OR userContainerId = ?`, id, id)
		return err
	})
	if err != nil {
		respondUserGearError(c, log, err)
		return
	}

	gearLabel := gearName.String
	if gearLabel == "" {
		gearLabel = fmt.Sprintf("registration #%d", id)
	}
	userLabel := username.String
	if userLabel == "" {
		userLabel = fmt.Sprintf("#%d", callerID)
	}
	log.Infow("user gear registration deleted", "registration_id", id, "gear", gearLabel, "user", userLabel)
	c.JSON(http.StatusOK, map[string]string{"status": fmt.Sprintf("success! User gear registration linking %s to user %s was deleted", gearLabel, userLabel)})
}

// respondUserGearError writes the response for an error from inTx: a refusal
// keeps its status, anything else is a logged 500 with a generic body.
func respondUserGearError(c *gin.Context, log interface{ Errorf(string, ...any) }, err error) {
	var oe *opError
	if errors.As(err, &oe) {
		c.IndentedJSON(oe.status, models.Error{Error: oe.msg})
		return
	}
	respondInternal(c, log, "registration operation failed", err)
}
