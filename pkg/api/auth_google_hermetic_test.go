package endpoints

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	googleOAuth "golang.org/x/oauth2/google"
)

// Hermetic compatibility checks for Google sign-in dependencies.
//
// These tests run the real google.golang.org/api/idtoken validator, the real
// golang.org/x/oauth2 code exchange and the real GoogleAuthCallback handler.
// Only the network is replaced: http.DefaultTransport (which both libraries use
// when no client is given) answers Google's two endpoints in process, with a
// locally generated RSA key. No socket is opened, no Google service is
// contacted and no credential is used. The tests exist so that a change of the
// google.golang.org/api, golang.org/x/oauth2 or cloud.google.com/go/auth
// versions can be shown not to change what sign-in accepts and rejects.
//
// They must not run in parallel: they swap the process-wide default transport.

const (
	// Constants of the libraries under test. If an upgrade moves them, the
	// fake transport refuses the request and these tests fail loudly.
	hermeticCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

	hermeticKeyID    = "hermetic-key-1"
	hermeticAudience = "harness-google-client" // harnessAuth().GoogleClientID
)

var (
	hermeticKeysOnce sync.Once
	hermeticKey      *rsa.PrivateKey // published in the fake JWKS
	hermeticOtherKey *rsa.PrivateKey // never published: signs "bad signature" tokens
)

func hermeticKeys(t *testing.T) (published, unpublished *rsa.PrivateKey) {
	t.Helper()
	hermeticKeysOnce.Do(func() {
		var err error
		if hermeticKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatalf("generate key: %v", err)
		}
		if hermeticOtherKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatalf("generate key: %v", err)
		}
	})
	if hermeticKey == nil || hermeticOtherKey == nil {
		t.Fatal("test keys were not generated")
	}
	return hermeticKey, hermeticOtherKey
}

// fakeGoogle is the in-process stand-in for Google's certificate and token
// endpoints.
type fakeGoogle struct {
	t *testing.T

	mu sync.Mutex
	// certHits counts fetches of the certificate URL.
	certHits int
	// certStatus overrides the certificate response status when non-zero.
	certStatus int
	// tokenForms records the form of each token request.
	tokenForms []url.Values
	// tokenStatus and tokenBody are the token endpoint answer.
	tokenStatus int
	tokenBody   string
}

// useFakeGoogle routes the default transport to a fakeGoogle until the test
// ends. It replaces a process-wide variable, so callers must not be parallel.
func useFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	f := &fakeGoogle{t: t}
	orig := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = orig })
	return f
}

func (f *fakeGoogle) certs() (hits int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.certHits
}

func (f *fakeGoogle) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch req.URL.String() {
	case hermeticCertsURL:
		f.certHits++
		status := f.certStatus
		if status == 0 {
			status = http.StatusOK
		}
		return f.respond(req, status, f.jwks()), nil
	case googleOAuth.Endpoint.TokenURL:
		body, _ := io.ReadAll(req.Body)
		form, err := url.ParseQuery(string(body))
		if err != nil {
			f.t.Errorf("token request body is not a form: %v", err)
		}
		f.tokenForms = append(f.tokenForms, form)
		status := f.tokenStatus
		if status == 0 {
			status = http.StatusOK
		}
		return f.respond(req, status, f.tokenBody), nil
	default:
		// Anything else would have been a real network call.
		f.t.Errorf("unexpected outbound request: %s %s", req.Method, req.URL)
		return nil, io.ErrNoProgress
	}
}

// respond builds a JSON response. It sets no Cache-Control header, so the
// idtoken certificate cache treats every answer as already expired and each
// validation fetches the key set again: no state leaks between cases.
func (f *fakeGoogle) respond(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func (f *fakeGoogle) jwks() string {
	key, _ := hermeticKeys(f.t)
	set := map[string]any{"keys": []map[string]string{{
		"kid": hermeticKeyID,
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"n":   b64(key.N.Bytes()),
		"e":   b64(big.NewInt(int64(key.E)).Bytes()),
	}}}
	out, err := json.Marshal(set)
	if err != nil {
		f.t.Fatalf("marshal jwks: %v", err)
	}
	return string(out)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// signGoogleToken builds an RS256 compact JWT shaped like a Google ID token.
func signGoogleToken(t *testing.T, key *rsa.PrivateKey, kid, alg string, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signing := b64(header) + "." + b64(payload)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signing + "." + b64(sig)
}

func googleClaims(overrides map[string]any) map[string]any {
	claims := map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            hermeticAudience,
		"sub":            "google-subject-1",
		"email":          "pilot@example.com",
		"email_verified": true,
		"name":           "Pilot Example",
		"iat":            time.Now().Add(-time.Minute).Unix(),
		"exp":            time.Now().Add(time.Hour).Unix(),
	}
	for k, v := range overrides {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	return claims
}

// TestGoogleIDTokenValidationWithRealLibrary runs validateGoogleIDToken, the
// wrapper sign-in uses, against the real idtoken validator.
func TestGoogleIDTokenValidationWithRealLibrary(t *testing.T) {
	key, other := hermeticKeys(t)

	tests := []struct {
		name         string
		token        func() string
		audience     string
		certStatus   int
		wantErr      bool
		wantCertHits int
		want         GoogleIdentity
	}{
		{
			name:         "valid token",
			token:        func() string { return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(nil)) },
			audience:     hermeticAudience,
			wantCertHits: 1,
			want:         GoogleIdentity{Subject: "google-subject-1", Email: "pilot@example.com", EmailVerified: true, Name: "Pilot Example"},
		},
		{
			name: "email_verified as a string and name from given and family name",
			token: func() string {
				return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{
					"email_verified": "true", "name": nil, "given_name": "Ada", "family_name": "Lovelace",
				}))
			},
			audience:     hermeticAudience,
			wantCertHits: 1,
			want:         GoogleIdentity{Subject: "google-subject-1", Email: "pilot@example.com", EmailVerified: true, Name: "Ada Lovelace"},
		},
		{
			name: "unverified email is reported as unverified",
			token: func() string {
				return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{"email_verified": false}))
			},
			audience:     hermeticAudience,
			wantCertHits: 1,
			want:         GoogleIdentity{Subject: "google-subject-1", Email: "pilot@example.com", EmailVerified: false, Name: "Pilot Example"},
		},
		{
			name: "wrong audience",
			token: func() string {
				return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{"aud": "someone-else"}))
			},
			audience: hermeticAudience,
			wantErr:  true,
		},
		{
			name: "expired token",
			token: func() string {
				return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}))
			},
			audience: hermeticAudience,
			wantErr:  true,
		},
		{
			name:         "signature from a key Google did not publish",
			token:        func() string { return signGoogleToken(t, other, hermeticKeyID, "RS256", googleClaims(nil)) },
			audience:     hermeticAudience,
			wantErr:      true,
			wantCertHits: 1,
		},
		{
			name:         "key id Google does not publish",
			token:        func() string { return signGoogleToken(t, key, "unknown-kid", "RS256", googleClaims(nil)) },
			audience:     hermeticAudience,
			wantErr:      true,
			wantCertHits: 1,
		},
		{
			name:     "unsupported signing algorithm",
			token:    func() string { return signGoogleToken(t, key, hermeticKeyID, "RS512", googleClaims(nil)) },
			audience: hermeticAudience,
			wantErr:  true,
		},
		{
			name:     "not a JWT",
			token:    func() string { return "opaque" },
			audience: hermeticAudience,
			wantErr:  true,
		},
		{
			name:     "audience not configured",
			token:    func() string { return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(nil)) },
			audience: "  ",
			wantErr:  true,
		},
		{
			name:         "certificate endpoint failure",
			token:        func() string { return signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(nil)) },
			audience:     hermeticAudience,
			certStatus:   http.StatusInternalServerError,
			wantErr:      true,
			wantCertHits: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := useFakeGoogle(t)
			fake.certStatus = tc.certStatus

			got, err := validateGoogleIDToken(context.Background(), tc.token(), tc.audience)

			if hits := fake.certs(); hits != tc.wantCertHits {
				t.Errorf("certificate fetches = %d, want %d", hits, tc.wantCertHits)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted %+v, want an error", got)
				}
				if got != nil {
					t.Errorf("error with identity %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("rejected a valid token: %v", err)
			}
			if *got != tc.want {
				t.Errorf("identity = %+v, want %+v", *got, tc.want)
			}
		})
	}
}

// googleCallbackRouter serves the real GoogleAuthCallback with no injected
// validator, so the production default (googleValidatorFrom falling back to
// validateGoogleIDToken) is the code that runs.
func googleCallbackRouter(env *authzEnv, oauthCfg *oauth2.Config) *gin.Engine {
	logger := zap.NewNop().Sugar()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", env.DB)
		c.Set("logger", logger)
		c.Set("auth", env.Auth)
		c.Set("oauth", oauthCfg)
		c.Next()
	})
	router.POST("/auth/google/callback", GoogleAuthCallback)
	return router
}

func postGoogleCallback(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/google/callback", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertSignedIn(t *testing.T, w *httptest.ResponseRecorder, email string) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var out struct {
		AccessToken string `json:"access_token"`
		User        struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	decodeJSON(t, w, &out)
	if out.AccessToken == "" || out.User.Email != email {
		t.Fatalf("sign-in answer = %s, want a token for %s", w.Body.String(), email)
	}
}

// TestGoogleSignInHandlerWithRealValidation drives POST /auth/google/callback
// with an id_token: real handler, real database, real idtoken validation.
func TestGoogleSignInHandlerWithRealValidation(t *testing.T) {
	key, other := hermeticKeys(t)
	env := newAuthzEnv(t)
	router := googleCallbackRouter(env, &oauth2.Config{})
	useFakeGoogle(t)

	valid := signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(nil))
	assertSignedIn(t, postGoogleCallback(router, `{"id_token":"`+valid+`"}`), "pilot@example.com")

	for name, token := range map[string]string{
		"wrong audience": signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{"aud": "someone-else"})),
		"expired":        signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})),
		"bad signature":  signGoogleToken(t, other, hermeticKeyID, "RS256", googleClaims(map[string]any{"sub": "google-subject-2", "email": "intruder@example.com"})),
	} {
		t.Run(name, func(t *testing.T) {
			w := postGoogleCallback(router, `{"id_token":"`+token+`"}`)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "access_token") {
				t.Fatalf("refused sign-in carries a token: %s", w.Body.String())
			}
		})
	}

	var count int
	if err := env.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE userEmail = 'intruder@example.com'`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Errorf("a rejected token created %d user(s)", count)
	}
}

// TestGoogleSignInCodeExchangeWithRealOAuth2 drives the authorization-code
// path: the real oauth2.Config built like main.go does (google.Endpoint) posts
// to Google's token URL, the answer carries an id_token, and the real validator
// checks it.
func TestGoogleSignInCodeExchangeWithRealOAuth2(t *testing.T) {
	key, _ := hermeticKeys(t)
	env := newAuthzEnv(t)
	oauthCfg := &oauth2.Config{
		ClientID:     hermeticAudience,
		ClientSecret: "not-a-real-secret",
		RedirectURL:  "https://app.example.invalid/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     googleOAuth.Endpoint,
	}
	router := googleCallbackRouter(env, oauthCfg)

	t.Run("code exchange then validation", func(t *testing.T) {
		fake := useFakeGoogle(t)
		idToken := signGoogleToken(t, key, hermeticKeyID, "RS256", googleClaims(map[string]any{"email": "coder@example.com", "sub": "google-subject-code"}))
		resp, err := json.Marshal(map[string]any{
			"access_token": "opaque-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
		if err != nil {
			t.Fatalf("marshal token response: %v", err)
		}
		fake.tokenBody = string(resp)

		assertSignedIn(t, postGoogleCallback(router, `{"code":"auth-code-1"}`), "coder@example.com")

		if len(fake.tokenForms) != 1 {
			t.Fatalf("token requests = %d, want 1", len(fake.tokenForms))
		}
		form := fake.tokenForms[0]
		for k, want := range map[string]string{
			"grant_type":    "authorization_code",
			"code":          "auth-code-1",
			"client_id":     hermeticAudience,
			"client_secret": "not-a-real-secret",
			"redirect_uri":  "https://app.example.invalid/callback",
		} {
			if got := form.Get(k); got != want {
				t.Errorf("token request %s = %q, want %q", k, got, want)
			}
		}
	})

	t.Run("token endpoint refuses the code", func(t *testing.T) {
		fake := useFakeGoogle(t)
		fake.tokenStatus = http.StatusBadRequest
		fake.tokenBody = `{"error":"invalid_grant"}`

		w := postGoogleCallback(router, `{"code":"stale-code"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
		}
		if fake.certs() != 0 {
			t.Errorf("certificates were fetched without a token to validate")
		}
	})

	t.Run("token response without id_token", func(t *testing.T) {
		fake := useFakeGoogle(t)
		fake.tokenBody = `{"access_token":"opaque","token_type":"Bearer","expires_in":3600}`

		w := postGoogleCallback(router, `{"code":"auth-code-2"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
		}
	})
}
