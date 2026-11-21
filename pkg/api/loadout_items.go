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

// Item writes bind the item ID, its parent loadout and the loadout's owner in
// one statement. Each mutation and the weight recalculation share one
// transaction (OC-001 G-2, G-4, G-5).

// itemInOwnedLoadout is the SQL condition shared by item update and delete.
// Arguments: itemID, loadoutID, loadoutID, userID.
const itemInOwnedLoadout = `loadoutItemId = ? AND loadoutId = ?
	AND loadoutId IN (SELECT loadoutId FROM loadouts WHERE loadoutId = ? AND userId = ?)`

func affectedOne(res sql.Result, err error, notFound *opError) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return notFound
	}
	return nil
}

// InsertLoadoutItem adds gear to a loadout.
//
//	@Summary		Insert loadout item
//	@Description	Add gear to a loadout owned by the caller. quantity defaults to 1; zero or negative is refused with 400. A loadout_id in the body that differs from the path is refused with 400.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			body	body		models.LoadoutItemNoID	true	"Item data"
//	@Success		201		{object}	models.LoadoutItem
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/insert [put]
func InsertLoadoutItem(c *gin.Context) {
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
	var created models.LoadoutItem
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		owned, err := LoadoutGetOwned(ctx, tx, loadoutID, userID)
		if err != nil {
			return err
		}
		if owned == nil {
			return errLoadoutNotFound
		}

		var body struct {
			LoadoutID *int64  `json:"loadout_id"`
			GearID    int64   `json:"gear_id"`
			Quantity  *int64  `json:"quantity"`
			Notes     *string `json:"notes"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		if body.LoadoutID != nil && *body.LoadoutID != loadoutID {
			return badRequest("loadout_id does not match the path")
		}
		quantity := int64(1)
		if body.Quantity != nil {
			quantity = *body.Quantity
		}
		if quantity < 1 {
			return badRequest("quantity must be at least 1")
		}
		notes := ""
		if body.Notes != nil {
			notes = *body.Notes
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO loadout_items (loadoutId, gearId, quantity, notes)
			 SELECT loadoutId, ?, ?, ? FROM loadouts WHERE loadoutId = ? AND userId = ?`,
			body.GearID, quantity, notes, loadoutID, userID)
		if err := affectedOne(res, err, errLoadoutNotFound); err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := LoadoutRecalculateWeight(ctx, tx, loadoutID); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx,
			`SELECT loadoutItemId, loadoutId, gearId, quantity, notes FROM loadout_items WHERE loadoutItemId = ?`, id).
			Scan(&created.LoadoutItemID, &created.LoadoutID, &created.GearID, &created.Quantity, &created.Notes)
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusCreated, created)
}

// ListLoadoutItems lists all items in a loadout.
//
//	@Summary		List loadout items
//	@Description	List all items in a loadout owned by the caller. An empty loadout returns [].
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Success		200		{array}		models.LoadoutItem
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/list [get]
func ListLoadoutItems(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}

	owned, err := LoadoutGetOwned(c.Request.Context(), db, loadoutID, userID)
	if err != nil {
		respondOpError(c, log, err)
		return
	}
	if owned == nil {
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: errLoadoutNotFound.msg})
		return
	}

	items, err := LoadoutItemsByLoadout(db, loadoutID)
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, items)
}

// UpdateLoadoutItem updates a loadout item.
//
//	@Summary		Update loadout item
//	@Description	Update an item of a loadout owned by the caller. Only fields present in the body change. The item must belong to the loadout in the path, otherwise 404. A loadout_id or loadout_item_id in the body that differs from the path is refused with 400.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int							true	"Loadout ID"
//	@Param			item	path		int							true	"Item ID"
//	@Param			body	body		models.LoadoutItemUpdate	true	"Update data"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/{item}/update [post]
func UpdateLoadoutItem(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}
	itemID, ok := parseLoadoutIDParam(c, "item")
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
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM loadout_items WHERE `+itemInOwnedLoadout,
			itemID, loadoutID, loadoutID, userID).Scan(&one)
		if err == sql.ErrNoRows {
			return errItemNotFound
		}
		if err != nil {
			return err
		}

		var body struct {
			LoadoutItemID *int64  `json:"loadout_item_id"`
			LoadoutID     *int64  `json:"loadout_id"`
			Quantity      *int64  `json:"quantity"`
			Notes         *string `json:"notes"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		if body.LoadoutItemID != nil && *body.LoadoutItemID != itemID {
			return badRequest("loadout_item_id does not match the path")
		}
		if body.LoadoutID != nil && *body.LoadoutID != loadoutID {
			return badRequest("loadout_id does not match the path")
		}

		sets := []string{}
		args := []any{}
		if body.Quantity != nil {
			if *body.Quantity < 1 {
				return badRequest("quantity must be at least 1")
			}
			sets, args = append(sets, "quantity = ?"), append(args, *body.Quantity)
		}
		if body.Notes != nil {
			sets, args = append(sets, "notes = ?"), append(args, *body.Notes)
		}
		if len(sets) == 0 {
			return badRequest("no updatable fields supplied")
		}
		args = append(args, itemID, loadoutID, loadoutID, userID)

		res, err := tx.ExecContext(ctx,
			`UPDATE loadout_items SET `+strings.Join(sets, ", ")+` WHERE `+itemInOwnedLoadout, args...)
		if err := affectedOne(res, err, errItemNotFound); err != nil {
			return err
		}
		return LoadoutRecalculateWeight(ctx, tx, loadoutID)
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}

// DeleteLoadoutItem removes an item from a loadout.
//
//	@Summary		Delete loadout item
//	@Description	Remove an item from a loadout owned by the caller. The item must belong to the loadout in the path, otherwise 404.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Param			item	path		int	true	"Item ID"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/{item}/delete [delete]
func DeleteLoadoutItem(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}
	itemID, ok := parseLoadoutIDParam(c, "item")
	if !ok {
		return
	}

	ctx := c.Request.Context()
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM loadout_items WHERE `+itemInOwnedLoadout,
			itemID, loadoutID, loadoutID, userID)
		if err := affectedOne(res, err, errItemNotFound); err != nil {
			return err
		}
		return LoadoutRecalculateWeight(ctx, tx, loadoutID)
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}
