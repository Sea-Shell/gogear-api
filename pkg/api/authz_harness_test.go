package endpoints

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	utils "github.com/Sea-Shell/gogear-api/pkg/utils"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Authorization test harness.
//
// Requests go through the real utils.JWTMiddleware with HS256 tokens signed
// like the service signs them (client audience for ordinary users, admin
// audience for admins), against a migrated SQLite database. Handlers decide
// from the token, so tests cannot hand-inject identity.
//
// Later waves extend it by registering more routes on env.V1 (or with
// newAuthzEnvWithAuth for non-default token configuration).

const (
	harnessSecret        = "harness-secret"
	harnessIssuer        = "gogear-test"
	harnessClientAud     = "gogear-client"
	harnessAdminAudience = "gogear-admin"
)

func harnessAuth() *models.Auth {
	return &models.Auth{
		JWTSecret:        harnessSecret,
		JWTIssuer:        harnessIssuer,
		JWTAudience:      harnessClientAud,
		JWTAdminAudience: harnessAdminAudience,
		JWTExpiryMinutes: 60,
	}
}

type authzEnv struct {
	T      *testing.T
	DB     *sql.DB
	Router *gin.Engine
	V1     *gin.RouterGroup // /api/v1, behind the real JWT middleware
	Auth   *models.Auth
}

func newAuthzEnv(t *testing.T) *authzEnv {
	return newAuthzEnvWithAuth(t, harnessAuth())
}

func newAuthzEnvWithAuth(t *testing.T, auth *models.Auth) *authzEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// A file database: unlike :memory:, every pooled connection sees the same
	// data, which transactional handlers need.
	db, err := sql.Open("sqlite3", utils.SQLiteDSN(filepath.Join(t.TempDir(), "authz.db")))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	runMigrate(t, db)

	logger := zap.NewNop().Sugar()
	router := gin.New()
	router.Use(gin.Recovery()) // a handler panic is a 500, as in production
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("logger", logger)
		c.Set("auth", auth)
		c.Next()
	})

	v1 := router.Group("/api/v1")
	v1.Use(utils.JWTMiddleware())

	RegisterUserRoutes(v1.Group("/users"))
	RegisterCatalogRoutes(v1)
	router.POST("/auth/refresh", utils.JWTMiddleware(), RefreshToken)
	RegisterLoadoutRoutes(v1.Group("/loadout"))
	RegisterPublicLoadoutRoutes(router.Group("/api/v1/public"))
	RegisterUserGearRoutes(v1.Group("/usergear"))
	RegisterContainerRoutes(v1.Group("/container"))

	return &authzEnv{T: t, DB: db, Router: router, V1: v1, Auth: auth}
}

// SignToken signs an HS256 service token with explicit claims.
func (e *authzEnv) SignToken(subject, audience string, expires time.Time) string {
	e.T.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		Issuer:    e.Auth.JWTIssuer,
		ExpiresAt: jwt.NewNumericDate(expires),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	if audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(e.Auth.JWTSecret))
	if err != nil {
		e.T.Fatalf("sign token: %v", err)
	}
	return s
}

// Token issues a token for userID with the client or admin audience. It does
// not look at the database, so a test can mint a stale admin token.
func (e *authzEnv) Token(userID int64, admin bool) string {
	aud := e.Auth.JWTAudience
	if admin {
		aud = e.Auth.JWTAdminAudience
	}
	return e.SignToken(fmt.Sprint(userID), aud, time.Now().Add(time.Hour))
}

// Do sends a request. token may be empty (no Authorization header). body may
// be nil, a string/[]byte (sent raw), or any value (sent as JSON).
func (e *authzEnv) Do(method, path, token string, body any) *httptest.ResponseRecorder {
	e.T.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case nil:
	case string:
		buf.WriteString(b)
	case []byte:
		buf.Write(b)
	default:
		if err := json.NewEncoder(&buf).Encode(b); err != nil {
			e.T.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.Router.ServeHTTP(w, req)
	return w
}

// SeedUser inserts a user with a distinct email and a recognisable password
// value, and returns the new ID.
func (e *authzEnv) SeedUser(name, email string, admin bool) int64 {
	e.T.Helper()
	a := 0
	if admin {
		a = 1
	}
	res, err := e.DB.Exec(
		`INSERT INTO users (userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal) VALUES (?, ?, ?, ?, ?, 0)`,
		name, "hash-of-"+name, name+" Full", email, a)
	if err != nil {
		e.T.Fatalf("seed user %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// SeedRegistration inserts a gear registration owned by userID.
func (e *authzEnv) SeedRegistration(userID, gearID int64) int64 {
	e.T.Helper()
	res, err := e.DB.Exec(`INSERT INTO user_gear_registrations (gearId, userId) VALUES (?, ?)`, gearID, userID)
	if err != nil {
		e.T.Fatalf("seed registration: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// SeedContainerLink links content registration into container registration.
func (e *authzEnv) SeedContainerLink(container, content int64) {
	e.T.Helper()
	if _, err := e.DB.Exec(`INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (?, ?)`, container, content); err != nil {
		e.T.Fatalf("seed container link: %v", err)
	}
}

// SeedCategory inserts a catalog category (an admin-only record) and returns its ID.
func (e *authzEnv) SeedCategory(name string) int64 {
	e.T.Helper()
	res, err := e.DB.Exec(`INSERT INTO gear_category (categoryTopCategoryId, categoryName) VALUES (1, ?)`, name)
	if err != nil {
		e.T.Fatalf("seed category: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// Count runs SELECT COUNT(*) FROM table [WHERE where].
func (e *authzEnv) Count(table, where string, args ...any) int {
	e.T.Helper()
	q := "SELECT COUNT(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	var n int
	if err := e.DB.QueryRow(q, args...).Scan(&n); err != nil {
		e.T.Fatalf("count %s: %v", table, err)
	}
	return n
}

// userRow is a full stored snapshot of one user, including the password column.
type userRow struct {
	Exists                          bool
	Username, Password, Name, Email string
	IsAdmin                         bool
}

func (e *authzEnv) User(id int64) userRow {
	e.T.Helper()
	var r userRow
	var admin int
	err := e.DB.QueryRow(`SELECT userUsername, userPassword, COALESCE(userName,''), userEmail, userIsAdmin FROM users WHERE userId = ?`, id).
		Scan(&r.Username, &r.Password, &r.Name, &r.Email, &admin)
	if err == sql.ErrNoRows {
		return userRow{}
	}
	if err != nil {
		e.T.Fatalf("read user %d: %v", id, err)
	}
	r.Exists, r.IsAdmin = true, admin != 0
	return r
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, want, w.Body.String())
	}
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("response is not a single JSON document: %v; body: %s", err, w.Body.String())
	}
}

// smoke test for the harness itself: tokens are verified by the real middleware.
func TestHarnessUsesRealJWTMiddleware(t *testing.T) {
	env := newAuthzEnv(t)
	id := env.SeedUser("alice", "alice@example.com", false)

	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list", "", nil), http.StatusUnauthorized)
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list", "garbage", nil), http.StatusUnauthorized)

	bad := env.SignToken(fmt.Sprint(id), "some-other-audience", time.Now().Add(time.Hour))
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list", bad, nil), http.StatusUnauthorized)

	expired := env.SignToken(fmt.Sprint(id), harnessClientAud, time.Now().Add(-time.Hour))
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list", expired, nil), http.StatusUnauthorized)

	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list", env.Token(id, false), nil), http.StatusOK)
}
