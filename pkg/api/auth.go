package endpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	zap "go.uber.org/zap"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

type googleCallbackRequest struct {
	IDToken    string `json:"id_token"`
	Credential string `json:"credential"`
	Code       string `json:"code"`
}

// RefreshToken reissues a GoGear JWT using the current authenticated session.
//
//	@Summary  Refresh service token
//	@Description  Exchanges a valid GoGear JWT for a new token with a fresh expiry
//	@Tags     Auth
//	@Produce  json
//	@Security  BearerAuth
//	@Success  200  {object}  map[string]interface{}
//	@Failure  401  {object}  models.Error
//	@Failure  500  {object}  models.Error
//	@Router   /auth/refresh [post]
func RefreshToken(c *gin.Context) {
	logger := c.MustGet("logger").(*zap.SugaredLogger)

	authAny, ok := c.Get("auth")
	if !ok {
		logger.Error("authentication configuration missing from context")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "authentication configuration not available"})
		return
	}

	authConfig, ok := authAny.(*models.Auth)
	if !ok {
		logger.Error("authentication configuration has unexpected type")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "authentication configuration invalid"})
		return
	}

	subjectAny, ok := c.Get("user_id")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "subject claim missing"})
		return
	}

	subject, _ := subjectAny.(string)
	subject = strings.TrimSpace(subject)
	if subject == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "subject claim missing"})
		return
	}

	ctx := c.Request.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	db := c.MustGet("db").(*sql.DB)
	user, err := resolveUserForSubject(ctx, db, subject)
	if err != nil {
		logger.Warnw("failed to resolve user for refresh", "error", err, "subject", subject)
		c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "user not found"})
		return
	}

	if adminAny, ok := c.Get("user_is_admin"); ok {
		if isAdmin, ok := adminAny.(bool); ok && isAdmin {
			user.UserIsAdmin = true
		}
	}

	token, expiresAt, err := issueServiceToken(authConfig, user)
	if err != nil {
		logger.Errorw("failed to issue refreshed token", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "failed to issue access token"})
		return
	}

	expiresIn := int64(time.Until(expiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}

	response := gin.H{
		"token_type":   "Bearer",
		"access_token": token,
		"expires_at":   expiresAt.Unix(),
		"expires_in":   expiresIn,
	}

	if user != nil {
		response["user"] = gin.H{
			"id":       user.UserID,
			"email":    user.UserEmail,
			"name":     user.UserName,
			"is_admin": user.UserIsAdmin,
		}
	}

	c.JSON(http.StatusOK, response)
}

// GoogleAuthCallback handles Google OAuth callbacks and issues a JWT for the API.
//
//	@Summary  Google OAuth callback
//	@Description  Validates the Google credential and returns a JWT for subsequent API calls
//	@Tags     Auth
//	@Accept   json
//	@Produce  json
//	@Param    request  body      googleCallbackRequest  false  "Callback payload"
//	@Param    code     query     string                 false  "Authorization code"
//	@Param    id_token query     string                 false  "Google ID token"
//	@Success  200      {object}  map[string]interface{}
//	@Failure  400      {object}  models.Error
//	@Failure  401      {object}  models.Error
//	@Failure  500      {object}  models.Error
//	@Router   /auth/google/callback [post]
//	@Router   /auth/google/callback [get]
func GoogleAuthCallback(c *gin.Context) {
	logger := c.MustGet("logger").(*zap.SugaredLogger)

	authConfigAny, ok := c.Get("auth")
	if !ok {
		logger.Error("authentication configuration missing from context")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "authentication configuration not available"})
		return
	}

	authConfig, ok := authConfigAny.(*models.Auth)
	if !ok {
		logger.Error("authentication configuration has unexpected type")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "authentication configuration invalid"})
		return
	}
	if strings.TrimSpace(authConfig.GoogleClientID) == "" {
		logger.Error("Google client ID is not configured")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "Google authentication not configured"})
		return
	}

	oauthAny, ok := c.Get("oauth")
	if !ok {
		logger.Error("OAuth configuration missing from context")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "OAuth configuration not available"})
		return
	}

	oauthConfig, ok := oauthAny.(*oauth2.Config)
	if !ok {
		logger.Error("OAuth configuration has unexpected type")
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "OAuth configuration invalid"})
		return
	}

	var body googleCallbackRequest
	if c.Request.Method == http.MethodPost {
		contentType := strings.ToLower(c.GetHeader("Content-Type"))
		switch {
		case strings.Contains(contentType, "application/json"):
			if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
				logger.Warnw("unable to parse JSON body", "error", err)
				c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "invalid request body"})
				return
			}
		default:
			body.IDToken = c.PostForm("id_token")
			if body.IDToken == "" {
				body.IDToken = c.PostForm("credential")
			}
			body.Code = c.PostForm("code")
		}
	}

	code := strings.TrimSpace(body.Code)
	if code == "" {
		code = strings.TrimSpace(c.Query("code"))
	}

	idToken := strings.TrimSpace(body.IDToken)
	if idToken == "" {
		idToken = strings.TrimSpace(body.Credential)
	}
	if idToken == "" {
		idToken = strings.TrimSpace(c.Query("id_token"))
	}
	if idToken == "" {
		idToken = strings.TrimSpace(c.Query("credential"))
	}

	ctx := c.Request.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if idToken == "" {
		if code == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "missing Google credentials"})
			return
		}

		token, err := oauthConfig.Exchange(ctx, code)
		if err != nil {
			logger.Warnw("failed to exchange authorization code", "error", err)
			c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "unable to exchange authorization code"})
			return
		}

		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok || strings.TrimSpace(rawIDToken) == "" {
			logger.Error("Google response did not contain id_token")
			c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "missing id_token in Google response"})
			return
		}
		idToken = rawIDToken
	}

	payload, err := idtoken.Validate(ctx, idToken, authConfig.GoogleClientID)
	if err != nil {
		logger.Warnw("Google token validation failed", "error", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid Google token"})
		return
	}

	email := ""
	if payload.Claims != nil {
		if claimEmail, ok := payload.Claims["email"].(string); ok {
			email = strings.TrimSpace(claimEmail)
		}
	}

	if email == "" {
		logger.Warn("Google token lacks an email claim")
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "Google token missing email"})
		return
	}

	fullName := ""
	if payload.Claims != nil {
		if v, ok := payload.Claims["name"].(string); ok {
			fullName = strings.TrimSpace(v)
		}
		if fullName == "" {
			given, _ := payload.Claims["given_name"].(string)
			family, _ := payload.Claims["family_name"].(string)
			fullName = strings.TrimSpace(strings.TrimSpace(given + " " + family))
		}
	}
	if fullName == "" {
		fullName = email
	}

	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		subject = email
	}

	db := c.MustGet("db").(*sql.DB)
	user, err := ensureUserFromGoogle(ctx, db, email, fullName, subject)
	if err != nil {
		logger.Errorw("failed to persist Google user", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "failed to persist user"})
		return
	}

	token, expiresAt, err := issueServiceToken(authConfig, user)
	if err != nil {
		logger.Errorw("failed to issue API token", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "failed to issue access token"})
		return
	}

	expiresIn := int64(time.Until(expiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}

	response := gin.H{
		"token_type":   "Bearer",
		"access_token": token,
		"expires_at":   expiresAt.Unix(),
		"expires_in":   expiresIn,
		"user": gin.H{
			"id":       user.UserID,
			"email":    user.UserEmail,
			"name":     user.UserName,
			"is_admin": user.UserIsAdmin,
		},
	}

	if state := strings.TrimSpace(c.Query("state")); state != "" {
		response["state"] = state
	}

	c.JSON(http.StatusOK, response)
}

func ensureUserFromGoogle(ctx context.Context, db *sql.DB, email, fullName, subject string) (*models.User, error) {
	user, err := findUserByEmail(ctx, db, email)
	if err == nil {
		return user, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	username := email
	if username == "" {
		username = fmt.Sprintf("google_%s", subject)
	}

	result, err := db.ExecContext(ctx, `INSERT INTO users (userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal) VALUES (?, ?, ?, ?, ?, ?)`,
		username, "", fullName, email, 0, 1,
	)
	if err != nil {
		existing, lookupErr := findUserByEmail(ctx, db, email)
		if lookupErr == nil {
			return existing, nil
		}
		return nil, err
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.User{
		UserID:       &userID,
		UserUsername: username,
		UserName:     fullName,
		UserEmail:    email,
		UserIsAdmin:  false,
	}, nil
}

func findUserByEmail(ctx context.Context, db *sql.DB, email string) (*models.User, error) {
	row := db.QueryRowContext(ctx, `SELECT userId, userPassword, userUsername, userName, userEmail, userIsAdmin FROM users WHERE userEmail = ? LIMIT 1`, email)

	var (
		userID   sql.NullInt64
		password sql.NullString
		username sql.NullString
		name     sql.NullString
		mail     sql.NullString
		admin    sql.NullInt64
	)

	if err := row.Scan(&userID, &password, &username, &name, &mail, &admin); err != nil {
		return nil, err
	}

	user := &models.User{
		UserPassword: password.String,
		UserUsername: username.String,
		UserName:     name.String,
		UserEmail:    mail.String,
	}

	if userID.Valid {
		user.UserID = &userID.Int64
	}

	if admin.Valid && admin.Int64 != 0 {
		user.UserIsAdmin = true
	}

	return user, nil
}

func findUserByID(ctx context.Context, db *sql.DB, id int64) (*models.User, error) {
	row := db.QueryRowContext(ctx, `SELECT userId, userPassword, userUsername, userName, userEmail, userIsAdmin FROM users WHERE userId = ? LIMIT 1`, id)

	var (
		userID   sql.NullInt64
		password sql.NullString
		username sql.NullString
		name     sql.NullString
		mail     sql.NullString
		admin    sql.NullInt64
	)

	if err := row.Scan(&userID, &password, &username, &name, &mail, &admin); err != nil {
		return nil, err
	}

	user := &models.User{
		UserPassword: password.String,
		UserUsername: username.String,
		UserName:     name.String,
		UserEmail:    mail.String,
	}

	if userID.Valid {
		id := userID.Int64
		user.UserID = &id
	}

	if admin.Valid && admin.Int64 != 0 {
		user.UserIsAdmin = true
	}

	return user, nil
}

func resolveUserForSubject(ctx context.Context, db *sql.DB, subject string) (*models.User, error) {
	if id, err := strconv.ParseInt(subject, 10, 64); err == nil {
		if user, err := findUserByID(ctx, db, id); err == nil {
			return user, nil
		}
	}

	return findUserByEmail(ctx, db, subject)
}

func issueServiceToken(authConfig *models.Auth, user *models.User) (string, time.Time, error) {
	secret := strings.TrimSpace(authConfig.JWTSecret)
	if secret == "" {
		return "", time.Time{}, errors.New("JWT secret not configured")
	}

	expiryMinutes := authConfig.JWTExpiryMinutes
	if expiryMinutes <= 0 {
		expiryMinutes = 60
	}

	expiresAt := time.Now().Add(time.Duration(expiryMinutes) * time.Minute)

	subject := ""
	if user.UserID != nil {
		subject = strconv.FormatInt(*user.UserID, 10)
	} else if user.UserEmail != "" {
		subject = user.UserEmail
	}

	claims := jwt.RegisteredClaims{
		Subject:   subject,
		Issuer:    strings.TrimSpace(authConfig.JWTIssuer),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	audience := strings.TrimSpace(authConfig.JWTAudience)
	if user != nil && user.UserIsAdmin {
		if adminAudience := strings.TrimSpace(authConfig.JWTAdminAudience); adminAudience != "" {
			audience = adminAudience
		}
	}

	if audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}
