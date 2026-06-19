package endpoints

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	models "github.com/Sea-Shell/gogear-api/pkg/models"
	utils "github.com/Sea-Shell/gogear-api/pkg/utils"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// InsertLoadoutItem adds gear to a loadout.
//
//	@Summary		Insert loadout item
//	@Description	Add gear to a loadout
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			body	body		models.LoadoutItemNoID	true	"Item data"
//	@Success		201		{object}	models.LoadoutItem
//	@Failure		400		{object}	models.Error
//	@Failure		403		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/insert [put]
func InsertLoadoutItem(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	loadoutID, err := strconv.ParseInt(c.Param("loadout"), 10, 64)
	if err != nil {
		log.Errorf("invalid loadout ID: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid loadout ID"})
		return
	}

	loadoutIDInt := int(loadoutID)
	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutIDInt, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	if existing.UserID != c.MustGet("user_id_int64").(int64) {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "Access denied"})
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Errorf("error reading body: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var item models.LoadoutItemNoID
	if err := json.Unmarshal(data, &item); err != nil {
		log.Errorf("error unmarshaling body: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	item.LoadoutID = loadoutID

	body, err := json.Marshal(item)
	if err != nil {
		log.Errorf("error marshaling item: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	createdObject, err := utils.GenericInsert[models.LoadoutItem]("loadout_items", body, db)
	if err != nil {
		log.Errorf("error inserting loadout item: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if err := LoadoutRecalculateWeight(db, loadoutID); err != nil {
		log.Errorf("error recalculating weight: %#v", err)
	}

	c.JSON(http.StatusCreated, createdObject)
}

// ListLoadoutItems lists all items in a loadout.
//
//	@Summary		List loadout items
//	@Description	List all items in a loadout
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

	loadoutID, err := strconv.ParseInt(c.Param("loadout"), 10, 64)
	if err != nil {
		log.Errorf("invalid loadout ID: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid loadout ID"})
		return
	}

	loadoutIDInt := int(loadoutID)
	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutIDInt, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	if existing.UserID != c.MustGet("user_id_int64").(int64) {
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}

	items, err := LoadoutItemsByLoadout(db, loadoutID)
	if err != nil {
		log.Errorf("error listing loadout items: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if items == nil {
		items = &[]models.LoadoutItem{}
	}

	c.JSON(http.StatusOK, items)
}

// UpdateLoadoutItem updates a loadout item.
//
//	@Summary		Update loadout item
//	@Description	Update a loadout item
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int							true	"Loadout ID"
//	@Param			item	path		int							true	"Item ID"
//	@Param			body	body		models.LoadoutItemUpdate	true	"Update data"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		403		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/{item}/update [post]
func UpdateLoadoutItem(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	loadoutID, err := strconv.ParseInt(c.Param("loadout"), 10, 64)
	if err != nil {
		log.Errorf("invalid loadout ID: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid loadout ID"})
		return
	}

	loadoutIDInt := int(loadoutID)
	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutIDInt, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	if existing.UserID != c.MustGet("user_id_int64").(int64) {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "Access denied"})
		return
	}

	itemID, err := strconv.ParseInt(c.Param("item"), 10, 64)
	if err != nil {
		log.Errorf("invalid item ID: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid item ID"})
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Errorf("error reading body: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	var update models.LoadoutItemUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		log.Errorf("error unmarshaling body: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	update.LoadoutItemID = itemID

	body, err := json.Marshal(update)
	if err != nil {
		log.Errorf("error marshaling update: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if err := utils.GenericUpdate[models.LoadoutItemUpdate]("loadout_items", body, db); err != nil {
		log.Errorf("error updating loadout item: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if err := LoadoutRecalculateWeight(db, loadoutID); err != nil {
		log.Errorf("error recalculating weight: %#v", err)
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}

// DeleteLoadoutItem removes an item from a loadout.
//
//	@Summary		Delete loadout item
//	@Description	Remove an item from a loadout
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Param			item	path		int	true	"Item ID"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		403		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/item/{item}/delete [delete]
func DeleteLoadoutItem(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	loadoutID, err := strconv.Atoi(c.Param("loadout"))
	if err != nil {
		log.Errorf("invalid loadout ID: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid loadout ID"})
		return
	}

	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutID, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	if existing.UserID != c.MustGet("user_id_int64").(int64) {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "Access denied"})
		return
	}

	itemID, err := strconv.Atoi(c.Param("item"))
	if err != nil {
		log.Errorf("invalid item ID: %#v", err)
		c.JSON(http.StatusBadRequest, models.Error{Error: "invalid item ID"})
		return
	}

	deletedItem, err := utils.GenericDelete[models.LoadoutItem]("loadout_items", itemID, db)
	if err != nil {
		log.Errorf("error deleting loadout item: %#v", err)
		c.JSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if err := LoadoutRecalculateWeight(db, deletedItem.LoadoutID); err != nil {
		log.Errorf("error recalculating weight: %#v", err)
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}
