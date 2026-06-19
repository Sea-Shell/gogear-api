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

// InsertLoadout creates a new loadout for the authenticated user.
//
//	@Summary		Insert loadout
//	@Description	Create a new loadout for the authenticated user
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.LoadoutNoID	true	"Loadout data"
//	@Success		201		{object}	models.Loadout
//	@Failure		400		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/insert [put]
func InsertLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	var body models.LoadoutNoID
	if err := json.Unmarshal(data, &body); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}
	body.UserID = c.MustGet("user_id_int64").(int64)

	result, err := db.Exec(
		`INSERT INTO loadouts (userId, loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug) VALUES (?, ?, ?, ?, ?)`,
		body.UserID, body.LoadoutName, body.LoadoutDescription, body.LoadoutIsPublic, body.LoadoutSlug,
	)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	createdObject, err := utils.GenericGet[models.Loadout]("loadouts", int(lastID), nil, db)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	c.IndentedJSON(http.StatusCreated, createdObject)
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
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if results == nil {
		c.IndentedJSON(http.StatusOK, []models.Loadout{})
		return
	}

	c.IndentedJSON(http.StatusOK, results)
}

// GetLoadout returns a single loadout by ID.
//
//	@Summary		Get loadout
//	@Description	Get a single loadout by ID
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Success		200		{object}	models.Loadout
//	@Failure		404		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/get [get]
func GetLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	loadoutParam, err := strconv.Atoi(c.Param("loadout"))
	if err != nil {
		log.Errorf("Invalid loadout id: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	userID := c.MustGet("user_id_int64").(int64)

	loadout, err := utils.GenericGet[models.Loadout]("loadouts", loadoutParam, nil, db)
	if err != nil || loadout.UserID != userID {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}

	c.IndentedJSON(http.StatusOK, loadout)
}

// UpdateLoadout updates an existing loadout.
//
//	@Summary		Update loadout
//	@Description	Update an existing loadout
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			request	body		models.LoadoutUpdate	true	"Updated loadout data"
//	@Success		200		{object}	models.Status
//	@Failure		403		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/update [post]
func UpdateLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutParam, err := strconv.Atoi(c.Param("loadout"))
	if err != nil {
		log.Errorf("Invalid loadout id: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	// Ownership check: verify loadout belongs to user
	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutParam, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	if existing.UserID != userID {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "Access denied"})
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	if err := utils.GenericUpdate[models.LoadoutUpdate]("loadouts", data, db); err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}

// DeleteLoadout deletes a loadout by ID.
//
//	@Summary		Delete loadout
//	@Description	Delete a loadout by ID
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Success		200		{object}	models.Status
//	@Failure		403		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/delete [delete]
func DeleteLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutParam, err := strconv.Atoi(c.Param("loadout"))
	if err != nil {
		log.Errorf("Invalid loadout id: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	// Ownership check: verify loadout belongs to user
	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutParam, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	if existing.UserID != userID {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "Access denied"})
		return
	}

	_, err = utils.GenericDelete[models.Loadout]("loadouts", loadoutParam, db)
	if err != nil {
		log.Errorf("Error deleting loadout: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}

// ImportLoadout imports gear IDs into a loadout as items.
//
//	@Summary		Import gear into loadout
//	@Description	Import gear items into a loadout
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int					true	"Loadout ID"
//	@Param			request	body		map[string][]int64	true	"Gear IDs to import"
//	@Success		201		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		403		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/import [post]
func ImportLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	loadoutParam, err := strconv.Atoi(c.Param("loadout"))
	if err != nil {
		log.Errorf("Invalid loadout id: %#v", err)
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		return
	}
	loadoutID := int64(loadoutParam)

	// Ownership check: verify loadout belongs to user
	existing, err := utils.GenericGet[models.Loadout]("loadouts", loadoutParam, nil, db)
	if err != nil {
		log.Errorf("Loadout not found: %#v", err)
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "Loadout not found"})
		return
	}
	userID := c.MustGet("user_id_int64").(int64)
	if existing.UserID != userID {
		c.IndentedJSON(http.StatusForbidden, models.Error{Error: "Access denied"})
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	var body struct {
		GearIDs []int64 `json:"gear_ids"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: err.Error()})
		log.Error(err.Error())
		return
	}

	for _, gearID := range body.GearIDs {
		item := models.LoadoutItemNoID{
			LoadoutID: loadoutID,
			GearID:    gearID,
			Quantity:  1,
			Notes:     "",
		}
		itemData, err := json.Marshal(item)
		if err != nil {
			c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
			log.Error(err.Error())
			return
		}
		if _, err := utils.GenericInsert[models.LoadoutItem]("loadout_items", itemData, db); err != nil {
			c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
			log.Error(err.Error())
			return
		}
	}

	if err := LoadoutRecalculateWeight(db, loadoutID); err != nil {
		log.Errorf("Error recalculating loadout weight: %#v", err)
	}

	c.JSON(http.StatusCreated, models.Status{Status: "success"})
}
