package endpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"

	models "github.com/Sea-Shell/gogear-api/pkg/models"

	gin "github.com/gin-gonic/gin"
)

// Shared SQL for gear registrations (inventory) and container links. Aliases:
// ugr registration, l container link, g gear, u owner, m manufacturer,
// t top category, c category. The caller's ID is always bound as a query
// argument; no caller-supplied text is ever placed in SQL.

var (
	errRegistrationNotFound = &opError{http.StatusNotFound, "Registration not found"}
	errContainerNotFound    = &opError{http.StatusNotFound, "Container not found"}
	errLinkNotFound         = &opError{http.StatusNotFound, "Container link not found"}
)

// userGearContainerExpr is the container status of a registration's gear: the
// derived value of the view gear_container_capability (migration 000009,
// ADR-001 D-3), joined as cap by userGearCapabilityJoin. A registration whose
// gear row is missing has no view row, hence the COALESCE. It is the reported
// value and the container filter's value alike (BR-021 AC-3, AC-7).
const userGearContainerExpr = `COALESCE(cap.isContainer, 0)`

// userGearCapabilityJoin attaches the view row of the registration's gear.
const userGearCapabilityJoin = `
	LEFT JOIN gear_container_capability cap ON cap.gearId = ugr.gearId`

// userGearColumns lists the columns in the field order of models.UserGear.
// Everything that comes from a LEFT JOIN or a nullable catalog column goes
// through COALESCE, so a NULL never breaks a scan; the catalog weight is the
// exception and stays NULL when it is 0 or missing (ADR-004 D-1). The link columns stay NULL
// when there is no link.
const userGearColumns = `ugr.userGearRegistrationId, ugr.gearId, ugr.userId, ugr.maxContainerWeight,
	COALESCE(u.userId, 0), COALESCE(u.userUsername, ''), COALESCE(u.userName, ''),
	COALESCE(g.gearId, 0), COALESCE(g.gearTopCategoryId, 0), COALESCE(g.gearCategoryId, 0), COALESCE(g.gearManufactureId, 0),
	COALESCE(g.gearName, ''), CASE WHEN g.gearWeight > 0 THEN g.gearWeight END, COALESCE(g.gearHeight, 0), COALESCE(g.gearLength, 0), COALESCE(g.gearWidth, 0),
	COALESCE(g.gearStatus, 0), ` + userGearContainerExpr + `,
	l.containerRegistrationId, l.userContainerId,
	COALESCE(m.manufactureId, 0), COALESCE(m.manufactureName, ''),
	COALESCE(t.topCategoryId, 0), COALESCE(t.topCategoryName, ''), COALESCE(t.topCategoryIcon, ''),
	COALESCE(c.categoryId, 0), COALESCE(c.categoryName, ''), COALESCE(c.categoryTopCategoryId, 0)`

// userGearCatalogJoins joins the one-to-one catalog and owner rows.
const userGearCatalogJoins = `
	LEFT JOIN gear g ON g.gearId = ugr.gearId
	LEFT JOIN users u ON u.userId = ugr.userId
	LEFT JOIN manufacture m ON m.manufactureId = g.gearManufactureId
	LEFT JOIN gear_top_category t ON t.topCategoryId = g.gearTopCategoryId
	LEFT JOIN gear_category c ON c.categoryId = g.gearCategoryId` + userGearCapabilityJoin

// userGearOwnLinkJoin attaches at most one container link to a registration:
// the lowest-numbered link whose container belongs to the same owner. A
// registration in several containers therefore yields one row, and a link into
// someone else's container (bad legacy data) is never shown.
const userGearOwnLinkJoin = `
	LEFT JOIN user_container_registration l ON l.containerRegistrationId = (
		SELECT MIN(l2.containerRegistrationId)
		FROM user_container_registration l2
		JOIN user_gear_registrations cr ON cr.userGearRegistrationId = l2.userContainerId
		WHERE l2.userGearRegistrationId = ugr.userGearRegistrationId AND cr.userId = ugr.userId)`

func scanUserGear(r rowScanner) (models.UserGear, error) {
	var x models.UserGear
	err := r.Scan(
		&x.UserGearRegistrationID, &x.UserGearGearID, &x.UserGearUserID, &x.MaxContainerWeight,
		&x.UserID, &x.UserUsername, &x.UserName,
		&x.GearID, &x.GearTopCategoryID, &x.GearCategoryID, &x.GearManufactureID,
		&x.GearName, &x.GearWeight, &x.GearHeight, &x.GearLength, &x.GearWidth,
		&x.GearStatus, &x.GearIsContainer,
		&x.ContainerLinkID, &x.ContainerID,
		&x.ManufactureID, &x.ManufactureName,
		&x.TopCategoryID, &x.TopCategoryName, &x.TopCategoryIcon,
		&x.CategoryID, &x.CategoryName, &x.CategoryTopCategoryID)
	return x, err
}

func scanUserGearRows(rows *sql.Rows) ([]models.UserGear, error) {
	defer rows.Close()
	out := []models.UserGear{} // never nil: encodes as [] when empty
	for rows.Next() {
		x, err := scanUserGear(rows)
		if err != nil {
			return nil, fmt.Errorf("scan registration: %w", err)
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return out, nil
}

// registrationOwned reports whether registration id belongs to userID.
func registrationOwned(ctx context.Context, q sqlRunner, id, userID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_gear_registrations WHERE userGearRegistrationId = ? AND userId = ?`, id, userID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check registration owner: %w", err)
	}
	return n > 0, nil
}

// parseInt64Param reads a numeric path value. A malformed ID is 400 before any
// ownership check, which reveals nothing about stored records.
func parseInt64Param(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "invalid " + name + " ID"})
		return 0, false
	}
	return id, true
}

// parsePaging reads page (default 1) and limit (default 30). Anything that is
// not a positive integer is 400.
func parsePaging(c *gin.Context) (page, limit int, ok bool) {
	page, limit = 1, 30
	if s := c.Query("page"); s != "" && s != "0" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid page number"})
			return 0, 0, false
		}
		page = v
	}
	if s := c.Query("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 {
			c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid limit number"})
			return 0, 0, false
		}
		limit = v
	}
	if page > math.MaxInt32 || limit > math.MaxInt32 {
		c.IndentedJSON(http.StatusBadRequest, models.Error{Error: "Invalid paging value"})
		return 0, 0, false
	}
	return page, limit, true
}

// pagePayload builds the list response with next/previous links.
func pagePayload(c *gin.Context, items []models.UserGear, total, page, limit int) models.ResponsePayload {
	p := models.ResponsePayload{
		TotalItemCount: total,
		CurrentPage:    page,
		ItemLimit:      limit,
		TotalPages:     int(math.Ceil(float64(total) / float64(limit))),
		Items:          items,
	}
	link := func(n int) *string {
		q := c.Request.URL.Query()
		q.Set("page", strconv.Itoa(n))
		s := (&url.URL{Path: c.Request.URL.Path, RawQuery: q.Encode()}).String()
		return &s
	}
	if page < p.TotalPages {
		p.NextPage = link(page + 1)
	}
	if page > 1 {
		p.PrevPage = link(page - 1)
	}
	return p
}

func respondInternal(c *gin.Context, log interface{ Errorf(string, ...any) }, what string, err error) {
	log.Errorf("%s: %v", what, err)
	c.IndentedJSON(http.StatusInternalServerError, models.Error{Error: "internal error"})
}

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
