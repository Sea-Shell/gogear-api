package endpoints

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"

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
// Nesting rules (BR-020, D9, D11): a registration may hold others only when its
// gear is container-capable, which the gear's sub-category decides (BR-021; the
// view gear_container_capability is the one place that rule is computed); any
// registration may be content, at any depth. A registration has at most one
// direct container, and nothing is ever inside itself. The checks run in the
// same immediate transaction as the insert, so concurrent requests queue and
// cannot jointly break a rule, and a catalog change that removes a container's
// capability is ordered against them by SQLite's write lock (BR-021 AC-8). The
// database repeats the rules as a unique index on the content column, a cycle
// trigger (migration 000008) and capability triggers (migration 000009).

// capabilityTriggerMessage is the abort message of the capability triggers of
// migration 000009; the API maps it to errCannotHoldItems.
const capabilityTriggerMessage = "the container registration cannot hold items"

// errCannotHoldItems is the 409 for a link into a registration whose gear is
// not container-capable. It names only the caller's own registration.
var errCannotHoldItems = &opError{http.StatusConflict, "this registration cannot hold items: its gear is not in a container category"}

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
//	@Description	Put one of the caller's registrations inside another. Any registration can be content, at any depth, but only a registration whose gear is container-capable (its sub-category is a container category) can hold items; otherwise the request is refused with 409. A registration has at most one direct container and cannot end up inside itself: a second container or a cycle is refused with 409, a link to itself with 400, and a foreign or unknown registration with 404. Checks run in that order: 404, then 400 for a self-link, then the not-capable 409, then the other 409s.
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
		// Ownership of both registrations is decided before any 409 (BR-020
		// AC-5), so a conflict never reveals another user's links.
		var owned int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM user_gear_registrations WHERE userGearRegistrationId IN (?, ?) AND userId = ?`,
			containerID, contentID, callerID).Scan(&owned); err != nil {
			return err
		}
		if owned != 2 {
			return errRegistrationNotFound
		}

		// Only a registration whose gear is container-capable can hold items
		// (BR-020 AC-1, BR-021). This is decided after ownership (so the answer
		// says nothing about anyone else's registration) and before the other
		// 409s. The capability is read here, inside the immediate transaction,
		// through the view, so a concurrent catalog change is ordered before or
		// after this request (BR-021 AC-8, ADR-001 D-8). Content may be any
		// registration.
		var capable int
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE((SELECT cap.isContainer FROM user_gear_registrations r
			                  JOIN gear_container_capability cap ON cap.gearId = r.gearId
			                  WHERE r.userGearRegistrationId = ?), 0)`, containerID).Scan(&capable); err != nil {
			return err
		}
		if capable != 1 {
			return errCannotHoldItems
		}

		// At most one direct container (AC-3).
		var current int64
		switch err := tx.QueryRowContext(ctx,
			`SELECT userContainerId FROM user_container_registration WHERE userGearRegistrationId = ?`, contentID).Scan(&current); {
		case err == nil && current == containerID:
			return &opError{http.StatusConflict, "registration is already in this container"}
		case err == nil:
			return &opError{http.StatusConflict, "registration is already in another container; remove that link first"}
		case !isNoRows(err):
			return err
		}

		// No cycles (AC-4): the new container must not already be inside the
		// content, at any depth. UNION ends the walk even over a stored ring.
		var cycle int
		if err := tx.QueryRowContext(ctx,
			`WITH RECURSIVE up(id) AS (
				SELECT ?
				UNION
				SELECT l.userContainerId FROM user_container_registration l JOIN up ON l.userGearRegistrationId = up.id
			) SELECT COUNT(*) FROM up WHERE id = ?`, containerID, contentID).Scan(&cycle); err != nil {
			return err
		}
		if cycle > 0 {
			return &opError{http.StatusConflict, "a registration cannot be put inside one of its own contents"}
		}

		// The insert still binds the owner in its own statement (OC-001 G-2).
		res, err := tx.ExecContext(ctx,
			`INSERT INTO user_container_registration (userContainerId, userGearRegistrationId)
			 SELECT ?, ?
			 WHERE (SELECT COUNT(*) FROM user_gear_registrations
			        WHERE userGearRegistrationId IN (?, ?) AND userId = ?) = 2`,
			containerID, contentID, containerID, contentID, callerID)
		// The database's own guards: a unique violation is a second container
		// (or a repeat), a trigger abort is the capability rule or a cycle. The
		// checks above make all of them unreachable in one transaction; they
		// are the last line of defence.
		if isUniqueViolation(err) {
			return &opError{http.StatusConflict, "registration is already in a container"}
		}
		if isTriggerViolation(err) {
			if strings.Contains(err.Error(), capabilityTriggerMessage) {
				return errCannotHoldItems
			}
			return &opError{http.StatusConflict, "a registration cannot be put inside one of its own contents"}
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
