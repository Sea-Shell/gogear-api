package endpoints

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	zap "go.uber.org/zap"
)

// Place writes (ARS-001 section 4, BR-024, BR-025). Every write checks all of
// its rules inside one immediate transaction, so concurrent requests queue
// and each sees the state the previous one left (BR-024 AC-10). Refusals
// follow ARS-001 section 1: 404 (target), then 400 (request), then 409
// (state). The database triggers repeat the structural rules; a trigger
// message reaching the response means a check here was missed.

const (
	placeKindWorn       = "worn"
	placeKindLastMinute = "last_minute"
	placeKindBag        = "bag"
)

// placeSelect is the one read of a place (ARS-001 section 3, milestone A).
// Bag values are live: the registration's gear, container capability and
// effective weight. Worn and Last minute have none of them.
const placeSelect = `SELECT p.placeId, p.loadoutId, p.placeKind, p.closed, p.registrationId,
	g.gearId, g.gearName, cap.isContainer = 1, r.maxContainerWeight,
	CASE WHEN g.gearWeight > 0 THEN g.gearWeight END,
	w.unitWeight, w.weightSource, p.parentPlaceId, p.position
FROM loadout_places p
LEFT JOIN user_gear_registrations r ON r.userGearRegistrationId = p.registrationId
LEFT JOIN gear g ON g.gearId = r.gearId
LEFT JOIN gear_container_capability cap ON cap.gearId = g.gearId
LEFT JOIN registration_effective_weight w ON w.registrationId = p.registrationId`

// placeOrder lists Worn, Last minute, then bags by position (ARS-001 LoadoutFull).
const placeOrder = ` ORDER BY CASE p.placeKind WHEN 'worn' THEN 0 WHEN 'last_minute' THEN 1 ELSE 2 END, p.position, p.placeId`

func scanLoadoutPlace(r rowScanner) (*models.LoadoutPlace, error) {
	var p models.LoadoutPlace
	if err := r.Scan(&p.PlaceID, &p.LoadoutID, &p.PlaceKind, &p.Closed, &p.UsergearRegistrationID,
		&p.GearID, &p.GearName, &p.GearIsContainer, &p.MaxContainerWeight, &p.CatalogWeight,
		&p.UnitWeight, &p.WeightSource, &p.ParentPlaceID, &p.Position); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadoutPlaceByID reads one place of loadoutID (nil, nil when it is not a
// place of that loadout). It does not check ownership.
func LoadoutPlaceByID(ctx context.Context, q sqlRunner, loadoutID, placeID int64) (*models.LoadoutPlace, error) {
	p, err := scanLoadoutPlace(q.QueryRowContext(ctx,
		placeSelect+` WHERE p.placeId = ? AND p.loadoutId = ?`, placeID, loadoutID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query loadout place: %w", err)
	}
	return p, nil
}

// LoadoutPlaces lists the places of loadoutID in the order of ARS-001
// LoadoutFull. It does not check ownership. The result is never nil.
func LoadoutPlaces(ctx context.Context, q sqlRunner, loadoutID int64) ([]models.LoadoutPlace, error) {
	rows, err := q.QueryContext(ctx, placeSelect+` WHERE p.loadoutId = ?`+placeOrder, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("query loadout places: %w", err)
	}
	defer rows.Close()
	places := []models.LoadoutPlace{}
	for rows.Next() {
		p, err := scanLoadoutPlace(rows)
		if err != nil {
			return nil, fmt.Errorf("scan loadout place: %w", err)
		}
		places = append(places, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return places, nil
}

// placeRef is the kind and registration of a place of the loadout.
type placeRef struct {
	kind           string
	registrationID *int64
	parentID       *int64
}

// findPlace is 404 (errPlaceNotFound) unless placeID is a place of loadoutID.
func findPlace(ctx context.Context, q sqlRunner, loadoutID, placeID int64) (*placeRef, error) {
	var ref placeRef
	var reg, parent sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT placeKind, registrationId, parentPlaceId FROM loadout_places WHERE placeId = ? AND loadoutId = ?`,
		placeID, loadoutID).Scan(&ref.kind, &reg, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errPlaceNotFound
	}
	if err != nil {
		return nil, err
	}
	if reg.Valid {
		ref.registrationID = &reg.Int64
	}
	if parent.Valid {
		ref.parentID = &parent.Int64
	}
	return &ref, nil
}

// registrationCapable reports whether the registration's gear can hold items
// now (BR-021).
func registrationCapable(ctx context.Context, q sqlRunner, registrationID int64) (bool, error) {
	var capable bool
	err := q.QueryRowContext(ctx,
		`SELECT cap.isContainer = 1 FROM user_gear_registrations r
		 JOIN gear_container_capability cap ON cap.gearId = r.gearId
		 WHERE r.userGearRegistrationId = ?`, registrationID).Scan(&capable)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return capable, nil
}

// clearableID reads a field that may be absent (given=false), null (id=nil)
// or a number.
func clearableID(raw json.RawMessage, name string) (given bool, id *int64, err error) {
	if raw == nil {
		return false, nil, nil
	}
	if string(raw) == "null" {
		return true, nil, nil
	}
	var v int64
	if err := json.Unmarshal(raw, &v); err != nil {
		return true, nil, badRequest(name + " must be a number or null")
	}
	return true, &v, nil
}

// respondPlaceError is respondOpError for place writes. A unique or trigger
// refusal means a concurrent or missed check; it is a 409 (the status of the
// rules the triggers repeat), and a trigger message is logged (ARS-001 s. 1).
func respondPlaceError(c *gin.Context, log *zap.SugaredLogger, err error) {
	switch {
	case isTriggerViolation(err):
		log.Errorf("place write refused by a database trigger (a handler check is missing): %v", err)
		c.IndentedJSON(http.StatusConflict, models.Error{Error: "the place change conflicts with the loadout's current state"})
	case isUniqueViolation(err):
		c.IndentedJSON(http.StatusConflict, models.Error{Error: "the place already exists in this loadout"})
	default:
		respondOpError(c, log, err)
	}
}

func readBody(c *gin.Context, log *zap.SugaredLogger) ([]byte, bool) {
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Errorf("read body: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
		return nil, false
	}
	return data, true
}

// ownedLoadout is 404 unless the caller owns loadoutID.
func ownedLoadout(ctx context.Context, q sqlRunner, loadoutID, userID int64) error {
	owned, err := LoadoutGetOwned(ctx, q, loadoutID, userID)
	if err != nil {
		return err
	}
	if owned == nil {
		return errLoadoutNotFound
	}
	return nil
}

// InsertLoadoutPlace adds a bag or Last minute to a loadout.
//
//	@Summary		Insert loadout place
//	@Description	Add a bag (one of the caller's container registrations) or the Last minute place to a loadout owned by the caller. A bag without parent_place_id starts inside its closet container when that container is a bag of this loadout whose gear can still hold items, otherwise at top level; parent_place_id null means top level; the closet links are never written. A new bag goes last in the bag order. An unknown or foreign registration or parent is 404; a place_kind other than bag or last_minute, a missing registration, or a parent that is not a bag (Worn, Last minute) is 400; a registration already a bag of this loadout, gear that cannot hold items, a parent bag that cannot hold items, or a second Last minute is 409. Capacity and the weight goal never refuse.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int							true	"Loadout ID"
//	@Param			body	body		models.LoadoutPlaceInsert	true	"Place data"
//	@Success		201		{object}	models.LoadoutPlace
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		409		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/place/insert [put]
func InsertLoadoutPlace(c *gin.Context) {
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
	var created *models.LoadoutPlace
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		if err := ownedLoadout(ctx, tx, loadoutID, userID); err != nil {
			return err
		}
		var body struct {
			PlaceKind      *string         `json:"place_kind"`
			RegistrationID *int64          `json:"usergear_registration_id"`
			ParentPlaceID  json.RawMessage `json:"parent_place_id"` // absent, null or number
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		parentGiven, parentID, err := clearableID(body.ParentPlaceID, "parent_place_id")
		if err != nil {
			return err
		}
		kind := ""
		if body.PlaceKind != nil {
			kind = *body.PlaceKind
		}

		// 404: the registration and the parent, when the request names them.
		if kind == placeKindBag {
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
			}
		}
		var parent *placeRef
		if kind == placeKindBag && parentID != nil {
			if parent, err = findPlace(ctx, tx, loadoutID, *parentID); err != nil {
				return err
			}
		}

		// 400: the request itself.
		if kind != placeKindBag && kind != placeKindLastMinute {
			return badRequest(`place_kind must be "bag" or "last_minute"`)
		}
		if kind == placeKindBag {
			if body.RegistrationID == nil {
				return badRequest("usergear_registration_id is required for a bag")
			}
			if parent != nil && parent.kind != placeKindBag {
				return badRequest("a bag can only be inside another bag; Worn and Last minute hold items, not bags")
			}
		}

		// 409: the state of the loadout.
		if kind == placeKindLastMinute {
			var exists bool
			if err := tx.QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM loadout_places WHERE loadoutId = ? AND placeKind = 'last_minute')`,
				loadoutID).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return &opError{http.StatusConflict, "the loadout already has a Last minute place"}
			}
			res, err := tx.ExecContext(ctx,
				`INSERT INTO loadout_places (loadoutId, placeKind) VALUES (?, 'last_minute')`, loadoutID)
			if err != nil {
				return err
			}
			return readCreatedPlace(ctx, tx, res, loadoutID, &created)
		}

		var already bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM loadout_places WHERE loadoutId = ? AND placeKind = 'bag' AND registrationId = ?)`,
			loadoutID, *body.RegistrationID).Scan(&already); err != nil {
			return err
		}
		if already {
			return &opError{http.StatusConflict, "this registration is already a bag of the loadout"}
		}
		capable, err := registrationCapable(ctx, tx, *body.RegistrationID)
		if err != nil {
			return err
		}
		if !capable {
			return &opError{http.StatusConflict, "this gear cannot hold items, so it cannot be a bag"}
		}
		if parent != nil {
			if err := requireCapableBag(ctx, tx, parent.registrationID); err != nil {
				return err
			}
		}

		// Parent: explicit (checked above), explicit null = top level, absent =
		// the closet default, skipped when it cannot hold items (BR-024 AC-4).
		var parentArg any
		switch {
		case parentID != nil:
			parentArg = *parentID
		case !parentGiven:
			def, err := closetDefaultParent(ctx, tx, loadoutID, *body.RegistrationID)
			if err != nil {
				return err
			}
			if def != nil {
				parentArg = *def
			}
		}

		var next int64
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(position), 0) + 1 FROM loadout_places WHERE loadoutId = ?`, loadoutID).Scan(&next); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO loadout_places (loadoutId, placeKind, registrationId, parentPlaceId, position)
			 VALUES (?, 'bag', ?, ?, ?)`, loadoutID, *body.RegistrationID, parentArg, next)
		if err != nil {
			return err
		}
		return readCreatedPlace(ctx, tx, res, loadoutID, &created)
	})
	if err != nil {
		respondPlaceError(c, log, err)
		return
	}

	c.JSON(http.StatusCreated, created)
}

func readCreatedPlace(ctx context.Context, q sqlRunner, res sql.Result, loadoutID int64, out **models.LoadoutPlace) error {
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p, err := LoadoutPlaceByID(ctx, q, loadoutID, id)
	if err == nil && p == nil {
		err = fmt.Errorf("inserted place %d not readable", id)
	}
	*out = p
	return err
}

// closetDefaultParent returns the bag of loadoutID that is the registration's
// closet direct container, only if that bag's gear can hold items now.
func closetDefaultParent(ctx context.Context, q sqlRunner, loadoutID, registrationID int64) (*int64, error) {
	var placeID, containerReg int64
	err := q.QueryRowContext(ctx,
		`SELECT p.placeId, p.registrationId FROM user_container_registration ucr
		 JOIN loadout_places p ON p.registrationId = ucr.userContainerId AND p.loadoutId = ? AND p.placeKind = 'bag'
		 WHERE ucr.userGearRegistrationId = ?
		 ORDER BY ucr.containerRegistrationId LIMIT 1`, loadoutID, registrationID).Scan(&placeID, &containerReg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	capable, err := registrationCapable(ctx, q, containerReg)
	if err != nil || !capable {
		return nil, err
	}
	return &placeID, nil
}

// UpdateLoadoutPlace changes the parent of a bag.
//
//	@Summary		Update loadout bag parent
//	@Description	Put a bag of a loadout owned by the caller inside another bag of the same loadout (parent_place_id a number) or at top level (null). parent_place_id is required. Never writes a closet container link. An unknown or foreign place or parent is 404; a place that is not a bag, a bag inside itself, or a parent that is Worn or Last minute is 400; a parent that is inside this bag (a cycle) or a new parent bag whose gear cannot hold items is 409. Leaving the parent as it is succeeds.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int							true	"Loadout ID"
//	@Param			place	path		int							true	"Place ID"
//	@Param			body	body		models.LoadoutPlaceUpdate	true	"parent_place_id (number or null, required)"
//	@Success		200		{object}	models.LoadoutPlace
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		409		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/place/{place}/update [post]
func UpdateLoadoutPlace(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}
	placeID, ok := parseLoadoutIDParam(c, "place")
	if !ok {
		return
	}
	data, ok := readBody(c, log)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	var updated *models.LoadoutPlace
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		if err := ownedLoadout(ctx, tx, loadoutID, userID); err != nil {
			return err
		}
		place, err := findPlace(ctx, tx, loadoutID, placeID)
		if err != nil {
			return err
		}
		var body struct {
			ParentPlaceID json.RawMessage `json:"parent_place_id"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		given, parentID, err := clearableID(body.ParentPlaceID, "parent_place_id")
		if err != nil {
			return err
		}
		var parent *placeRef
		if parentID != nil {
			if parent, err = findPlace(ctx, tx, loadoutID, *parentID); err != nil {
				return err
			}
		}

		if place.kind != placeKindBag {
			return badRequest("only a bag can have a parent")
		}
		if !given {
			return badRequest("parent_place_id is required (a bag's place ID, or null for top level)")
		}
		if parentID != nil && *parentID == placeID {
			return badRequest("a bag cannot be inside itself")
		}
		if parent != nil && parent.kind != placeKindBag {
			return badRequest("a bag can only be inside another bag; Worn and Last minute hold items, not bags")
		}

		unchanged := (parentID == nil && place.parentID == nil) ||
			(parentID != nil && place.parentID != nil && *parentID == *place.parentID)
		if parentID != nil && !unchanged {
			inside, err := isInsidePlace(ctx, tx, placeID, *parentID)
			if err != nil {
				return err
			}
			if inside {
				return &opError{http.StatusConflict, "a bag cannot be put inside one of its own contents"}
			}
			if err := requireCapableBag(ctx, tx, parent.registrationID); err != nil {
				return err
			}
		}

		if !unchanged {
			var arg any
			if parentID != nil {
				arg = *parentID
			}
			res, err := tx.ExecContext(ctx,
				`UPDATE loadout_places SET parentPlaceId = ? WHERE placeId = ? AND loadoutId = ?`, arg, placeID, loadoutID)
			if err := affectedOne(res, err, errPlaceNotFound); err != nil {
				return err
			}
		}
		updated, err = LoadoutPlaceByID(ctx, tx, loadoutID, placeID)
		if err == nil && updated == nil {
			err = errPlaceNotFound
		}
		return err
	})
	if err != nil {
		respondPlaceError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// isInsidePlace reports whether startID is placeID or sits (directly or
// through other bags) inside it.
func isInsidePlace(ctx context.Context, q sqlRunner, placeID, startID int64) (bool, error) {
	var inside bool
	err := q.QueryRowContext(ctx,
		`WITH RECURSIVE up(id) AS (
		   SELECT ?
		   UNION
		   SELECT p.parentPlaceId FROM loadout_places p JOIN up ON p.placeId = up.id WHERE p.parentPlaceId IS NOT NULL)
		 SELECT EXISTS(SELECT 1 FROM up WHERE id = ?)`, startID, placeID).Scan(&inside)
	return inside, err
}

// OrderLoadoutPlaces sets the order of the bags of a loadout.
//
//	@Summary		Order loadout bags
//	@Description	Set the order of the bags of a loadout owned by the caller. place_ids must list every bag of the loadout exactly once. An ID that is not a place of this loadout is 404; Worn or Last minute, a duplicate, or a missing bag is 400. Returns the bags in the new order.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			body	body		models.LoadoutPlaceOrder	true	"Every bag ID once"
//	@Success		200		{array}		models.LoadoutPlace
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/place/order [post]
func OrderLoadoutPlaces(c *gin.Context) {
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
	bags := []models.LoadoutPlace{}
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		if err := ownedLoadout(ctx, tx, loadoutID, userID); err != nil {
			return err
		}
		var body struct {
			PlaceIDs *[]int64 `json:"place_ids"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}

		kinds := map[int64]string{}
		rows, err := tx.QueryContext(ctx, `SELECT placeId, placeKind FROM loadout_places WHERE loadoutId = ?`, loadoutID)
		if err != nil {
			return err
		}
		bagCount := 0
		for rows.Next() {
			var id int64
			var kind string
			if err := rows.Scan(&id, &kind); err != nil {
				rows.Close()
				return err
			}
			kinds[id] = kind
			if kind == placeKindBag {
				bagCount++
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		if body.PlaceIDs == nil {
			return badRequest("place_ids is required")
		}
		ids := *body.PlaceIDs
		for _, id := range ids { // 404 first: any ID that is not a place of this loadout
			if _, ok := kinds[id]; !ok {
				return errPlaceNotFound
			}
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if kinds[id] != placeKindBag {
				return badRequest("place_ids may list bags only")
			}
			if seen[id] {
				return badRequest("place_ids lists a bag twice")
			}
			seen[id] = true
		}
		if len(ids) != bagCount {
			return badRequest("place_ids must list every bag of the loadout")
		}

		for i, id := range ids {
			res, err := tx.ExecContext(ctx,
				`UPDATE loadout_places SET position = ? WHERE placeId = ? AND loadoutId = ?`, int64(i+1), id, loadoutID)
			if err := affectedOne(res, err, errPlaceNotFound); err != nil {
				return err
			}
		}
		places, err := LoadoutPlaces(ctx, tx, loadoutID)
		if err != nil {
			return err
		}
		for _, p := range places {
			if p.PlaceKind == placeKindBag {
				bags = append(bags, p)
			}
		}
		return nil
	})
	if err != nil {
		respondPlaceError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, bags)
}

// CloseLoadoutPlace sets whether a place is closed.
//
//	@Summary		Close or open loadout place
//	@Description	Set (not toggle) the closed state of Worn, Last minute or a bag of a loadout owned by the caller, whatever the ticks of its items. Setting the value it already has succeeds. Never changes a tick or the closed state of another place. A place ID of 0 or a non-number ("no place" cannot be closed) is 400; an unknown or foreign place is 404; a missing or non-boolean closed is 400.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int						true	"Loadout ID"
//	@Param			place	path		int						true	"Place ID"
//	@Param			body	body		models.LoadoutPlaceClose	true	"closed (boolean, required)"
//	@Success		200		{object}	models.LoadoutPlaceCloseResult
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/place/{place}/close [post]
func CloseLoadoutPlace(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}
	placeID, err := strconv.ParseInt(c.Param("place"), 10, 64)
	if err != nil || placeID == 0 {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "invalid place ID: no place cannot be closed"})
		return
	}
	data, ok := readBody(c, log)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	var closed bool
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		if err := ownedLoadout(ctx, tx, loadoutID, userID); err != nil {
			return err
		}
		if _, err := findPlace(ctx, tx, loadoutID, placeID); err != nil {
			return err
		}
		var body struct {
			Closed *bool `json:"closed"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return badRequest(err.Error())
		}
		if body.Closed == nil {
			return badRequest("closed (true or false) is required")
		}
		closed = *body.Closed
		res, err := tx.ExecContext(ctx,
			`UPDATE loadout_places SET closed = ? WHERE placeId = ? AND loadoutId = ?`, closed, placeID, loadoutID)
		return affectedOne(res, err, errPlaceNotFound)
	})
	if err != nil {
		respondPlaceError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.LoadoutPlaceCloseResult{PlaceID: placeID, Closed: closed})
}

// DeleteLoadoutPlace removes a bag or Last minute from a loadout.
//
//	@Summary		Delete loadout place
//	@Description	Remove a bag or Last minute from a loadout owned by the caller. Items directly in it get no place, bags directly in it become top level, and its closed state ends; nothing else is removed and the registration stays in the closet. Worn cannot be removed (400). An unknown or foreign place is 404.
//	@Security		BearerAuth
//	@Tags			Loadouts
//	@Accept			json
//	@Produce		json
//	@Param			loadout	path		int	true	"Loadout ID"
//	@Param			place	path		int	true	"Place ID"
//	@Success		200		{object}	models.Status
//	@Failure		400		{object}	models.Error
//	@Failure		404		{object}	models.Error
//	@Failure		500		{object}	models.Error
//	@Router			/api/v1/loadout/{loadout}/place/{place}/delete [delete]
func DeleteLoadoutPlace(c *gin.Context) {
	log := c.MustGet("logger").(*zap.SugaredLogger)
	db := c.MustGet("db").(*sql.DB)
	userID := c.MustGet("user_id_int64").(int64)

	loadoutID, ok := parseLoadoutIDParam(c, "loadout")
	if !ok {
		return
	}
	placeID, ok := parseLoadoutIDParam(c, "place")
	if !ok {
		return
	}

	ctx := c.Request.Context()
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		if err := ownedLoadout(ctx, tx, loadoutID, userID); err != nil {
			return err
		}
		place, err := findPlace(ctx, tx, loadoutID, placeID)
		if err != nil {
			return err
		}
		if place.kind == placeKindWorn {
			return badRequest("Worn cannot be removed")
		}
		// The foreign-key actions unplace the items and lift the child bags.
		res, err := tx.ExecContext(ctx, `DELETE FROM loadout_places WHERE placeId = ? AND loadoutId = ?`, placeID, loadoutID)
		return affectedOne(res, err, errPlaceNotFound)
	})
	if err != nil {
		respondPlaceError(c, log, err)
		return
	}

	c.JSON(http.StatusOK, models.Status{Status: "success"})
}
