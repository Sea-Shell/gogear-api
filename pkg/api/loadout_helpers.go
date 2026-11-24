package endpoints

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
	sqlite3 "github.com/mattn/go-sqlite3"
)

// sqlRunner is satisfied by both *sql.DB and *sql.Tx, so a helper can run
// inside or outside a transaction.
type sqlRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Loadout column list. loadoutSlug is nullable (NULL means "no slug"), so it
// is read through COALESCE and the API shows no slug as "". The on-your-back
// weight (BR-029 AC-3) is not stored and SQLite cannot add it exactly, so the
// statement returns its terms as two JSON arrays of [quantity, unitWeight] pairs
// (items, then bags) from the loadout_on_back_*_terms views, and sumOnBackTerms
// adds them. The terms, the counts and the closed places come from the same
// statement, so one row is one snapshot, and a list needs no per-loadout item
// read (BR-029 AC-4).
// closed_places is a JSON array in the order of the place list (Worn, Last
// minute, bags by position); json_group_array gives [] when none is closed.
// doneAt is read only while the stage is done, so a loadout that is not done
// never reports a time (BR-028 AC-3).
const loadoutColumns = `loadoutId, userId, loadoutName, loadoutDescription, loadoutIsPublic, COALESCE(loadoutSlug, ''),
	(SELECT json_group_array(json_array(t.quantity, t.unitWeight)) FROM loadout_on_back_item_terms t WHERE t.loadoutId = loadouts.loadoutId),
	(SELECT json_group_array(json_array(t.quantity, t.unitWeight)) FROM loadout_on_back_bag_terms t WHERE t.loadoutId = loadouts.loadoutId),
	(SELECT COUNT(*) FROM loadout_items li WHERE li.loadoutId = loadouts.loadoutId),
	(SELECT COUNT(*) FROM loadout_items li WHERE li.loadoutId = loadouts.loadoutId AND li.ticked = 1),
	(SELECT COUNT(*) FROM loadout_places p WHERE p.loadoutId = loadouts.loadoutId AND p.placeKind = 'bag'),
	(SELECT json_group_array(json_object('place_id', p.placeId, 'place_kind', p.placeKind,
	        'name', CASE p.placeKind WHEN 'worn' THEN 'Worn' WHEN 'last_minute' THEN 'Last minute' ELSE COALESCE(g.gearName, '') END)
	        ORDER BY CASE p.placeKind WHEN 'worn' THEN 0 WHEN 'last_minute' THEN 1 ELSE 2 END, p.position, p.placeId)
	 FROM loadout_places p
	 LEFT JOIN user_gear_registrations r ON r.userGearRegistrationId = p.registrationId
	 LEFT JOIN gear g ON g.gearId = r.gearId
	 WHERE p.loadoutId = loadouts.loadoutId AND p.closed = 1),
	stage, CASE WHEN stage = 'done' THEN doneAt END, startDate, endDate, weightGoal,
	createdAt, updatedAt`

type rowScanner interface{ Scan(dest ...any) error }

// sumOnBackTerms adds quantity x unitWeight over the JSON arrays of pairs that
// loadoutColumns returns (items, then bags), exactly: math/big has no overflow,
// so nothing is capped, clamped or rounded. Every value must be an integer
// literal; a fraction, exponent, string or null is an error, never rounded.
func sumOnBackTerms(arrays ...string) (models.WeightTotal, error) {
	sum, product := new(big.Int), new(big.Int)
	for _, terms := range arrays {
		dec := json.NewDecoder(strings.NewReader(terms))
		dec.UseNumber() // numbers stay their literal text; they are never a float64
		var pairs [][]any
		if err := dec.Decode(&pairs); err != nil {
			return models.WeightTotal{}, fmt.Errorf("decode on-back weight terms: %w", err)
		}
		for _, pair := range pairs {
			if len(pair) != 2 {
				return models.WeightTotal{}, fmt.Errorf("on-back weight term %v is not [quantity, unitWeight]", pair)
			}
			var factors [2]big.Int
			for i, v := range pair {
				lit, isNumber := v.(json.Number)
				if _, ok := factors[i].SetString(string(lit), 10); !isNumber || !ok {
					return models.WeightTotal{}, fmt.Errorf("on-back weight term value %v is not an integer", v)
				}
			}
			sum.Add(sum, product.Mul(&factors[0], &factors[1]))
		}
	}
	return models.NewWeightTotal(sum), nil
}

func scanLoadout(r rowScanner) (*models.Loadout, error) {
	var l models.Loadout
	var itemTerms, bagTerms, closed string
	if err := r.Scan(&l.LoadoutID, &l.UserID, &l.LoadoutName, &l.LoadoutDescription,
		&l.LoadoutIsPublic, &l.LoadoutSlug, &itemTerms, &bagTerms, &l.ItemCount, &l.TickedCount, &l.BagCount,
		&closed, &l.Stage, &l.DoneAt, &l.StartDate, &l.EndDate, &l.WeightGoal,
		&l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}
	var err error
	if l.TotalWeight, err = sumOnBackTerms(itemTerms, bagTerms); err != nil {
		return nil, err
	}
	l.ClosedPlaces = []models.ClosedPlace{}
	if err := json.Unmarshal([]byte(closed), &l.ClosedPlaces); err != nil {
		return nil, fmt.Errorf("decode closed places: %w", err)
	}
	return &l, nil
}

// LoadoutGetOwned returns the loadout only if it is owned by userID.
// Unknown and foreign loadouts both give nil, nil (OC-001 G-2).
func LoadoutGetOwned(ctx context.Context, q sqlRunner, loadoutID, userID int64) (*models.Loadout, error) {
	l, err := scanLoadout(q.QueryRowContext(ctx,
		`SELECT `+loadoutColumns+` FROM loadouts WHERE loadoutId = ? AND userId = ?`, loadoutID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query owned loadout: %w", err)
	}
	return l, nil
}

// LoadoutGetBySlug fetches a loadout by its slug.
// Returns nil, nil if not found. A blank slug never matches anything.
func LoadoutGetBySlug(db *sql.DB, slug string) (*models.Loadout, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, nil
	}
	l, err := scanLoadout(db.QueryRow(
		`SELECT `+loadoutColumns+` FROM loadouts WHERE loadoutSlug = ?`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query loadout by slug: %w", err)
	}
	return l, nil
}

// LoadoutListByUser returns all loadouts for a given user ordered by most
// recent update. The result is never nil, so it encodes as [] when empty.
func LoadoutListByUser(db *sql.DB, userID int64) (*[]models.Loadout, error) {
	rows, err := db.Query(
		`SELECT `+loadoutColumns+` FROM loadouts WHERE userId = ? ORDER BY updatedAt DESC, loadoutId DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query loadouts by user: %w", err)
	}
	defer rows.Close()

	loadouts := []models.Loadout{}
	for rows.Next() {
		l, err := scanLoadout(rows)
		if err != nil {
			return nil, fmt.Errorf("scan loadout: %w", err)
		}
		loadouts = append(loadouts, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &loadouts, nil
}

// itemSelect is the one read of a loadout item (ARS-001 section 3). Names and
// the catalog weight are live from the current gear (the registration's gear,
// or detachedGearId for a legacy item); an item whose registration was
// deleted shows its snapshot name and no other live gear value. Weights come
// from the loadout_item_weight view, the only definition of them. The
// registration's own weight and count are live and null when the item has no
// registration (the LEFT JOIN finds nothing). consumable is the item's own
// role in this loadout (BR-027).
const itemSelect = `SELECT li.loadoutItemId, li.loadoutId,
	COALESCE(r.gearId, li.detachedGearId),
	li.quantity, li.notes, li.registrationId, li.registrationId IS NOT NULL, li.detachedReason,
	COALESCE(li.detachedGearName, g.gearName, ''),
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL ELSE m.manufactureName END,
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL ELSE c.categoryName END,
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL ELSE t.topCategoryName END,
	li.placeId, li.ticked,
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL WHEN g.gearWeight > 0 THEN g.gearWeight END,
	iw.unitWeight, COALESCE(iw.weightSource, 'none'),
	r.measuredWeight, r.measuredWeightSource, r.closetCount, li.consumable
FROM loadout_items li
LEFT JOIN user_gear_registrations r ON r.userGearRegistrationId = li.registrationId
LEFT JOIN gear g ON g.gearId = COALESCE(r.gearId, li.detachedGearId)
LEFT JOIN manufacture m ON m.manufactureId = g.gearManufactureId
LEFT JOIN gear_category c ON c.categoryId = g.gearCategoryId
LEFT JOIN gear_top_category t ON t.topCategoryId = g.gearTopCategoryId
LEFT JOIN loadout_item_weight iw ON iw.loadoutItemId = li.loadoutItemId`

func scanLoadoutItem(r rowScanner) (*models.LoadoutItem, error) {
	var it models.LoadoutItem
	if err := r.Scan(&it.LoadoutItemID, &it.LoadoutID, &it.GearID, &it.Quantity, &it.Notes,
		&it.UsergearRegistrationID, &it.RegistrationExists, &it.DetachedReason,
		&it.GearName, &it.ManufactureName, &it.CategoryName, &it.TopCategoryName,
		&it.PlaceID, &it.Ticked, &it.CatalogWeight, &it.UnitWeight, &it.WeightSource,
		&it.MeasuredWeight, &it.MeasuredWeightSource, &it.ClosetCount, &it.Consumable); err != nil {
		return nil, err
	}
	return &it, nil
}

// LoadoutItemByID reads one item of loadoutID inside the caller's runner (nil,
// nil when it is not an item of that loadout). It does not check ownership.
func LoadoutItemByID(ctx context.Context, q sqlRunner, loadoutID, itemID int64) (*models.LoadoutItem, error) {
	it, err := scanLoadoutItem(q.QueryRowContext(ctx,
		itemSelect+` WHERE li.loadoutItemId = ? AND li.loadoutId = ?`, itemID, loadoutID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query loadout item: %w", err)
	}
	return it, nil
}

// LoadoutItemsOwned returns the items of loadoutID only when it belongs to
// userID; the owner is bound in the query itself (OC-001 G-2), so the result
// is empty for anyone else's loadout. The result is never nil, so it encodes
// as [] when empty.
func LoadoutItemsOwned(ctx context.Context, db sqlRunner, loadoutID, userID int64) (*[]models.LoadoutItem, error) {
	rows, err := db.QueryContext(ctx,
		itemSelect+` JOIN loadouts l ON l.loadoutId = li.loadoutId AND l.userId = ?
		 WHERE li.loadoutId = ? ORDER BY li.loadoutItemId`, userID, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("query loadout items: %w", err)
	}
	defer rows.Close()
	items := []models.LoadoutItem{}
	for rows.Next() {
		it, err := scanLoadoutItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan loadout item: %w", err)
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &items, nil
}

// LoadoutItemsPublic returns the items of a loadout already known to be public.
// An item is listed only if it has a current gear row (BR-016 AC-5). The result
// is never nil, so it encodes as [] when empty.
func LoadoutItemsPublic(db *sql.DB, loadoutID int64) (*[]models.LoadoutItemPublic, error) {
	rows, err := db.Query(
		`SELECT li.loadoutItemId, li.loadoutId, g.gearId, li.quantity, li.notes, COALESCE(li.detachedGearName, g.gearName)
		 FROM loadout_items li
		 LEFT JOIN user_gear_registrations r ON r.userGearRegistrationId = li.registrationId
		 JOIN gear g ON g.gearId = COALESCE(r.gearId, li.detachedGearId)
		 WHERE li.loadoutId = ? ORDER BY li.loadoutItemId`, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("query public loadout items: %w", err)
	}
	defer rows.Close()
	items := []models.LoadoutItemPublic{}
	for rows.Next() {
		var it models.LoadoutItemPublic
		if err := rows.Scan(&it.LoadoutItemID, &it.LoadoutID, &it.GearID, &it.Quantity, &it.Notes, &it.GearName); err != nil {
			return nil, fmt.Errorf("scan public loadout item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &items, nil
}

// The stage, trip dates and weight goal of a loadout (BR-028). The request
// body carries them as raw JSON so a write can tell "not named" from null and
// refuse a wrong type with 400 instead of dropping the value.

// optionalValue is a clearable field of a write: Given is false when the body
// did not name it, and Value is nil when it named null (remove the value).
type optionalValue[T any] struct {
	Given bool
	Value *T
}

// loadoutTripBody is embedded in the request bodies of loadout insert and
// update; encoding/json reads its fields as if they were the body's own.
type loadoutTripBody struct {
	Stage      json.RawMessage `json:"stage"`
	StartDate  json.RawMessage `json:"start_date"`
	EndDate    json.RawMessage `json:"end_date"`
	WeightGoal json.RawMessage `json:"weight_goal"`
}

// loadoutTrip is a validated loadoutTripBody.
type loadoutTrip struct {
	stage      *string // nil: the request does not change the stage
	start, end optionalValue[string]
	goal       optionalValue[int64]
}

// parse validates the fields. On update (update = true) null removes a date or
// the goal. On insert null is the same as absent, and the only stage that may
// be named is "planning", because a new loadout always starts there. A null
// stage is never a change. Every failure is a 400 message naming the field.
func (b loadoutTripBody) parse(update bool) (loadoutTrip, error) {
	var t loadoutTrip
	if b.Stage != nil && !isJSONNull(b.Stage) {
		var stage string
		if err := json.Unmarshal(b.Stage, &stage); err != nil || !validStage(stage) {
			return t, badRequest(`stage must be "planning", "packing" or "done"`)
		}
		if !update && stage != "planning" {
			return t, badRequest(`a new loadout starts in stage "planning"`)
		}
		t.stage = &stage
	}
	var err error
	if t.start, err = parseDateField(b.StartDate, "start_date", update); err != nil {
		return t, err
	}
	if t.end, err = parseDateField(b.EndDate, "end_date", update); err != nil {
		return t, err
	}
	if b.WeightGoal != nil && (update || !isJSONNull(b.WeightGoal)) {
		t.goal.Given = true
		if !isJSONNull(b.WeightGoal) {
			goal, ok := wholeGrams(b.WeightGoal)
			if !ok {
				return t, badRequest("weight_goal must be a whole number of grams, at least 1")
			}
			t.goal.Value = &goal
		}
	}
	return t, nil
}

// sqlValue is the value to bind: the stored text or number, or NULL.
func (v optionalValue[T]) sqlValue() any {
	if v.Value == nil {
		return nil
	}
	return *v.Value
}

func validStage(s string) bool { return s == "planning" || s == "packing" || s == "done" }

// parseDateField reads a calendar date written YYYY-MM-DD, which the column
// CHECK (SQLite's date() gives the text back unchanged) must also accept;
// TestLoadoutDates_EdgeDatesAreStored covers the ends of the calendar.
func parseDateField(raw json.RawMessage, field string, update bool) (optionalValue[string], error) {
	var v optionalValue[string]
	if raw == nil || (isJSONNull(raw) && !update) {
		return v, nil
	}
	v.Given = true
	if isJSONNull(raw) {
		return v, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return v, badRequest(field + " must be a calendar date written YYYY-MM-DD")
	}
	if d, err := time.Parse(time.DateOnly, s); err != nil || d.Format(time.DateOnly) != s {
		return v, badRequest(field + " must be a calendar date written YYYY-MM-DD")
	}
	v.Value = &s
	return v, nil
}

// checkDateOrder refuses an end date before the start date, judged on the
// values after the write: what the request names, else what is stored now
// (BR-028 AC-4, ARS-001 milestone B). YYYY-MM-DD text sorts as dates do.
func (t loadoutTrip) checkDateOrder(storedStart, storedEnd *string) error {
	start, end := storedStart, storedEnd
	if t.start.Given {
		start = t.start.Value
	}
	if t.end.Given {
		end = t.end.Value
	}
	if start != nil && end != nil && *end < *start {
		return badRequest("end_date is before start_date")
	}
	return nil
}

// optionalBool reads a boolean field that is not clearable: absent and null
// both mean "not named", and anything but true or false is a 400.
func optionalBool(raw json.RawMessage, field string) (*bool, error) {
	if raw == nil || isJSONNull(raw) {
		return nil, nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, badRequest(field + " must be true or false")
	}
	return &v, nil
}

// nullableSlug maps a blank slug to NULL so the UNIQUE index never sees "".
func nullableSlug(s string) any {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var se sqlite3.Error
	return errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintUnique
}

func isTriggerViolation(err error) bool {
	var se sqlite3.Error
	return errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintTrigger
}

func isForeignKeyViolation(err error) bool {
	var se sqlite3.Error
	return errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintForeignKey
}

// errGearNotFound is the answer when a request names catalog gear that does
// not exist (registration insert, legacy loadout item insert). Any other
// foreign-key refusal of a write is 400 (see gearReferenceError).
var errGearNotFound = &opError{http.StatusNotFound, "Gear not found"}

// gearReferenceError turns a foreign-key failure of a write that names gearID
// into the response error: 404 when the gear does not exist, 400 when it does
// and some other reference is invalid. Other errors pass through unchanged.
// q must be usable after the failed statement (a transaction is).
func gearReferenceError(ctx context.Context, q sqlRunner, gearID int64, err error) error {
	if !isForeignKeyViolation(err) {
		return err
	}
	var exists bool
	if qerr := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gear WHERE gearId = ?)`, gearID).Scan(&exists); qerr != nil {
		return err
	}
	if exists {
		return badRequest("a referenced record does not exist")
	}
	return errGearNotFound
}

// opError is a refusal decided inside a transaction. Returning it rolls the
// transaction back and becomes the response.
type opError struct {
	status int
	msg    string
}

func (e *opError) Error() string { return e.msg }

var (
	errLoadoutNotFound = &opError{http.StatusNotFound, "Loadout not found"}
	errItemNotFound    = &opError{http.StatusNotFound, "Loadout item not found"}
	// One body for a place that does not exist or belongs to another loadout
	// (BR-024 AC-11); registrations use errRegistrationNotFound likewise.
	errPlaceNotFound = &opError{http.StatusNotFound, "Place not found"}
)

func badRequest(msg string) *opError { return &opError{http.StatusBadRequest, msg} }

// inTx runs fn in one transaction, rolling back on any error or panic. The
// database is opened with _txlock=immediate (utils.SQLiteDSN), so concurrent
// writers queue on the busy timeout instead of failing.
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	// Rollback after a successful Commit is a no-op (sql.ErrTxDone); it only
	// matters on error returns and panics, which must not leave the lock held.
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// respondOpError writes the response for an error from inTx: a refusal keeps
// its status, a unique-slug clash is 409, anything else is a logged 500 with a
// generic body.
func respondOpError(c *gin.Context, log interface{ Errorf(string, ...any) }, err error) {
	var oe *opError
	switch {
	case errors.As(err, &oe):
		c.IndentedJSON(oe.status, models.Error{Error: oe.msg})
	case isUniqueViolation(err):
		c.IndentedJSON(http.StatusConflict, models.Error{Error: "loadout slug already in use"})
	default:
		log.Errorf("loadout operation failed: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
	}
}
