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
// is read through COALESCE and the API shows no slug as "".
const loadoutColumns = `loadoutId, userId, loadoutName, loadoutDescription, loadoutIsPublic, COALESCE(loadoutSlug, ''), totalWeight, createdAt, updatedAt`

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

// LoadoutItemsByLoadout returns all items belonging to a loadout. The result
// is never nil, so it encodes as [] when empty. It does not check ownership:
// use it only for a loadout already known to be public.
func LoadoutItemsByLoadout(db *sql.DB, loadoutID int64) (*[]models.LoadoutItem, error) {
	rows, err := db.Query(
		`SELECT loadoutItemId, loadoutId, gearId, quantity, notes FROM loadout_items WHERE loadoutId = ? ORDER BY loadoutItemId`, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("query loadout items: %w", err)
	}
	return scanLoadoutItems(rows)
}

// LoadoutItemsOwned returns the items of loadoutID only when it belongs to
// userID; the owner is bound in the query itself (OC-001 G-2), so the result
// is empty for anyone else's loadout.
func LoadoutItemsOwned(ctx context.Context, db *sql.DB, loadoutID, userID int64) (*[]models.LoadoutItem, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT li.loadoutItemId, li.loadoutId, li.gearId, li.quantity, li.notes
		 FROM loadout_items li
		 JOIN loadouts l ON l.loadoutId = li.loadoutId AND l.userId = ?
		 WHERE li.loadoutId = ? ORDER BY li.loadoutItemId`, userID, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("query loadout items: %w", err)
	}
	return scanLoadoutItems(rows)
}

func scanLoadoutItems(rows *sql.Rows) (*[]models.LoadoutItem, error) {
	defer rows.Close()
	items := []models.LoadoutItem{}
	for rows.Next() {
		var it models.LoadoutItem
		if err := rows.Scan(&it.LoadoutItemID, &it.LoadoutID, &it.GearID, &it.Quantity, &it.Notes); err != nil {
			return nil, fmt.Errorf("scan loadout item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &items, nil
}

// LoadoutRecalculateWeight updates totalWeight based on gear weights and
// quantities. Pass the *sql.Tx of the mutation so both commit or roll back
// together (OC-001 G-5).
func LoadoutRecalculateWeight(ctx context.Context, q sqlRunner, loadoutID int64) error {
	const stmt = `UPDATE loadouts SET totalWeight = (
        SELECT IFNULL(SUM(g.gearWeight * li.quantity), 0)
        FROM loadout_items li
        JOIN gear g ON g.gearId = li.gearId
        WHERE li.loadoutId = ?
    ) WHERE loadoutId = ?`
	if _, err := q.ExecContext(ctx, stmt, loadoutID, loadoutID); err != nil {
		return fmt.Errorf("recalculate weight for loadout %d: %w", loadoutID, err)
	}
	return nil
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
