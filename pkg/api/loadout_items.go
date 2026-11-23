package endpoints

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// Item writes bind the item ID, its parent loadout and the loadout's owner in
// one statement, inside one transaction (OC-001 G-2, G-4). Refusals follow
// ARS-001 section 1: 404 (target), then 400 (request), then 409 (state).

// itemInOwnedLoadout is the SQL condition shared by item update, tick and delete.
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

// InsertLoadoutItem adds an item to a loadout.
//
//	@Summary		Insert loadout item
//	@Description	Add one of the caller's gear registrations to a loadout, optionally into a place (Worn, Last minute or a bag of the loadout). quantity defaults to 1; zero or negative is refused with 400. A loadout_id in the body that differs from the path is refused with 400. Deprecated legacy path (until the cutover, ADR-004 D-2): gear_id instead of usergear_registration_id binds the item to the caller's lowest-ID registration of that gear, or keeps it as a legacy item that reads the gear live. Sending both is 400. A foreign or unknown registration or place is 404; a place that is a bag whose gear cannot hold items is 409.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			body	body		models.LoadoutItemNoID	true	"Item data"
//	@Success		201		{object}	models.LoadoutItem
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		409		{object}	models.Error
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
	var created *models.LoadoutItem
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		owned, err := LoadoutGetOwned(ctx, tx, loadoutID, userID)
		if err != nil {
			return err
		}
		if owned == nil {
			return errLoadoutNotFound
		}

		var body struct {
			LoadoutID      *int64  `json:"loadout_id"`
			GearID         *int64  `json:"gear_id"`
			RegistrationID *int64  `json:"usergear_registration_id"`
			Quantity       *int64  `json:"quantity"`
			Notes          *string `json:"notes"`
			PlaceID        *int64  `json:"place_id"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		if body.GearID != nil && body.RegistrationID != nil {
			return badRequest("send usergear_registration_id or gear_id, not both")
		}
		if body.GearID == nil && body.RegistrationID == nil {
			return badRequest("usergear_registration_id is required")
		}

		// Targets first (404), then the request itself (400), then state (409).
		var registrationID, detachedGearID *int64
		if body.RegistrationID != nil {
			var one int
			err := tx.QueryRowContext(ctx,
				`SELECT 1 FROM user_gear_registrations WHERE userGearRegistrationId = ? AND userId = ?`,
				*body.RegistrationID, userID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return errRegistrationNotFound
			}
			if err != nil {
				return err
			}
			registrationID = body.RegistrationID
		} else {
			// Legacy path (ADR-004 D-2): the lowest registration of this gear
			// that the caller owns, else a legacy item reading the gear live.
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gear WHERE gearId = ?)`, *body.GearID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return errGearNotFound
			}
			var lowest sql.NullInt64
			if err := tx.QueryRowContext(ctx,
				`SELECT MIN(userGearRegistrationId) FROM user_gear_registrations WHERE gearId = ? AND userId = ?`,
				*body.GearID, userID).Scan(&lowest); err != nil {
				return err
			}
			if lowest.Valid {
				registrationID = &lowest.Int64
			} else {
				detachedGearID = body.GearID
			}
		}
		var bagRegistration *int64
		if body.PlaceID != nil {
			if bagRegistration, err = lookupPlace(ctx, tx, loadoutID, *body.PlaceID); err != nil {
				return err
			}
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
		if err := requireCapableBag(ctx, tx, bagRegistration); err != nil {
			return err
		}

		var reason any
		if detachedGearID != nil {
			reason = "legacy"
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO loadout_items (loadoutId, registrationId, placeId, quantity, notes, detachedReason, detachedGearId)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			loadoutID, registrationID, body.PlaceID, quantity, notes, reason, detachedGearID)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		created, err = LoadoutItemByID(ctx, tx, loadoutID, id)
		if err == nil && created == nil {
			err = fmt.Errorf("inserted item %d not readable", id)
		}
		return err
	})
	if err != nil {
		respondItemError(c, log, err)
		return
	}

	c.JSON(http.StatusCreated, created)
}

// respondItemError is respondOpError for item writes. A unique or trigger
// refusal means a concurrent or missed check; it is a 409 (the status of the
// rules the triggers repeat), and a trigger message is logged (ARS-001 s. 1).
// respondOpError would turn them into a 500 or a slug message.
func respondItemError(c *gin.Context, log *zap.SugaredLogger, err error) {
	switch {
	case isTriggerViolation(err):
		log.Errorf("item write refused by a database trigger (a handler check is missing): %v", err)
		c.IndentedJSON(http.StatusConflict, models.Error{Error: "the item change conflicts with the loadout's current state"})
	case isUniqueViolation(err):
		c.IndentedJSON(http.StatusConflict, models.Error{Error: "the item already exists in this loadout"})
	default:
		respondOpError(c, log, err)
	}
}

// lookupPlace checks that placeID is a place of loadoutID (404 otherwise) and
// returns the registration of the place when it is a bag. Call inside the
// write transaction.
func lookupPlace(ctx context.Context, q sqlRunner, loadoutID, placeID int64) (*int64, error) {
	var reg sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT registrationId FROM loadout_places WHERE placeId = ? AND loadoutId = ?`, placeID, loadoutID).Scan(&reg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errPlaceNotFound
	}
	if err != nil {
		return nil, err
	}
	if reg.Valid {
		return &reg.Int64, nil
	}
	return nil, nil
}

// requireCapableBag is 409 when the place is a bag (bagRegistration set) whose
// gear cannot hold items now (BR-024 AC-2, AC-6). Nil means Worn, Last minute
// or no place, which always accept items.
func requireCapableBag(ctx context.Context, q sqlRunner, bagRegistration *int64) error {
	if bagRegistration == nil {
		return nil
	}
	var capable bool
	err := q.QueryRowContext(ctx,
		`SELECT cap.isContainer = 1 FROM user_gear_registrations r
		 JOIN gear_container_capability cap ON cap.gearId = r.gearId
		 WHERE r.userGearRegistrationId = ?`, *bagRegistration).Scan(&capable)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !capable {
		return &opError{http.StatusConflict, "the bag's gear cannot hold items"}
	}
	return nil
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

	items, err := LoadoutItemsOwned(c.Request.Context(), db, loadoutID, userID)
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, items)
}

// UpdateLoadoutItem updates a loadout item.
//
//	@Summary		Update loadout item
//	@Description	Update an item of a loadout owned by the caller. Only fields present in the body change; place_id null removes the item from its place. The item must belong to the loadout in the path, otherwise 404, and so must a place_id. A loadout_id or loadout_item_id in the body that differs from the path is refused with 400. A bag whose gear cannot hold items is 409. Never changes ticked.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int							true	"Loadout ID"
//	@Param			item	path		int							true	"Item ID"
//	@Param			body	body		models.LoadoutItemUpdate	true	"Update data"
//	@Success		200		{object}	models.LoadoutItem
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		409		{object}	models.Error
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
	var updated *models.LoadoutItem
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
			LoadoutItemID *int64          `json:"loadout_item_id"`
			LoadoutID     *int64          `json:"loadout_id"`
			Quantity      *int64          `json:"quantity"`
			Notes         *string         `json:"notes"`
			PlaceID       json.RawMessage `json:"place_id"` // absent: nil; null: "null" (clearable)
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		placeGiven := body.PlaceID != nil
		var placeID *int64
		if placeGiven && string(body.PlaceID) != "null" {
			var p int64
			if err := json.Unmarshal(body.PlaceID, &p); err != nil {
				return badRequest("place_id must be a number or null")
			}
			placeID = &p
		}

		var bagRegistration *int64
		if placeID != nil {
			if bagRegistration, err = lookupPlace(ctx, tx, loadoutID, *placeID); err != nil {
				return err
			}
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
		if placeGiven {
			sets, args = append(sets, "placeId = ?"), append(args, placeID)
		}
		if len(sets) == 0 {
			return badRequest("no updatable fields supplied")
		}
		if err := requireCapableBag(ctx, tx, bagRegistration); err != nil {
			return err
		}
		args = append(args, itemID, loadoutID, loadoutID, userID)

		res, err := tx.ExecContext(ctx,
			`UPDATE loadout_items SET `+strings.Join(sets, ", ")+` WHERE `+itemInOwnedLoadout, args...)
		if err := affectedOne(res, err, errItemNotFound); err != nil {
			return err
		}
		updated, err = LoadoutItemByID(ctx, tx, loadoutID, itemID)
		if err == nil && updated == nil {
			err = errItemNotFound
		}
		return err
	})
	if err != nil {
		respondItemError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// TickLoadoutItem sets whether an item is packed.
//
//	@Summary		Tick loadout item
//	@Description	Set (not toggle) the packed state of an item of a loadout owned by the caller. Setting the value it already has succeeds and changes nothing. An item that was removed, or a loadout that is gone or not the caller's, is 404 and changes nothing.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int					true	"Loadout ID"
//	@Param			item	path		int					true	"Item ID"
//	@Param			body	body		models.LoadoutItemTick	true	"ticked (boolean, required)"
//	@Success		200		{object}	models.LoadoutItemTickResult
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/{item}/tick [post]
func TickLoadoutItem(c *gin.Context) {
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
	var ticked bool
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM loadout_items WHERE `+itemInOwnedLoadout,
			itemID, loadoutID, loadoutID, userID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return errItemNotFound
		}
		if err != nil {
			return err
		}

		var body struct {
			Ticked *bool `json:"ticked"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		if body.Ticked == nil {
			return badRequest("ticked (true or false) is required")
		}
		ticked = *body.Ticked
		res, err := tx.ExecContext(ctx, `UPDATE loadout_items SET ticked = ? WHERE `+itemInOwnedLoadout,
			ticked, itemID, loadoutID, loadoutID, userID)
		return affectedOne(res, err, errItemNotFound)
	})
	if err != nil {
		respondItemError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.LoadoutItemTickResult{LoadoutItemID: itemID, Ticked: ticked})
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
		return affectedOne(res, err, errItemNotFound)
	})
	if err != nil {
		respondItemError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}
