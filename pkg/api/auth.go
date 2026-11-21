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
)

type googleCallbackRequest struct {
	IDToken    string `json:"id_token"`
	Credential string `json:"credential"`
	Code       string `json:"code"`
}

// RefreshToken reissues a GoGear JWT using the current authenticated session.
//
//	@Summary		Refresh service token
//	@Description	Exchanges a valid GoGear JWT for a new token with a fresh expiry
//	@Tags			Auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	models.Error
//	@Failure		500	{object}	models.Error
//	@Router			/auth/refresh [post]
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

	// The role comes from the stored account only, never from the presented
	// token (BR-006): a demoted admin refreshes into a non-admin token.

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
//	@Summary		Google OAuth callback
//	@Description	Validates the Google credential and returns a JWT for subsequent API calls
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request		body		googleCallbackRequest	false	"Callback payload"
//	@Param			code		query		string					false	"Authorization code"
//	@Param			id_token	query		string					false	"Google ID token"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		400			{object}	models.Error
//	@Failure		401			{object}	models.Error
//	@Failure		500			{object}	models.Error
//	@Router			/auth/google/callback [post]
//	@Router			/auth/google/callback [get]
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

	validate := googleValidatorFrom(c)
	identity, err := validate(ctx, idToken, authConfig.GoogleClientID)
	if err != nil {
		logger.Warnw("Google token validation failed", "error", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid Google token"})
		return
	}

	// BR-003 AC-3: without a subject the account cannot be bound, and an
	// email fallback would be an email-only link.
	identity.Subject = strings.TrimSpace(identity.Subject)
	if identity.Subject == "" {
		logger.Warn("Google token lacks a subject claim")
		c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid Google token"})
		return
	}

	db := c.MustGet("db").(*sql.DB)
	user, err := signInWithGoogle(ctx, db, identity)
	switch {
	case errors.Is(err, errGoogleEmailMissing):
		logger.Warn("Google token lacks an email claim")
		c.AbortWithStatusJSON(http.StatusBadRequest, models.Error{Error: "Google token missing email"})
		return
	case errors.Is(err, errGoogleRefused):
		logger.Warnw("Google sign-in refused", "reason", err.Error())
		c.AbortWithStatusJSON(http.StatusForbidden, models.Error{Error: "Google sign-in not permitted for this account"})
		return
	case err != nil:
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

// resolveUserForSubject finds the account a service token's subject names.
// The subject is the numeric user ID; there is deliberately no email fallback
// (BR-006 AC-3).
func resolveUserForSubject(ctx context.Context, db *sql.DB, subject string) (*models.User, error) {
	id, err := strconv.ParseInt(subject, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("token subject %q is not a user ID", subject)
	}
	return findUserByID(ctx, db, id)
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

	if user == nil || user.UserID == nil {
		return "", time.Time{}, errors.New("user ID required for JWT subject")
	}
	subject := strconv.FormatInt(*user.UserID, 10)

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
