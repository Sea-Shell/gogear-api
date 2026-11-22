package endpoints

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// Every write below binds the loadout ID, the owner (the authenticated
// subject) and any parent in the same SQL statement, and checks RowsAffected
// (OC-001 G-2, G-4). Owner, target and parent values in a request body never
// take effect; a conflicting value is refused with 400 (BR-017). A loadout that
// is unknown, foreign or wrongly addressed gives one identical 404 (BR-018).

// parseLoadoutIDParam reads the :loadout path value. A malformed ID is 400
// before any ownership check, which reveals nothing about stored records.
func parseLoadoutIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "invalid " + name + " ID"})
		return 0, false
	}
	return id, true
}

// InsertLoadout creates a new loadout for the authenticated user.
//
//	@Summary		Insert loadout
//	@Description	Create a new loadout for the authenticated user. The owner is always the caller; a different user_id in the body is refused with 400. A blank slug is stored as no slug.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.LoadoutNoID	true	"Loadout data"
//	@Success		201		{object}	models.Loadout
//	@Failure		400		{object}	models.Error
//	@Failure		409		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/insert [put]
func InsertLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Errorf("read body: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
		return
	}

	var body struct {
		UserID      *int64 `json:"user_id"`
		Name        string `json:"loadout_name"`
		Description string `json:"loadout_description"`
		IsPublic    bool   `json:"loadout_is_public"`
		Slug        string `json:"loadout_slug"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	if body.UserID != nil && *body.UserID != userID {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "user_id does not match the authenticated user"})
		return
	}

	var created *models.Loadout
	err = inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(c.Request.Context(),
			`INSERT INTO loadouts (userId, loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug) VALUES (?, ?, ?, ?, ?)`,
			userID, body.Name, body.Description, body.IsPublic, nullableSlug(body.Slug))
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		created, err = LoadoutGetOwned(c.Request.Context(), tx, id, userID)
		return err
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.IndentedJSON(http.StatusCreated, created)
}

// ListLoadouts returns all loadouts for the authenticated user.
//
//	@Summary		List loadouts
//	@Description	Get all loadouts for the authenticated user
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Success		200	{array}		models.Loadout
//	@Failure		500	{object}	models.Error
//	@Router			/api/v1/loadout/list [get]
func ListLoadouts(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	results, err := LoadoutListByUser(db, userID)
	if err != nil {
		log.Errorf("Error listing loadouts: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
		return
	}

	c.IndentedJSON(http.StatusOK, results)
}

// GetLoadout returns a single loadout by ID.
//
//	@Summary		Get loadout
//	@Description	Get a single loadout by ID. Unknown and other users' loadouts both return 404.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Success		200		{object}	models.Loadout
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/get [get]
func GetLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}

	loadout, err := LoadoutGetOwned(c.Request.Context(), db, loadoutID, userID)
	if err != nil {
		respondOpError(c, log, err)
		return
	}
	if loadout == nil {
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: errLoadoutNotFound.msg})
		return
	}

	c.IndentedJSON(http.StatusOK, loadout)
}

// GetLoadoutFull returns a loadout with its places and items.
//
//	@Summary		Get loadout board
//	@Description	Get one loadout of the caller with all its places (Worn, Last minute, then bags by position) and all its items (by item ID), read from one transaction so the three parts describe one state. Unknown and other users' loadouts both return 404.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Success		200		{object}	models.LoadoutFull
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/full [get]
func GetLoadoutFull(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}

	var full models.LoadoutFull
	err := inTx(c.Request.Context(), db, func(tx *sql.Tx) error {
		ctx := c.Request.Context()
		loadout, err := LoadoutGetOwned(ctx, tx, loadoutID, userID)
		if err != nil {
			return err
		}
		if loadout == nil {
			return errLoadoutNotFound
		}
		places, err := LoadoutPlaces(ctx, tx, loadoutID)
		if err != nil {
			return err
		}
		items, err := LoadoutItemsOwned(ctx, tx, loadoutID, userID)
		if err != nil {
			return err
		}
		full = models.LoadoutFull{Loadout: *loadout, Places: places, Items: *items}
		return nil
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.IndentedJSON(http.StatusOK, full)
}

// UpdateLoadout updates an existing loadout.
//
//	@Summary		Update loadout
//	@Description	Update the loadout named in the path. Only fields present in the body change. A loadout_id or user_id in the body that differs from the path or the caller is refused with 400.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			request	body		models.LoadoutUpdate	true	"Updated loadout data"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		409		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/update [post]
func UpdateLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Errorf("read body: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
		return
	}

	ctx := c.Request.Context()
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		// 404 takes precedence over 400 (BR-018 AC-4).
		owned, err := LoadoutGetOwned(ctx, tx, loadoutID, userID)
		if err != nil {
			return err
		}
		if owned == nil {
			return errLoadoutNotFound
		}

		var body struct {
			LoadoutID   *int64  `json:"loadout_id"`
			UserID      *int64  `json:"user_id"`
			Name        *string `json:"loadout_name"`
			Description *string `json:"loadout_description"`
			IsPublic    *bool   `json:"loadout_is_public"`
			Slug        *string `json:"loadout_slug"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		if body.LoadoutID != nil && *body.LoadoutID != loadoutID {
			return badRequest("loadout_id does not match the path")
		}
		if body.UserID != nil && *body.UserID != userID {
			return badRequest("user_id does not match the authenticated user")
		}

		sets := []string{}
		args := []any{}
		if body.Name != nil {
			sets, args = append(sets, "loadoutName = ?"), append(args, *body.Name)
		}
		if body.Description != nil {
			sets, args = append(sets, "loadoutDescription = ?"), append(args, *body.Description)
		}
		if body.IsPublic != nil {
			sets, args = append(sets, "loadoutIsPublic = ?"), append(args, *body.IsPublic)
		}
		if body.Slug != nil {
			sets, args = append(sets, "loadoutSlug = ?"), append(args, nullableSlug(*body.Slug))
		}
		if len(sets) == 0 {
			return badRequest("no updatable fields supplied")
		}
		sets = append(sets, "updatedAt = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
		args = append(args, loadoutID, userID)

		res, err := tx.ExecContext(ctx,
			`UPDATE loadouts SET `+strings.Join(sets, ", ")+` WHERE loadoutId = ? AND userId = ?`, args...)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errLoadoutNotFound
		}
		return nil
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}

// DeleteLoadout deletes a loadout by ID.
//
//	@Summary		Delete loadout
//	@Description	Delete a loadout and its items. Unknown and other users' loadouts both return 404.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/delete [delete]
func DeleteLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}

	ctx := c.Request.Context()
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM loadouts WHERE loadoutId = ? AND userId = ?`, loadoutID, userID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errLoadoutNotFound
		}
		// The items go with the loadout through ON DELETE CASCADE; the
		// explicit delete keeps the transaction correct on its own.
		_, err = tx.ExecContext(ctx, `DELETE FROM loadout_items WHERE loadoutId = ?`, loadoutID)
		return err
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}
