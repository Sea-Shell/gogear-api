package endpoints

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	sqlite3driver "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func migrationsPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := filepath.Join(dir, "..", "..", "migrations")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("migrations directory not found at %s", path)
	}
	return path
}

func runMigrate(t *testing.T, db *sql.DB) {
	t.Helper()
	path := migrationsPath(t)
	driver, err := sqlite3driver.WithInstance(db, &sqlite3driver.Config{})
	if err != nil {
		t.Fatalf("driver init: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+path, "sqlite3", driver)
	if err != nil {
		t.Fatalf("migrate instance: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}
}

func tempDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open :memory: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedUser(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO users (userId, userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal) VALUES (?, ?, ?, ?, ?, 0, 0)`,
		id, "testuser"+itoa64(id), "pwd", "Test User", "test@example.com",
	)
	if err != nil {
		t.Fatalf("seed user %d: %v", id, err)
	}
}

// itoa64 is an int64→string helper avoiding strconv import for seeds.
func itoa64(i int64) string {
	return strings.TrimSpace(func() string {
		if i == 0 {
			return "0"
		}
		var buf [20]byte
		neg := i < 0
		if neg {
			i = -i
		}
		pos := len(buf)
		for i > 0 {
			pos--
			buf[pos] = byte('0' + i%10)
			i /= 10
		}
		if neg {
			pos--
			buf[pos] = '-'
		}
		return string(buf[pos:])
	}())
}

func seedGear(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO gear (gearId, gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight) VALUES (?, 1, 1, 1, 'test gear', 100)`,
		id,
	)
	if err != nil {
		t.Fatalf("seed gear %d: %v", id, err)
	}
}

func seedLoadout(t *testing.T, db *sql.DB, userID int64, isPublic bool, slug string) int64 {
	t.Helper()
	pub := 0
	if isPublic {
		pub = 1
	}
	res, err := db.Exec(
		`INSERT INTO loadouts (userId, loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug, totalWeight, createdAt, updatedAt) VALUES (?, 'test', '', ?, ?, 0, datetime('now'), datetime('now'))`,
		userID, pub, slug,
	)
	if err != nil {
		t.Fatalf("seed loadout user=%d public=%v slug=%s: %v", userID, isPublic, slug, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedLoadoutItem(t *testing.T, db *sql.DB, loadoutID, gearID int64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO loadout_items (loadoutId, gearId, quantity, notes) VALUES (?, ?, 1, '')`,
		loadoutID, gearID,
	)
	if err != nil {
		t.Fatalf("seed loadout item loadout=%d gear=%d: %v", loadoutID, gearID, err)
	}
}

// ---------------------------------------------------------------------------
// Test middleware
// ---------------------------------------------------------------------------

func testMiddleware(db *sql.DB, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Set("logger", logger)
		c.Next()
	}
}

func testAuthMiddleware(userID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", itoa64(userID))
		c.Set("user_id_int64", userID)
		c.Next()
	}
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

// setupTestWithUser creates a fresh :memory: DB with migrations applied,
// a Gin engine in test mode, and a test user seeded. All protected routes
// are wired with db/logger middleware and auth middleware for userID.
func setupTestWithUser(t *testing.T, userID int64) (*sql.DB, *gin.Engine, *zap.SugaredLogger) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := tempDB(t)
	runMigrate(t, db)
	seedUser(t, db, userID)

	logger := zap.NewNop().Sugar()

	router := gin.New()
	router.Use(testMiddleware(db, logger))

	// Protected routes (auth required)
	v1 := router.Group("/api/v1")
	v1.Use(testAuthMiddleware(userID))

	loadoutGroup := v1.Group("/loadout")
	loadoutGroup.PUT("/insert", InsertLoadout)
	loadoutGroup.GET("/list", ListLoadouts)
	loadoutGroup.GET("/:loadout/get", GetLoadout)
	loadoutGroup.POST("/:loadout/update", UpdateLoadout)
	loadoutGroup.DELETE("/:loadout/delete", DeleteLoadout)
	loadoutGroup.POST("/:loadout/import", ImportLoadout)
	loadoutGroup.PUT("/:loadout/item/insert", InsertLoadoutItem)
	loadoutGroup.GET("/:loadout/item/list", ListLoadoutItems)
	loadoutGroup.POST("/:loadout/item/:item/update", UpdateLoadoutItem)
	loadoutGroup.DELETE("/:loadout/item/:item/delete", DeleteLoadoutItem)

	// Public routes (no auth)
	pub := router.Group("/api/v1/public")
	pub.GET("/loadout/:slug", GetPublicLoadout)
	pub.GET("/loadout/:slug/items", GetPublicLoadoutItems)

	return db, router, logger
}

func setupTest(t *testing.T) (*sql.DB, *gin.Engine, *zap.SugaredLogger) {
	return setupTestWithUser(t, 1)
}

// routerWithUser creates a Gin engine wired with all loadout routes using the
// given db and logger, authenticating as userID. This lets tests reuse a single
// DB while changing the authenticated user.
func routerWithUser(db *sql.DB, logger *zap.SugaredLogger, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testMiddleware(db, logger))

	v1 := router.Group("/api/v1")
	v1.Use(testAuthMiddleware(userID))

	loadoutGroup := v1.Group("/loadout")
	loadoutGroup.PUT("/insert", InsertLoadout)
	loadoutGroup.GET("/list", ListLoadouts)
	loadoutGroup.GET("/:loadout/get", GetLoadout)
	loadoutGroup.POST("/:loadout/update", UpdateLoadout)
	loadoutGroup.DELETE("/:loadout/delete", DeleteLoadout)
	loadoutGroup.POST("/:loadout/import", ImportLoadout)
	loadoutGroup.PUT("/:loadout/item/insert", InsertLoadoutItem)
	loadoutGroup.GET("/:loadout/item/list", ListLoadoutItems)
	loadoutGroup.POST("/:loadout/item/:item/update", UpdateLoadoutItem)
	loadoutGroup.DELETE("/:loadout/item/:item/delete", DeleteLoadoutItem)

	pub := router.Group("/api/v1/public")
	pub.GET("/loadout/:slug", GetPublicLoadout)
	pub.GET("/loadout/:slug/items", GetPublicLoadoutItems)

	return router
}

// ---------------------------------------------------------------------------
// Request helper
// ---------------------------------------------------------------------------

func authRequest(t *testing.T, router *gin.Engine, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Loadout CRUD Tests
// ---------------------------------------------------------------------------

func TestInsertLoadout(t *testing.T) {
	_, router, _ := setupTest(t)

	body := `{"loadout_name":"My Weekend Pack","loadout_description":"A light pack","loadout_is_public":false,"loadout_slug":"weekend-pack"}`
	w := authRequest(t, router, http.MethodPut, "/api/v1/loadout/insert", body)

	if w.Code != http.StatusCreated {
		t.Errorf("InsertLoadout: expected 201, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("InsertLoadout: unmarshal error: %v", err)
	}
	// Check that returned body has the fields we sent
	if name, ok := resp["loadout_name"]; !ok || name != "My Weekend Pack" {
		t.Errorf("InsertLoadout: expected loadout_name='My Weekend Pack', got %v", name)
	}
}

func TestInsertLoadout_InvalidBody(t *testing.T) {
	_, router, _ := setupTest(t)

	w := authRequest(t, router, http.MethodPut, "/api/v1/loadout/insert", `{bad json`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InsertLoadout_InvalidBody: expected 400, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestListLoadouts(t *testing.T) {
	db, router, _ := setupTest(t)

	// Seed two loadouts for user 1
	seedLoadout(t, db, 1, false, "pack-a")
	seedLoadout(t, db, 1, true, "pack-b")

	w := authRequest(t, router, http.MethodGet, "/api/v1/loadout/list", "")
	if w.Code != http.StatusOK {
		t.Fatalf("ListLoadouts: expected 200, got %d", w.Code)
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("ListLoadouts: unmarshal error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListLoadouts: expected 2 loadouts, got %d", len(list))
	}
}

func TestListLoadouts_Empty(t *testing.T) {
	_, router, _ := setupTest(t)

	w := authRequest(t, router, http.MethodGet, "/api/v1/loadout/list", "")
	if w.Code != http.StatusOK {
		t.Fatalf("ListLoadouts_Empty: expected 200, got %d", w.Code)
	}

	var list []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("ListLoadouts_Empty: unmarshal error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListLoadouts_Empty: expected empty array, got %d items", len(list))
	}
}

func TestGetLoadout(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "my-pack")
	w := authRequest(t, router, http.MethodGet, "/api/v1/loadout/"+itoa64(loadoutID)+"/get", "")

	if w.Code != http.StatusOK {
		t.Fatalf("GetLoadout: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("GetLoadout: unmarshal error: %v", err)
	}
	// loadout_name should match what seedLoadout inserted
	if name, ok := resp["loadout_name"]; !ok || name != "test" {
		t.Errorf("GetLoadout: expected loadout_name='test', got %v", name)
	}
}

func TestGetLoadout_NotFound(t *testing.T) {
	_, router, _ := setupTest(t)

	w := authRequest(t, router, http.MethodGet, "/api/v1/loadout/99999/get", "")

	if w.Code != http.StatusNotFound {
		t.Errorf("GetLoadout_NotFound: expected 404, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestGetLoadout_OtherUser(t *testing.T) {
	db, _, logger := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "private-pack")

	router2 := routerWithUser(db, logger, 2)

	w := authRequest(t, router2, http.MethodGet, "/api/v1/loadout/"+itoa64(loadoutID)+"/get", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("GetLoadout_OtherUser: expected 404, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateLoadout(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "update-me")
	body := `{"loadout_id":` + itoa64(loadoutID) + `,"loadout_name":"Updated Name","loadout_description":"Updated desc","loadout_is_public":true,"loadout_slug":"updated-slug"}`

	w := authRequest(t, router, http.MethodPost, "/api/v1/loadout/"+itoa64(loadoutID)+"/update", body)

	if w.Code != http.StatusOK {
		t.Errorf("TestUpdateLoadout: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestUpdateLoadout: unmarshal error: %v", err)
	}
	if resp["status"] != "success" {
		t.Errorf("TestUpdateLoadout: expected status=success, got %v", resp["status"])
	}
}

func TestUpdateLoadout_OtherUser(t *testing.T) {
	db, _, logger := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "not-yours")

	router2 := routerWithUser(db, logger, 2)

	body := `{"loadout_id":` + itoa64(loadoutID) + `,"loadout_name":"Hacked","loadout_description":"","loadout_is_public":false,"loadout_slug":"hacked"}`
	w := authRequest(t, router2, http.MethodPost, "/api/v1/loadout/"+itoa64(loadoutID)+"/update", body)
	if w.Code != http.StatusForbidden {
		t.Errorf("TestUpdateLoadout_OtherUser: expected 403, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteLoadout(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "delete-me")
	w := authRequest(t, router, http.MethodDelete, "/api/v1/loadout/"+itoa64(loadoutID)+"/delete", "")

	if w.Code != http.StatusOK {
		t.Errorf("TestDeleteLoadout: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestDeleteLoadout: unmarshal error: %v", err)
	}
	if resp["status"] != "success" {
		t.Errorf("TestDeleteLoadout: expected status=success, got %v", resp["status"])
	}
}

func TestDeleteLoadout_OtherUser(t *testing.T) {
	db, _, logger := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "not-yours-del")

	router2 := routerWithUser(db, logger, 2)

	w := authRequest(t, router2, http.MethodDelete, "/api/v1/loadout/"+itoa64(loadoutID)+"/delete", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("TestDeleteLoadout_OtherUser: expected 403, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Import Tests
// ---------------------------------------------------------------------------

func TestImportLoadout(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "import-me")
	seedGear(t, db, 1)
	seedGear(t, db, 2)

	body := `{"gear_ids":[1,2]}`
	w := authRequest(t, router, http.MethodPost, "/api/v1/loadout/"+itoa64(loadoutID)+"/import", body)

	if w.Code != http.StatusCreated {
		t.Errorf("TestImportLoadout: expected 201, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestImportLoadout: unmarshal error: %v", err)
	}
	if resp["status"] != "success" {
		t.Errorf("TestImportLoadout: expected status=success, got %v", resp["status"])
	}
}

func TestImportLoadout_OtherUser(t *testing.T) {
	db, _, logger := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "not-yours-import")
	seedGear(t, db, 1)

	router2 := routerWithUser(db, logger, 2)

	body := `{"gear_ids":[1]}`
	w := authRequest(t, router2, http.MethodPost, "/api/v1/loadout/"+itoa64(loadoutID)+"/import", body)
	if w.Code != http.StatusForbidden {
		t.Errorf("TestImportLoadout_OtherUser: expected 403, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Loadout Items Tests
// ---------------------------------------------------------------------------

func TestInsertLoadoutItem(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "item-test")
	seedGear(t, db, 1)

	body := `{"gear_id":1,"quantity":2,"notes":"test note"}`
	w := authRequest(t, router, http.MethodPut, "/api/v1/loadout/"+itoa64(loadoutID)+"/item/insert", body)

	if w.Code != http.StatusCreated {
		t.Errorf("TestInsertLoadoutItem: expected 201, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestInsertLoadoutItem: unmarshal error: %v", err)
	}
	if gid, ok := resp["gear_id"]; !ok || gid.(float64) != 1 {
		t.Errorf("TestInsertLoadoutItem: expected gear_id=1, got %v", gid)
	}
}

func TestListLoadoutItems(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "items-list")
	seedGear(t, db, 1)
	seedGear(t, db, 2)
	seedLoadoutItem(t, db, loadoutID, 1)
	seedLoadoutItem(t, db, loadoutID, 2)

	w := authRequest(t, router, http.MethodGet, "/api/v1/loadout/"+itoa64(loadoutID)+"/item/list", "")
	if w.Code != http.StatusOK {
		t.Fatalf("TestListLoadoutItems: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("TestListLoadoutItems: unmarshal error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("TestListLoadoutItems: expected 2 items, got %d", len(list))
	}
}

func TestUpdateLoadoutItem(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "item-update")
	seedGear(t, db, 1)
	seedLoadoutItem(t, db, loadoutID, 1)

	// Get the inserted item ID
	var itemID int64
	err := db.QueryRow("SELECT loadoutItemId FROM loadout_items WHERE loadoutId = ?", loadoutID).Scan(&itemID)
	if err != nil {
		t.Fatalf("select item ID: %v", err)
	}

	body := `{"quantity":5,"notes":"updated notes"}`
	w := authRequest(t, router, http.MethodPost, "/api/v1/loadout/"+itoa64(loadoutID)+"/item/"+itoa64(itemID)+"/update", body)
	if w.Code != http.StatusOK {
		t.Errorf("TestUpdateLoadoutItem: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestUpdateLoadoutItem: unmarshal error: %v", err)
	}
	if resp["status"] != "success" {
		t.Errorf("TestUpdateLoadoutItem: expected status=success, got %v", resp["status"])
	}
}

func TestDeleteLoadoutItem(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "item-delete")
	seedGear(t, db, 1)
	seedLoadoutItem(t, db, loadoutID, 1)

	// Get the inserted item ID
	var itemID int64
	err := db.QueryRow("SELECT loadoutItemId FROM loadout_items WHERE loadoutId = ?", loadoutID).Scan(&itemID)
	if err != nil {
		t.Fatalf("select item ID: %v", err)
	}

	w := authRequest(t, router, http.MethodDelete, "/api/v1/loadout/"+itoa64(loadoutID)+"/item/"+itoa64(itemID)+"/delete", "")
	if w.Code != http.StatusOK {
		t.Errorf("TestDeleteLoadoutItem: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestDeleteLoadoutItem: unmarshal error: %v", err)
	}
	if resp["status"] != "success" {
		t.Errorf("TestDeleteLoadoutItem: expected status=success, got %v", resp["status"])
	}
}

func TestInsertLoadoutItem_OtherUser(t *testing.T) {
	db, _, logger := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "other-item-insert")
	seedGear(t, db, 1)

	router2 := routerWithUser(db, logger, 2)

	body := `{"gear_id":1,"quantity":1,"notes":""}`
	w := authRequest(t, router2, http.MethodPut, "/api/v1/loadout/"+itoa64(loadoutID)+"/item/insert", body)
	if w.Code != http.StatusForbidden {
		t.Errorf("TestInsertLoadoutItem_OtherUser: expected 403, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestListLoadoutItems_OtherUser(t *testing.T) {
	db, _, logger := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, false, "other-items-list")
	seedGear(t, db, 1)
	seedLoadoutItem(t, db, loadoutID, 1)

	router2 := routerWithUser(db, logger, 2)

	w := authRequest(t, router2, http.MethodGet, "/api/v1/loadout/"+itoa64(loadoutID)+"/item/list", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("TestListLoadoutItems_OtherUser: expected 404, got %d — body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Public Routes
// ---------------------------------------------------------------------------

func TestGetPublicLoadout(t *testing.T) {
	db, router, _ := setupTest(t)

	_ = seedLoadout(t, db, 1, true, "public-slug")

	w := authRequest(t, router, http.MethodGet, "/api/v1/public/loadout/public-slug", "")
	if w.Code != http.StatusOK {
		t.Fatalf("TestGetPublicLoadout: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("TestGetPublicLoadout: unmarshal error: %v", err)
	}
	// Must NOT contain user_id
	if _, ok := resp["user_id"]; ok {
		t.Errorf("TestGetPublicLoadout: response must not include user_id")
	}
	// Must contain loadout_name
	if name, ok := resp["loadout_name"]; !ok || name != "test" {
		t.Errorf("TestGetPublicLoadout: expected loadout_name='test', got %v", name)
	}
}

func TestGetPublicLoadout_NotFound(t *testing.T) {
	_, router, _ := setupTest(t)

	w := authRequest(t, router, http.MethodGet, "/api/v1/public/loadout/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("TestGetPublicLoadout_NotFound: expected 404, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestGetPublicLoadout_NotPublic(t *testing.T) {
	db, router, _ := setupTest(t)

	_ = seedLoadout(t, db, 1, false, "private-slug")

	w := authRequest(t, router, http.MethodGet, "/api/v1/public/loadout/private-slug", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("TestGetPublicLoadout_NotPublic: expected 404, got %d — body: %s", w.Code, w.Body.String())
	}
}

func TestGetPublicLoadoutItems(t *testing.T) {
	db, router, _ := setupTest(t)

	loadoutID := seedLoadout(t, db, 1, true, "public-items-slug")
	seedGear(t, db, 1)
	seedGear(t, db, 2)
	seedLoadoutItem(t, db, loadoutID, 1)
	seedLoadoutItem(t, db, loadoutID, 2)

	w := authRequest(t, router, http.MethodGet, "/api/v1/public/loadout/public-items-slug/items", "")
	if w.Code != http.StatusOK {
		t.Fatalf("TestGetPublicLoadoutItems: expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("TestGetPublicLoadoutItems: unmarshal error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("TestGetPublicLoadoutItems: expected 2 items, got %d", len(list))
	}
}
