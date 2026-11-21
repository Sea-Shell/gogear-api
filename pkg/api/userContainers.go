package endpoints

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// Container links. A link says that one registration (the content) is inside
// another (the container). The link's owner is its container's owner; creating
// a link also needs the content to be owned by the caller (BR-013, OC-001).
// Ownership is bound in the same SQL statement that reads or changes the data,
// and a container, content or link that is unknown or not the caller's gives
// one identical 404 (BR-018).
//
// Not decided yet (open question Q-4), so not enforced: nesting depth, cycles
// longer than a self-link, duplicate links, and whether a registration may hold
// gear at all.

// ownedContainerExists is a SQL fragment that is true when its first ? (a
// registration ID) belongs to the user given by the second ?.
const ownedContainerExists = `EXISTS (SELECT 1 FROM user_gear_registrations WHERE userGearRegistrationId = ? AND userId = ?)`

// ListUserGearInContainer list all gear items registered to a user inside a container
//
//	@Summary		List all items inside a container
//	@Description	List all items inside a container registered to a user
//	@Security		BearerAuth
//	@Tags			User container
//	@Accept			json
//	@Produce		json
//	@Param			page		query		int	false	"Page number"				default(1)
//	@Param			limit		query		int	false	"Number of items per page"	default(30)
//	@Param			container	path		int	true	"Unique ID of userGear you want to update"
//	@Success		200			{object}	models.ResponsePayload{items=[]models.FullGear}
//	@Router			/api/v1/container/{container}/list [get]
func ListUserGearInContainer(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	containerID, ok := parseInt64Param(c, "container")
	if !ok {
		return
	}
	page, limit, ok := parsePaging(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	owned, err := registrationOwned(ctx, db, containerID, callerID)
	if err != nil {
		respondInternal(c, log, "check container owner", err)
		return
	}
	if !owned {
		c.IndentedJSON(errContainerNotFound.status, models.Error{Error: errContainerNotFound.msg})
		return
	}

	// Both sides are owner-scoped in the query itself: the container and every
	// content registration must belong to the caller, so a legacy link to
	// someone else's registration is never returned.
	const from = ` FROM user_container_registration l
		JOIN user_gear_registrations ugr ON ugr.userGearRegistrationId = l.userGearRegistrationId AND ugr.userId = ?`
	const where = ` WHERE l.userContainerId = ? AND ` + ownedContainerExists

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*)`+from+where,
		callerID, containerID, containerID, callerID).Scan(&total); err != nil {
		respondInternal(c, log, "count container contents", err)
		return
	}

	rows, err := db.QueryContext(ctx,
		`SELECT `+userGearColumns+from+userGearCatalogJoins+where+` ORDER BY l.containerRegistrationId LIMIT ? OFFSET ?`,
		callerID, containerID, containerID, callerID, limit, int64(page-1)*int64(limit))
	if err != nil {
		respondInternal(c, log, "list container contents", err)
		return
	}
	items, err := scanUserGearRows(rows)
	if err != nil {
		respondInternal(c, log, "list container contents", err)
		return
	}

	c.IndentedJSON(http.StatusOK, pagePayload(c, items, total, page, limit))
}

// InsertContainer puts gear on the users gear list
//
//	@Summary		Insert user registered gear
//	@Description	Insert user registered gear with corresponding values
//	@Security		BearerAuth
//	@Tags			User container
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.UserContainerNoID	true	"query params"
//	@Success		200		{object}	models.Status				"status: success when all goes well"
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		409		{object}	models.Error
//	@Router			/api/v1/container/insert [put]
func InsertContainer(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondInternal(c, log, "read body", err)
		return
	}
	var body struct {
		ContainerID *int64 `json:"user_container_id"`
		ContentID   *int64 `json:"user_gear_registration_id"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	if body.ContainerID == nil || body.ContentID == nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "user_container_id and user_gear_registration_id are required"})
		return
	}
	containerID, contentID := *body.ContainerID, *body.ContentID

	err = inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		ctx := c.Request.Context()
		if containerID == contentID {
			// A registration cannot be inside itself. Whether the caller owns it
			// is checked first, so a foreign ID gives the same 404 as any other.
			owned, err := registrationOwned(ctx, tx, containerID, callerID)
			if err != nil {
				return err
			}
			if !owned {
				return errRegistrationNotFound
			}
			return badRequest("a registration cannot be contained in itself")
		}
		// One statement inserts only if the caller owns both registrations.
		res, err := tx.ExecContext(ctx,
			`INSERT INTO user_container_registration (userContainerId, userGearRegistrationId)
			 SELECT ?, ?
			 WHERE (SELECT COUNT(*) FROM user_gear_registrations
			        WHERE userGearRegistrationId IN (?, ?) AND userId = ?) = 2`,
			containerID, contentID, containerID, contentID, callerID)
		if isUniqueViolation(err) {
			return &opError{http.StatusConflict, "registration is already in this container"}
		}
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

// DeleteContainerRegistration deletes a registered container gear item from users container gear
//
//	@Summary		Delete user container content with ID
//	@Description	Delete user container content with corresponding ID value
//	@Security		BearerAuth
//	@Tags			User container
//	@Accept			json
//	@Produce		json
//	@Param			container	path		int				true	"Unique ID of userGear you want to update"
//	@Success		200			{object}	models.Status	"status: success when all goes well"
//	@Failure		default		{object}	models.Error
//	@Router			/api/v1/container/{container}/delete [delete]
func DeleteContainerRegistration(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	callerID := c.MustGet("user_id_int64").(int64)

	linkID, ok := parseInt64Param(c, "container")
	if !ok {
		return
	}

	// The link goes only if its container registration belongs to the caller.
	res, err := db.ExecContext(c.Request.Context(),
		`DELETE FROM user_container_registration
		 WHERE containerRegistrationId = ?
		   AND userContainerId IN (SELECT userGearRegistrationId FROM user_gear_registrations WHERE userId = ?)`,
		linkID, callerID)
	if err != nil {
		respondInternal(c, log, "delete container link", err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		respondInternal(c, log, "delete container link", err)
		return
	}
	if n != 1 {
		c.IndentedJSON(errLinkNotFound.status, models.Error{Error: errLinkNotFound.msg})
		return
	}

	log.Infow("container link deleted", "link_id", linkID)
	c.JSON(http.StatusOK, map[string]string{"status": "success! User gear registration linking was deleted"})
}
