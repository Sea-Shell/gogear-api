package utils

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	zap "go.uber.org/zap"
)

// JWTMiddleware validates Bearer tokens issued by this service and attaches the claims to the request context.
func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		loggerAny, ok := c.Get("logger")
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "request logger missing from context"})
			return
		}

		logger, ok := loggerAny.(*zap.SugaredLogger)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "invalid logger type in context"})
			return
		}

		authAny, ok := c.Get("auth")
		if !ok {
			logger.Error("authentication config missing from context")
			c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "authentication config unavailable"})
			return
		}

		authConfig, ok := authAny.(*models.Auth)
		if !ok {
			logger.Error("authentication config has unexpected type")
			c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "authentication config invalid"})
			return
		}

		if strings.TrimSpace(authConfig.JWTSecret) == "" {
			logger.Error("JWT secret is not configured")
			c.AbortWithStatusJSON(http.StatusInternalServerError, models.Error{Error: "JWT secret not configured"})
			return
		}

		authorization := c.GetHeader("Authorization")
		if authorization == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "missing Authorization header"})
			return
		}

		parts := strings.SplitN(authorization, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid Authorization header format"})
			return
		}

		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "empty bearer token"})
			return
		}

		claims := &jwt.RegisteredClaims{}
		parserOptions := []jwt.ParserOption{jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})}

		if issuer := strings.TrimSpace(authConfig.JWTIssuer); issuer != "" {
			parserOptions = append(parserOptions, jwt.WithIssuer(issuer))
		}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(authConfig.JWTSecret), nil
		}, parserOptions...)
		if err != nil {
			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "token has expired"})
			case errors.Is(err, jwt.ErrTokenNotValidYet):
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "token not valid yet"})
			case errors.Is(err, jwt.ErrTokenInvalidAudience):
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid token audience"})
			case errors.Is(err, jwt.ErrTokenInvalidIssuer):
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid token issuer"})
			default:
				logger.Warnw("failed to parse JWT", "error", err)
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid token"})
			}
			return
		}

		if !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "token validation failed"})
			return
		}

		allowedAudiences := make([]string, 0, 2)
		if aud := strings.TrimSpace(authConfig.JWTAudience); aud != "" {
			allowedAudiences = append(allowedAudiences, aud)
		}
		if adminAud := strings.TrimSpace(authConfig.JWTAdminAudience); adminAud != "" {
			allowedAudiences = append(allowedAudiences, adminAud)
		}

		userIsAdmin := false
		if len(allowedAudiences) > 0 {
			if len(claims.Audience) == 0 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid token audience"})
				return
			}

			matched := false
			for _, tokenAud := range claims.Audience {
				tokenAud = strings.TrimSpace(tokenAud)
				for _, allowed := range allowedAudiences {
					if tokenAud == allowed {
						matched = true
						if allowed == strings.TrimSpace(authConfig.JWTAdminAudience) && allowed != "" {
							userIsAdmin = true
						}
						break
					}
				}
				if matched {
					break
				}
			}

			if !matched {
				c.AbortWithStatusJSON(http.StatusUnauthorized, models.Error{Error: "invalid token audience"})
				return
			}
		}

		c.Set("jwt_claims", claims)
		c.Set("user_id", claims.Subject)
		c.Set("user_is_admin", userIsAdmin)
		c.Next()
	}
}
