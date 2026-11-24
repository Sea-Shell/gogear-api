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
		where = append(where, userGearContainerExpr+" = 1")
	case "false":
		where = append(where, userGearContainerExpr+" = 0")
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	ctx := c.Request.Context()
	var total int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_gear_registrations ugr LEFT JOIN gear g ON g.gearId = ugr.gearId`+userGearCapabilityJoin+whereSQL, args...).Scan(&total); err != nil {
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
//	@Description	Insert user registered gear with corresponding values. closet_count (how many the owner has, a whole number of at least 1) defaults to 1. measured_weight (whole grams, at least 1) and measured_weight_source ("weighed" or "guess") are the owner's own unit weight; give both or neither. A value that breaks these rules is 400. Neither changes the catalog.
//	@Security		BearerAuth
//	@Tags			User gear
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.UserGearLinkNoID	true	"query params"
//	@Success		200		{object}	models.Status			"status: success when all goes well"
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
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
	// On insert a null new field is the same as an absent one.
	ctx := c.Request.Context()
	fields, err := parseRegistrationFields(data, false)
	if err != nil {
		// An unknown gear is 404 before a malformed field is 400 (ARS-001 section 1).
		var exists bool
		if qerr := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gear WHERE gearId = ?)`, body.GearID).Scan(&exists); qerr == nil && !exists {
			respondUserGearError(c, log, errGearNotFound)
			return
		}
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	// The owner is the caller. A registration ID in the body is not read.
	// Gear existence is enforced by the foreign key: a gear ID that is not in
	// the catalog is 404 (see gearReferenceError).
	closetCount := int64(1)
	if fields.closetCount != nil {
		closetCount = *fields.closetCount
	}
	var measured, source any // NULL unless the request gave both
	if fields.measured == measuredSet {
		measured, source = fields.measuredWeight, fields.measuredSource
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO user_gear_registrations (gearId, userId, maxContainerWeight, closetCount, measuredWeight, measuredWeightSource)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		body.GearID, callerID, body.MaxContainerWeight, closetCount, measured, source); err != nil {
		respondUserGearError(c, log, gearReferenceError(ctx, db, body.GearID, err))
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// UpdateUserGear updates a record of user registered gear
//
//	@Summary		Update user registered gear with ID
//	@Description	Update user registered gear identified by ID. Only the owner can; anyone else gets 404. Absent fields keep their stored value. max_container_weight, and measured_weight with measured_weight_source, are clearable: null removes them (the measured weight and its source only together, after which the catalog weight counts again). measured_weight is whole grams, at least 1, and is always sent with measured_weight_source ("weighed" or "guess"); one without the other is 400. closet_count is a whole number of at least 1. Using more than closet_count in loadouts is never refused. Changing any of these never changes the catalog or another registration.
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

	err = inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		ctx := c.Request.Context()
		// A foreign or unknown target is 404 before the body is read as
		// anything but bytes: a malformed or invalid body must not tell a
		// non-owner more than "not found" (BR-018 AC-4).
		owned, err := registrationOwned(ctx, tx, id, callerID)
		if err != nil {
			return err
		}
		if !owned {
			return errRegistrationNotFound
		}
		var body struct {
			RegistrationID     *int64 `json:"usergear_registration_id"`
			UserID             *int64 `json:"usergear_user_id"`
			MaxContainerWeight *int32 `json:"max_container_weight"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return badRequest(err.Error())
		}
		_, setWeight := raw["max_container_weight"]
		if body.RegistrationID != nil && *body.RegistrationID != id {
			return badRequest("request body registration ID does not match route parameter")
		}
		if body.UserID != nil && *body.UserID != callerID {
			return badRequest("usergear_user_id does not match the authenticated user")
		}
		fields, err := parseRegistrationFields(data, true)
		if err != nil {
			return badRequest(err.Error())
		}
		// Only the fields the request names change. The gear and the owner of
		// a registration never change, whatever the body says.
		var sets []string
		var args []any
		if setWeight {
			sets, args = append(sets, "maxContainerWeight = ?"), append(args, body.MaxContainerWeight)
		}
		switch fields.measured {
		case measuredSet:
			sets = append(sets, "measuredWeight = ?", "measuredWeightSource = ?")
			args = append(args, fields.measuredWeight, fields.measuredSource)
		case measuredCleared:
			sets = append(sets, "measuredWeight = NULL", "measuredWeightSource = NULL")
		}
		if fields.closetCount != nil {
			sets, args = append(sets, "closetCount = ?"), append(args, *fields.closetCount)
		}
		if len(sets) == 0 {
			return nil
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE user_gear_registrations SET `+strings.Join(sets, ", ")+` WHERE userGearRegistrationId = ? AND userId = ?`,
			append(args, id, callerID)...)
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
		// registration follow in the same transaction (ON DELETE CASCADE
		// would remove them too; the explicit delete keeps this correct on its own).
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

// measuredChange says what a request does to a registration's measured weight.
type measuredChange int

const (
	measuredUnchanged measuredChange = iota // neither field named
	measuredSet                             // both named with values
	measuredCleared                         // both named null (update only)
)

// registrationFields are the milestone B fields of a registration write that
// passed validation (BR-026 AC-1, BR-036 AC-1).
type registrationFields struct {
	measured       measuredChange
	measuredWeight int64  // set when measured == measuredSet
	measuredSource string // "weighed" or "guess"
	closetCount    *int64 // nil: not named
}

// parseRegistrationFields reads measured_weight, measured_weight_source and
// closet_count from a request body. A weight is a whole number of grams of at
// least 1, always with its source; one without the other is refused. clearable
// is the update rule: both null removes the measured weight. On insert a null
// field is the same as an absent one, as is a null closet_count everywhere
// (it is never cleared). A fraction, a string or a number that does not fit
// is refused, so the stored value is always exactly what was asked for.
func parseRegistrationFields(body []byte, clearable bool) (registrationFields, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return registrationFields{}, err
	}
	weight, hasWeight := raw["measured_weight"]
	source, hasSource := raw["measured_weight_source"]
	weightNull, sourceNull := isJSONNull(weight), isJSONNull(source)
	if !clearable {
		hasWeight, hasSource = hasWeight && !weightNull, hasSource && !sourceNull
	}

	var f registrationFields
	switch {
	case !hasWeight && !hasSource:
	case hasWeight != hasSource:
		return f, errors.New("measured_weight and measured_weight_source must be given together")
	case weightNull && sourceNull:
		f.measured = measuredCleared
	case weightNull || sourceNull:
		return f, errors.New("measured_weight and measured_weight_source must be given together")
	default:
		w, ok := wholeGrams(weight)
		if !ok {
			return f, errors.New("measured_weight must be a whole number of grams, at least 1")
		}
		var src string
		if err := json.Unmarshal(source, &src); err != nil || (src != "weighed" && src != "guess") {
			return f, errors.New(`measured_weight_source must be "weighed" or "guess"`)
		}
		f.measured, f.measuredWeight, f.measuredSource = measuredSet, w, src
	}

	if count, ok := raw["closet_count"]; ok && !isJSONNull(count) {
		n, valid := wholeGrams(count)
		if !valid {
			return f, errors.New("closet_count must be a whole number, at least 1")
		}
		f.closetCount = &n
	}
	return f, nil
}

// wholeGrams parses a JSON number written as a plain integer of at least 1.
func wholeGrams(raw json.RawMessage) (int64, bool) {
	n, err := strconv.ParseInt(string(raw), 10, 64)
	return n, err == nil && n >= 1
}

func isJSONNull(raw json.RawMessage) bool { return string(raw) == "null" }
