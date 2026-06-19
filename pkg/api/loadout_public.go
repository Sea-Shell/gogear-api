package endpoints

import (
	"database/sql"
	"net/http"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	zap "go.uber.org/zap"
)

// sanitizeLoadout strips private fields from a Loadout for public responses.
func sanitizeLoadout(l *models.Loadout) *models.LoadoutPublic {
	if l == nil {
		return nil
	}
	return &models.LoadoutPublic{
		LoadoutID:          *l.LoadoutID,
		LoadoutName:        l.LoadoutName,
		LoadoutDescription: l.LoadoutDescription,
		LoadoutSlug:        l.LoadoutSlug,
		TotalWeight:        l.TotalWeight,
		CreatedAt:          l.CreatedAt,
		UpdatedAt:          l.UpdatedAt,
	}
}

// GetPublicLoadout returns a public loadout by slug.
// No authentication required — only public (loadoutIsPublic=true) loadouts are returned.
//
//	@Summary		Get public loadout
//	@Description	Get a public loadout by slug. Only returns loadouts with loadout_is_public=true. No authentication required.
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			slug	path		string	true	"Loadout slug"
//	@Success		200		{object}	models.LoadoutPublic
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/public/loadout/{slug} [get]
func GetPublicLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	slug := c.Param("slug")
	if slug == "" {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "slug is required"})
		return
	}

	l, err := LoadoutGetBySlug(db, slug)
	if err != nil {
		log.Errorf("Error fetching loadout by slug: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	if l == nil {
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "loadout not found"})
		return
	}
	if !l.LoadoutIsPublic {
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "loadout not found"})
		return
	}

	c.IndentedJSON(http.StatusOK, sanitizeLoadout(l))
}

// GetPublicLoadoutItems returns items for a public loadout by slug.
//
//	@Summary		List public loadout items
//	@Description	List items for a public loadout by slug. No authentication required.
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			slug	path		string	true	"Loadout slug"
//	@Success		200		{object}	[]models.LoadoutItem
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/public/loadout/{slug}/items [get]
func GetPublicLoadoutItems(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)

	slug := c.Param("slug")
	if slug == "" {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "slug is required"})
		return
	}

	l, err := LoadoutGetBySlug(db, slug)
	if err != nil {
		log.Errorf("Error fetching loadout by slug: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}
	if l == nil || !l.LoadoutIsPublic {
		c.IndentedJSON(http.StatusNotFound, models.Error{Error: "loadout not found"})
		return
	}

	items, err := LoadoutItemsByLoadout(db, *l.LoadoutID)
	if err != nil {
		log.Errorf("Error fetching loadout items: %#v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: err.Error()})
		return
	}

	if items == nil {
		items = &[]models.LoadoutItem{}
	}

	c.IndentedJSON(http.StatusOK, items)
}
