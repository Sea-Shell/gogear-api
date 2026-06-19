package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

func TestJWTMiddlewareSetsStringAndNumericUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const secret = "test-secret"
	claims := jwt.RegisteredClaims{
		Subject:   "42",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("logger", zap.NewNop().Sugar())
		c.Set("auth", &models.Auth{JWTSecret: secret})
		c.Next()
	})
	router.Use(JWTMiddleware())
	router.GET("/protected", func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			t.Fatal("user_id not set")
		}
		if got, ok := userID.(string); !ok || got != "42" {
			t.Fatalf("user_id = %#v (%T), want string 42", userID, userID)
		}

		userIDInt64, ok := c.Get("user_id_int64")
		if !ok {
			t.Fatal("user_id_int64 not set")
		}
		if got, ok := userIDInt64.(int64); !ok || got != 42 {
			t.Fatalf("user_id_int64 = %#v (%T), want int64 42", userIDInt64, userIDInt64)
		}

		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestJWTMiddlewareRejectsNonNumericSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const secret = "test-secret"
	claims := jwt.RegisteredClaims{
		Subject:   "not-numeric",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	handlerCalled := false
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("logger", zap.NewNop().Sugar())
		c.Set("auth", &models.Auth{JWTSecret: secret})
		c.Next()
	})
	router.Use(JWTMiddleware())
	router.GET("/protected", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	if handlerCalled {
		t.Fatal("protected handler was called for non-numeric subject")
	}
}
