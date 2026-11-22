package endpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
// is read through COALESCE and the API shows no slug as "". total_weight is
// the on-your-back weight from the view (BR-029 AC-3); nothing stores it.
const loadoutColumns = `loadoutId, userId, loadoutName, loadoutDescription, loadoutIsPublic, COALESCE(loadoutSlug, ''),
	COALESCE((SELECT w.totalWeight FROM loadout_on_back_weight w WHERE w.loadoutId = loadouts.loadoutId), 0), createdAt, updatedAt`

type rowScanner interface{ Scan(dest ...any) error }

func scanLoadout(r rowScanner) (*models.Loadout, error) {
	var l models.Loadout
	if err := r.Scan(&l.LoadoutID, &l.UserID, &l.LoadoutName, &l.LoadoutDescription,
		&l.LoadoutIsPublic, &l.LoadoutSlug, &l.TotalWeight, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
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
// from the loadout_item_weight view, the only definition of them.
const itemSelect = `SELECT li.loadoutItemId, li.loadoutId,
	COALESCE(r.gearId, li.detachedGearId),
	li.quantity, li.notes, li.registrationId, li.registrationId IS NOT NULL, li.detachedReason,
	COALESCE(li.detachedGearName, g.gearName, ''),
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL ELSE m.manufactureName END,
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL ELSE c.categoryName END,
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL ELSE t.topCategoryName END,
	li.placeId, li.ticked,
	CASE WHEN li.detachedReason = 'registration_deleted' THEN NULL WHEN g.gearWeight > 0 THEN g.gearWeight END,
	iw.unitWeight, COALESCE(iw.weightSource, 'none')
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
		&it.PlaceID, &it.Ticked, &it.CatalogWeight, &it.UnitWeight, &it.WeightSource); err != nil {
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
