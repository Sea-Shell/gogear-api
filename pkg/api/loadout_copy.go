package endpoints

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	zap "go.uber.org/zap"
)

// CopyLoadout copies one of the caller's loadouts into a new loadout.
//
//	@Summary		Copy loadout
//	@Description	Copy a loadout of the caller into a new private loadout of the caller, all or nothing, in one transaction. The request states every option: loadout_name (any string, as for insert), copy_places and copy_notes_and_goal; a missing, null or wrongly typed one is 400 and nothing is created. Always copied: every item whose registration still exists, with its registration, quantity and consumable role, as a new item (several items of one registration stay several items). With copy_places the copy has Last minute if the source has one, the same bags with the same nesting and order, and each item in the same place, Worn included (capability is not checked again); without it the copy has no bags and no Last minute and no item has a place, not even one that was in Worn. With copy_notes_and_goal the copy has the loadout description, the weight goal and each item's notes; without it none of them. Never copied: ticks, closed places, stage (the copy is in planning), done_at, trip dates, the public flag (the copy is private) and the slug (the copy has none). Items whose registration no longer exists are not copied and are named in left_out by gear_name. The source is only read and is unchanged; a change to it made at the same time is either fully in the copy or not in it. An unknown loadout, another user's loadout (public or not) is 404 and nothing is created. A loadout_id or user_id in the body that differs from the path or the caller is 400.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"ID of the loadout to copy"
//	@Param			request	body		models.LoadoutCopyRequest	true	"Name and copy options"
//	@Success		201		{object}	models.LoadoutCopyResult
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/copy [post]
func CopyLoadout(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}
	data, ok := readBody(c, log)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	var result models.LoadoutCopyResult
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		// 404 before 400 (ARS-001 section 1): a foreign source is refused
		// whatever the body says.
		source, err := LoadoutGetOwned(ctx, tx, loadoutID, userID)
		if err != nil {
			return err
		}
		if source == nil {
			return errLoadoutNotFound
		}
		req, err := parseCopyRequest(data, loadoutID, userID)
		if err != nil {
			return err
		}
		result, err = copyLoadout(ctx, tx, source, req)
		return err
	})
	if err != nil {
		respondOpError(c, log, err)
		return
	}

	c.IndentedJSON(http.StatusCreated, result)
}

// parseCopyRequest reads the copy body. The name and both options must be
// named with the right type (ARS-001 milestone C, BR-031 AC-8); identity fields
// never take effect, and a conflicting one is 400 (BR-017).
func parseCopyRequest(data []byte, loadoutID, userID int64) (models.LoadoutCopyRequest, error) {
	var req models.LoadoutCopyRequest
	var body struct {
		LoadoutID        *int64          `json:"loadout_id"`
		UserID           *int64          `json:"user_id"`
		LoadoutName      json.RawMessage `json:"loadout_name"`
		CopyPlaces       json.RawMessage `json:"copy_places"`
		CopyNotesAndGoal json.RawMessage `json:"copy_notes_and_goal"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return req, badRequest(err.Error())
	}
	if body.LoadoutID != nil && *body.LoadoutID != loadoutID {
		return req, badRequest("loadout_id does not match the path")
	}
	if body.UserID != nil && *body.UserID != userID {
		return req, badRequest("user_id does not match the authenticated user")
	}
	if body.LoadoutName == nil || json.Unmarshal(body.LoadoutName, &req.LoadoutName) != nil || isJSONNull(body.LoadoutName) {
		return req, badRequest("loadout_name is required and must be a string")
	}
	var err error
	if req.CopyPlaces, err = requiredBool(body.CopyPlaces, "copy_places"); err != nil {
		return req, err
	}
	if req.CopyNotesAndGoal, err = requiredBool(body.CopyNotesAndGoal, "copy_notes_and_goal"); err != nil {
		return req, err
	}
	return req, nil
}

// requiredBool reads an option the caller must state: absent, null and any
// non-boolean are 400.
func requiredBool(raw json.RawMessage, field string) (bool, error) {
	var v bool
	if raw == nil || isJSONNull(raw) || json.Unmarshal(raw, &v) != nil {
		return false, badRequest(field + " is required and must be true or false")
	}
	return v, nil
}

// copyLoadout makes the copy inside the caller's transaction, so any failure
// leaves nothing behind (BR-031 AC-7), and reads the source in the same
// transaction, so the copy shows it at one moment. The source is only read.
func copyLoadout(ctx context.Context, tx *sql.Tx, source *models.Loadout, req models.LoadoutCopyRequest) (models.LoadoutCopyResult, error) {
	none := models.LoadoutCopyResult{}
	sourceID, userID := *source.LoadoutID, source.UserID

	places, err := LoadoutPlaces(ctx, tx, sourceID)
	if err != nil {
		return none, err
	}
	items, err := LoadoutItemsOwned(ctx, tx, sourceID, userID)
	if err != nil {
		return none, err
	}

	// The new loadout: private, no slug, no dates, stage planning (column
	// defaults). Description and goal only with the notes option.
	var description string
	var goal any
	if req.CopyNotesAndGoal {
		description = source.LoadoutDescription
		if source.WeightGoal != nil {
			goal = *source.WeightGoal
		}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO loadouts (userId, loadoutName, loadoutDescription, weightGoal) VALUES (?, ?, ?, ?)`,
		userID, req.LoadoutName, description, goal)
	if err != nil {
		return none, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return none, err
	}

	// source place ID -> copy place ID. Without copy_places it stays empty, so
	// every copied item has no place, even one that was in Worn: BR-031 AC-3
	// ("every copied item has no place") governs, not the ARS-001 step 3
	// wording that maps Worn always.
	placeMap := map[int64]int64{}
	if req.CopyPlaces {
		if err := copyPlaces(ctx, tx, places, newID, placeMap); err != nil {
			return none, err
		}
	}

	leftOut := []models.LoadoutLeftOutItem{}
	for _, it := range *items {
		if it.UsergearRegistrationID == nil {
			leftOut = append(leftOut, models.LoadoutLeftOutItem{LoadoutItemID: *it.LoadoutItemID, GearName: it.GearName})
			continue
		}
		var place any
		if it.PlaceID != nil {
			if id, mapped := placeMap[*it.PlaceID]; mapped {
				place = id
			} else if req.CopyPlaces {
				return none, fmt.Errorf("copy loadout %d: item %d is in place %d, which is not a place of the loadout", sourceID, *it.LoadoutItemID, *it.PlaceID)
			}
		}
		notes := ""
		if req.CopyNotesAndGoal {
			notes = it.Notes
		}
		// ticked takes its default, 0 (BR-031 AC-5).
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO loadout_items (loadoutId, registrationId, placeId, quantity, notes, consumable) VALUES (?, ?, ?, ?, ?, ?)`,
			newID, *it.UsergearRegistrationID, place, it.Quantity, notes, it.Consumable); err != nil {
			return none, err
		}
	}

	created, err := LoadoutGetOwned(ctx, tx, newID, userID)
	if err != nil {
		return none, err
	}
	if created == nil {
		return none, fmt.Errorf("copy loadout %d: the new loadout %d cannot be read back", sourceID, newID)
	}
	return models.LoadoutCopyResult{Loadout: *created, LeftOut: leftOut}, nil
}

// copyPlaces maps the source's Worn to the copy's Worn (made by the trigger
// on loadouts), creates Last minute if the source has it and every bag with
// the same registration and position, then sets the parents through the ID
// map, so a parent never has to exist before its child. Everything starts
// open. Capability is not checked: a bag that can no longer hold items is
// copied like the others (BR-031 AC-3).
func copyPlaces(ctx context.Context, tx *sql.Tx, places []models.LoadoutPlace, newID int64, placeMap map[int64]int64) error {
	for _, p := range places {
		switch p.PlaceKind {
		case placeKindWorn:
			var newWorn int64
			if err := tx.QueryRowContext(ctx,
				`SELECT placeId FROM loadout_places WHERE loadoutId = ? AND placeKind = 'worn'`, newID).Scan(&newWorn); err != nil {
				return fmt.Errorf("read the new loadout's Worn: %w", err)
			}
			placeMap[p.PlaceID] = newWorn
		case placeKindLastMinute:
			res, err := tx.ExecContext(ctx, `INSERT INTO loadout_places (loadoutId, placeKind) VALUES (?, 'last_minute')`, newID)
			if err != nil {
				return err
			}
			if placeMap[p.PlaceID], err = res.LastInsertId(); err != nil {
				return err
			}
		case placeKindBag:
			res, err := tx.ExecContext(ctx,
				`INSERT INTO loadout_places (loadoutId, placeKind, registrationId, position) VALUES (?, 'bag', ?, ?)`,
				newID, p.UsergearRegistrationID, p.Position)
			if err != nil {
				return err
			}
			if placeMap[p.PlaceID], err = res.LastInsertId(); err != nil {
				return err
			}
		}
	}
	for _, p := range places {
		if p.PlaceKind != placeKindBag || p.ParentPlaceID == nil {
			continue
		}
		parent, ok := placeMap[*p.ParentPlaceID]
		if !ok {
			return fmt.Errorf("copy loadout: bag %d has parent %d, which is not a bag of the loadout", p.PlaceID, *p.ParentPlaceID)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ?`, parent, placeMap[p.PlaceID]); err != nil {
			return err
		}
	}
	return nil
}
