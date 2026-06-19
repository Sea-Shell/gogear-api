package endpoints

import (
	"database/sql"
	"fmt"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// LoadoutGetBySlug fetches a loadout by its slug.
// Returns nil, nil if not found.
func LoadoutGetBySlug(db *sql.DB, slug string) (*models.Loadout, error) {
	const query = `SELECT loadoutId, userId, loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug, totalWeight, createdAt, updatedAt FROM loadouts WHERE loadoutSlug = ?`
	var l models.Loadout
	err := db.QueryRow(query, slug).Scan(
		&l.LoadoutID,
		&l.UserID,
		&l.LoadoutName,
		&l.LoadoutDescription,
		&l.LoadoutIsPublic,
		&l.LoadoutSlug,
		&l.TotalWeight,
		&l.CreatedAt,
		&l.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query loadout by slug: %w", err)
	}
	return &l, nil
}

// LoadoutListByUser returns all loadouts for a given user ordered by most recent update.
func LoadoutListByUser(db *sql.DB, userID int64) (*[]models.Loadout, error) {
	const query = `SELECT loadoutId, userId, loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug, totalWeight, createdAt, updatedAt FROM loadouts WHERE userId = ? ORDER BY updatedAt DESC`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("query loadouts by user: %w", err)
	}
	defer rows.Close()

	var loadouts []models.Loadout
	for rows.Next() {
		var l models.Loadout
		if err := rows.Scan(
			&l.LoadoutID,
			&l.UserID,
			&l.LoadoutName,
			&l.LoadoutDescription,
			&l.LoadoutIsPublic,
			&l.LoadoutSlug,
			&l.TotalWeight,
			&l.CreatedAt,
			&l.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan loadout: %w", err)
		}
		loadouts = append(loadouts, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &loadouts, nil
}

// LoadoutItemsByLoadout returns all items belonging to a loadout.
func LoadoutItemsByLoadout(db *sql.DB, loadoutID int64) (*[]models.LoadoutItem, error) {
	const query = `SELECT loadoutItemId, loadoutId, gearId, quantity, notes FROM loadout_items WHERE loadoutId = ?`
	rows, err := db.Query(query, loadoutID)
	if err != nil {
		return nil, fmt.Errorf("query loadout items: %w", err)
	}
	defer rows.Close()

	var items []models.LoadoutItem
	for rows.Next() {
		var it models.LoadoutItem
		if err := rows.Scan(
			&it.LoadoutItemID,
			&it.LoadoutID,
			&it.GearID,
			&it.Quantity,
			&it.Notes,
		); err != nil {
			return nil, fmt.Errorf("scan loadout item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &items, nil
}

// LoadoutRecalculateWeight updates totalWeight based on gear weights and quantities.
func LoadoutRecalculateWeight(db *sql.DB, loadoutID int64) error {
	// Assume gear table has gearWeight column (int64).
	const stmt = `UPDATE loadouts SET totalWeight = (
        SELECT IFNULL(SUM(g.gearWeight * li.quantity), 0)
        FROM loadout_items li
        JOIN gear g ON g.gearId = li.gearId
        WHERE li.loadoutId = ?
    ) WHERE loadoutId = ?`
	_, err := db.Exec(stmt, loadoutID, loadoutID)
	if err != nil {
		return fmt.Errorf("recalculate weight for loadout %d: %w", loadoutID, err)
	}
	return nil
}
