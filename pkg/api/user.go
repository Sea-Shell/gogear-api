package endpoints

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
//	@Summary		List user
//	@Description	Get a list of user items
//	@Security		BearerAuth
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			page		query		int		false	"Page number"				default(1)
//	@Param			limit		query		int		false	"Number of items per page"	default(30)
//	@Param			user		query		string	false	"search by user's username (this is case insensitive and wildcard)"
//	@Param			username	query		string	false	"search by users full name (this is case insensitive and wildcard)"
//	@Param			email		query		string	false	"search by users email (this is case insensitive and wildcard)"
//	@Success		200			{object}	models.ResponsePayload{items=[]models.User}
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/users/list [get]
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

	callerID, callerIsAdmin := callerIdentity(c)
	if len(qUserEmail) > 0 && !callerIsAdmin {
		// An email search is an oracle for other users' addresses (BR-007).
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "admin privileges required to search by email"})
		return
	}

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
	// Google-created accounts use the email as username and name. Non-admins
	// must not be able to match (or see) such values on other users, so the
	// search applies to exactly what the response will show (see
	// emailLike and hiddenFromOthers, which must agree).
	for _, username := range qUserUsername {
		if callerIsAdmin {
			conditions = append(conditions, "userUsername LIKE ?")
			params = append(params, "%"+username+"%")
		} else {
			conditions = append(conditions, "(userUsername LIKE ? AND (userId = ? OR "+hiddenFromOthers("userUsername")+" = 0))")
			params = append(params, "%"+username+"%", callerID)
		}
	}

	for _, name := range qUserName {
		if callerIsAdmin {
			conditions = append(conditions, "userName LIKE ?")
			params = append(params, "%"+name+"%")
		} else {
			conditions = append(conditions, "(userName LIKE ? AND (userId = ? OR "+hiddenFromOthers("userName")+" = 0))")
			params = append(params, "%"+name+"%", callerID)
		}
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

	users := []userView{}

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

		users = append(users, viewUserFor(paramUser, callerID, callerIsAdmin))
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
//	@Summary		Get user with ID
//	@Description	Get user spessific to ID
//	@Security		BearerAuth
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			user	path		int			true	"Unique ID of user you want to get"
//	@Success		200		{object}	models.User	"desc"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/users/{user}/get [get]
func GetUser(c *gin.Context) {
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

	callerID, callerIsAdmin := callerIdentity(c)

	var extraSQL []string
	// extraSQL = append(extraSQL, " LEFT JOIN manufacture ON gear.gearManufactureId = manufacture.manufactureId ")
	// extraSQL = append(extraSQL, " LEFT JOIN gear_top_category ON gear.gearTopCategoryId = gear_top_category.topCategoryId ")
	// extraSQL = append(extraSQL, "  LEFT JOIN gear_category ON gear.gearCategoryId = gear_category.categoryId ")

	results, err := utils.GenericGet[models.User]("users", urlParameter, extraSQL, db)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.IndentedJSON(http.StatusNotFound, models.Error{Error: "user not found"})
			return
		}
		log.Errorf("Unable to get %s with id: %d. Error: %#v", function, urlParameter, err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	log.Infof("Successfully fetched %s with ID %d", function, urlParameter)
	c.IndentedJSON(http.StatusOK, viewUserFor(*results, callerID, callerIsAdmin))
}

// userView is the response shape for a user profile. Email and admin status
// are pointers with omitempty so that they are absent, not just blank, when
// the caller may not see them (BR-007). The password is never part of it.
type userView struct {
	UserID       *int64  `json:"user_id"`
	UserUsername string  `json:"user_username"`
	UserName     string  `json:"user_name"`
	UserEmail    *string `json:"user_email,omitempty"`
	UserIsAdmin  *bool   `json:"user_is_admin,omitempty"`
}

// callerIdentity returns the authenticated user and admin role set by the JWT middleware.
func callerIdentity(c *gin.Context) (id int64, isAdmin bool) {
	if v, ok := c.Get("user_id_int64"); ok {
		id, _ = v.(int64)
	}
	if v, ok := c.Get("user_is_admin"); ok {
		isAdmin, _ = v.(bool)
	}
	return id, isAdmin
}

// emailLike reports whether value must be hidden from other users: it
// contains '@', so it looks like an email address. Google sign-in uses the
// email for both username and name, and a value that embeds a real address
// always contains '@', so the check needs no comparison with the stored
// email. Such a comparison would need the same case folding in Go and SQL
// (SQLite's lower() folds ASCII only), and the two disagreed for non-ASCII
// text.
func emailLike(value string) bool {
	return strings.Contains(value, "@")
}

// hiddenFromOthers is the SQL form of emailLike for a users column: true when
// the column value is hidden from other users. The user search uses it so a
// search cannot match what the response redacts. Both sides test the same
// byte, so they cannot disagree.
func hiddenFromOthers(column string) string {
	return "(instr(" + column + ", '@') > 0)"
}

// viewUserFor builds the profile response for a caller. Admins and the user
// themself see email and admin status. For anyone else, a username or name that
// looks like an email address is blanked.
func viewUserFor(u models.User, callerID int64, callerIsAdmin bool) userView {
	v := userView{UserID: u.UserID, UserUsername: u.UserUsername, UserName: u.UserName}
	if callerIsAdmin || (u.UserID != nil && *u.UserID == callerID) {
		email, admin := u.UserEmail, u.UserIsAdmin
		v.UserEmail, v.UserIsAdmin = &email, &admin
		return v
	}
	if emailLike(v.UserUsername) {
		v.UserUsername = ""
	}
	if emailLike(v.UserName) {
		v.UserName = ""
	}
	return v
}

// userWriteBody is the request body of user create and update. Pointers tell
// "absent" from "zero". UserID and UserIsAdmin are only checked against the
// values the API determined (BR-017); they are never applied.
type userWriteBody struct {
	UserID       *int64  `json:"user_id"`
	UserUsername *string `json:"user_username"`
	UserName     *string `json:"user_name"`
	UserEmail    *string `json:"user_email"`
	UserIsAdmin  *bool   `json:"user_is_admin"`
}

func (b *userWriteBody) blankIdentityField() bool {
	return (b.UserUsername != nil && strings.TrimSpace(*b.UserUsername) == "") ||
		(b.UserEmail != nil && strings.TrimSpace(*b.UserEmail) == "")
}

func parseUserID(c *gin.Context, log *zap.SugaredLogger) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("user"), 10, 64)
	if err != nil {
		log.Errorf("urlParamter is of wrong type: %#v", err)
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "invalid user ID"})
		return 0, false
	}
	return id, true
}

var (
	errUserNotFound   = &opError{http.StatusNotFound, "user not found"}
	errPermanentAdmin = &opError{http.StatusForbidden, "the permanent admin cannot lose admin status or be deleted"}
)

// isPermanentAdmin reports whether userID is the permanent admin (BR-019).
func isPermanentAdmin(ctx context.Context, q sqlRunner, userID int64) (bool, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM permanent_admin WHERE userId = ?`, userID).Scan(&n); err != nil {
		return false, fmt.Errorf("check permanent admin: %w", err)
	}
	return n > 0, nil
}

func internalError(c *gin.Context, log *zap.SugaredLogger, err error) {
	log.Error(err.Error())
	c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
}

// InsertUser creates new user in the database
//
//	@Summary		Insert new user
//	@Description	Insert new user with corresponding values. Admin only. The account is never an admin (use the admin-status operation), and a body that asks for admin status or names a user ID is rejected.
//	@Security		BearerAuth
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.User		true	"query params"	test
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/users/insert [put]
func InsertUser(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	if _, isAdmin := callerIdentity(c); !isAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "admin privileges required"})
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		internalError(c, log, err)
		return
	}

	var body userWriteBody
	if err := json.Unmarshal(data, &body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "invalid request body"})
		return
	}
	if body.UserID != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "user_id is assigned by the API and must not be sent"})
		return
	}
	if body.UserIsAdmin != nil && *body.UserIsAdmin {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "admin status cannot be set when creating a user"})
		return
	}
	if body.UserUsername == nil || body.UserEmail == nil || body.blankIdentityField() {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "user_username and user_email are required"})
		return
	}
	name := ""
	if body.UserName != nil {
		name = *body.UserName
	}
	email := normalizeEmail(*body.UserEmail)

	// Always non-admin and without password; admin status has its own operation (BR-010, BR-011).
	if _, err := db.ExecContext(c.Request.Context(),
		`INSERT INTO users (userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal) VALUES (?, '', ?, ?, 0, 0)`,
		*body.UserUsername, name, email); err != nil {
		internalError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// UpdateUser updates user in database
//
//	@Summary		Update user with ID
//	@Description	Update the caller's own profile. Other accounts are refused with 403, admins included. Omitted fields are left unchanged. Admin status and user ID in the body must match the stored values or the request is rejected with 400.
//	@Security		BearerAuth
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			user	path		int				true	"Unique ID of user you want to update"
//	@Param			request	body		models.User		true	"query params"	test
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/users/{user}/update [post]
func UpdateUser(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	targetID, ok := parseUserID(c, log)
	if !ok {
		return
	}
	callerID, _ := callerIdentity(c)
	if targetID != callerID {
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "you can only update your own account"})
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		internalError(c, log, err)
		return
	}
	var body userWriteBody
	if err := json.Unmarshal(data, &body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "invalid request body"})
		return
	}
	if body.UserID != nil && *body.UserID != targetID {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "user_id in body does not match the path"})
		return
	}
	if body.blankIdentityField() {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "user_username and user_email must not be blank"})
		return
	}

	// Stored emails are trimmed (BR-004 AC-4); the edited-flag comparison below
	// sees the value that will be stored.
	if body.UserEmail != nil {
		trimmed := normalizeEmail(*body.UserEmail)
		body.UserEmail = &trimmed
	}

	ctx := c.Request.Context()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, log, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	var storedAdmin bool
	if err := tx.QueryRowContext(ctx, `SELECT userIsAdmin <> 0 FROM users WHERE userId = ?`, targetID).Scan(&storedAdmin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusNotFound, models.Error{Error: "user not found"})
			return
		}
		internalError(c, log, err)
		return
	}
	if body.UserIsAdmin != nil && *body.UserIsAdmin != storedAdmin {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "admin status cannot be changed through profile update"})
		return
	}

	// userPassword and userIsAdmin are deliberately not in the SET list.
	// Changing the email marks the account as never eligible for the Google
	// subject backfill (BR-004): the email is no longer one the account was
	// created with, so it cannot vouch for a Google identity.
	res, err := tx.ExecContext(ctx,
		`UPDATE users SET userUsername = COALESCE(?, userUsername), userName = COALESCE(?, userName), userEmail = COALESCE(?, userEmail),
		userEmailEdited = CASE WHEN ? IS NOT NULL AND ? <> userEmail THEN 1 ELSE userEmailEdited END WHERE userId = ?`,
		body.UserUsername, body.UserName, body.UserEmail, body.UserEmail, body.UserEmail, targetID)
	if err != nil {
		internalError(c, log, err)
		return
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err == nil {
			err = fmt.Errorf("update of user %d affected %d rows", targetID, n)
		}
		internalError(c, log, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// SetUserAdmin grants or revokes a user's admin status
//
//	@Summary		Set admin status of user with ID
//	@Description	Grant or revoke admin status. Admin only. New tokens reflect the change at the next sign-in or refresh. The permanent admin (the first admin) can never lose admin status: revoking it is refused with 403 for every caller, and granting it is a no-op success. Any other admin may be revoked, including by themselves.
//	@Security		BearerAuth
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			user	path		int				true	"Unique ID of the user"
//	@Param			request	body		map[string]bool	true	"new admin status"
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/users/{user}/admin [post]
func SetUserAdmin(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	actorID, isAdmin := callerIdentity(c)
	if !isAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "admin privileges required"})
		return
	}
	targetID, ok := parseUserID(c, log)
	if !ok {
		return
	}

	var body struct {
		UserIsAdmin *bool `json:"user_is_admin"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.UserIsAdmin == nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "body must be {\"user_is_admin\": true|false}"})
		return
	}

	// The permanent-admin check and the change are one transaction (the
	// database takes the write lock at begin), so a concurrent request cannot
	// slip a revocation in between (BR-019).
	err := inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		ctx := c.Request.Context()
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE userId = ?`, targetID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errUserNotFound
		}
		if !*body.UserIsAdmin {
			permanent, err := isPermanentAdmin(ctx, tx, targetID)
			if err != nil {
				return err
			}
			if permanent {
				return errPermanentAdmin
			}
		}
		// Granting the permanent admin status it already has is a no-op.
		res, err := tx.ExecContext(ctx, `UPDATE users SET userIsAdmin = ? WHERE userId = ?`, *body.UserIsAdmin, targetID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return fmt.Errorf("admin status update of user %d affected %d rows", targetID, n)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errPermanentAdmin) {
			log.Warnw("refused to revoke the permanent admin", "actor_user_id", actorID, "target_user_id", targetID)
		}
		respondUserGearError(c, log, err)
		return
	}

	log.Infow("admin status changed", "actor_user_id", actorID, "target_user_id", targetID, "user_is_admin", *body.UserIsAdmin)
	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// ClearUserEmailEdited clears the edited-email mark of a user
//
//	@Summary		Clear the edited-email mark of user with ID
//	@Description	Admin only. Removes the mark that stops an unbound account from being bound to a Google identity by its email (BR-004). Nothing else about the account changes. Clearing an account without the mark succeeds. Clear it only after confirming the account's email belongs to its owner.
//	@Security		BearerAuth
//	@Tags			User
//	@Produce		json
//	@Param			user	path		int				true	"Unique ID of the user"
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/users/{user}/email-edited/clear [post]
func ClearUserEmailEdited(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	actorID, isAdmin := callerIdentity(c)
	if !isAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "admin privileges required"})
		return
	}
	targetID, ok := parseUserID(c, log)
	if !ok {
		return
	}

	// Only the mark is in the SET list. SQLite counts a matched row even when
	// the value does not change, so 0 rows means the user does not exist.
	res, err := db.ExecContext(c.Request.Context(), `UPDATE users SET userEmailEdited = 0 WHERE userId = ?`, targetID)
	if err != nil {
		internalError(c, log, err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		internalError(c, log, err)
		return
	}
	if n == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, models.Error{Error: "user not found"})
		return
	}

	log.Infow("edited-email mark cleared", "actor_user_id", actorID, "target_user_id", targetID)
	c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// DeleteUser delets user from database
//
//	@Summary		Delete user with ID
//	@Description	Delete the caller's own account together with all of its registrations, container links, loadouts and loadout items, all or nothing. Other accounts are refused with 403, admins included.
//	@Security		BearerAuth
//	@Tags			User
//	@Accept			json
//	@Produce		json
//	@Param			user	path		int				true	"Unique ID of user you want to update"
//	@Success		200		{object}	models.Status	"status: success when all goes well"
//	@Failure		default	{object}	models.Error
//	@Router			/api/v1/users/{user}/delete [delete]
func DeleteUser(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	targetID, ok := parseUserID(c, log)
	if !ok {
		return
	}
	callerID, _ := callerIdentity(c)
	if targetID != callerID {
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "you can only delete your own account"})
		return
	}

	ctx := c.Request.Context()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, log, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	var username string
	if err := tx.QueryRowContext(ctx, `SELECT userUsername FROM users WHERE userId = ?`, targetID).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusNotFound, models.Error{Error: "user not found"})
			return
		}
		internalError(c, log, err)
		return
	}

	// The permanent admin is never deleted through the API, not even by
	// themselves (BR-019 AC-5). The check shares the transaction with the delete.
	if permanent, err := isPermanentAdmin(ctx, tx, targetID); err != nil {
		internalError(c, log, err)
		return
	} else if permanent {
		log.Warnw("refused to delete the permanent admin", "actor_user_id", callerID, "target_user_id", targetID)
		c.AbortWithStatusJSON(errPermanentAdmin.status, models.Error{Error: errPermanentAdmin.msg})
		return
	}

	// Dependent rows are removed explicitly, children before parents: with
	// foreign keys enforced, the registrations and loadouts of the user would
	// otherwise block the final delete.
	const ownedRegistrations = `SELECT userGearRegistrationId FROM user_gear_registrations WHERE userId = ?`
	steps := []struct{ name, query string }{
		{"container links", `DELETE FROM user_container_registration WHERE userContainerId IN (` + ownedRegistrations + `) OR userGearRegistrationId IN (` + ownedRegistrations + `)`},
		{"loadout items", `DELETE FROM loadout_items WHERE loadoutId IN (SELECT loadoutId FROM loadouts WHERE userId = ?)`},
		{"loadouts", `DELETE FROM loadouts WHERE userId = ?`},
		{"gear registrations", `DELETE FROM user_gear_registrations WHERE userId = ?`},
	}
	for _, step := range steps {
		args := []any{targetID}
		if step.name == "container links" {
			args = append(args, targetID)
		}
		if _, err := tx.ExecContext(ctx, step.query, args...); err != nil {
			internalError(c, log, fmt.Errorf("deleting %s of user %d: %w", step.name, targetID, err))
			return
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE userId = ?`, targetID)
	if err != nil {
		internalError(c, log, fmt.Errorf("deleting user %d: %w", targetID, err))
		return
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err == nil {
			err = fmt.Errorf("delete of user %d affected %d rows", targetID, n)
		}
		internalError(c, log, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, log, err)
		return
	}

	msg := fmt.Sprintf("success! User %s (ID: %d) and all the users data was deleted", username, targetID)
	log.Info(msg)
	c.JSON(http.StatusOK, map[string]string{"status": msg})
}
