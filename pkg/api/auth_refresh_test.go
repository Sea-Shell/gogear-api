package endpoints

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Adversarial regression tests for BR-006 (issued tokens reflect current role).

func (e *authzEnv) refresh(token string) (status int, access string, isAdmin bool) {
	e.T.Helper()
	w := e.Do(http.MethodPost, "/auth/refresh", token, nil)
	var r struct {
		AccessToken string `json:"access_token"`
		User        struct {
			IsAdmin bool `json:"is_admin"`
		} `json:"user"`
	}
	if w.Code == http.StatusOK {
		decodeJSON(e.T, w, &r)
	}
	return w.Code, r.AccessToken, r.User.IsAdmin
}

func (e *authzEnv) deleteCategory(token string, id int64) int {
	e.T.Helper()
	return e.Do(http.MethodDelete, fmt.Sprintf("/api/v1/category/%d/delete", id), token, nil).Code
}

func TestRefresh_DemotedAdminGetsNonAdminToken(t *testing.T) {
	env := newAuthzEnv(t)
	c := env.SeedUser("carol", "carol@example.com", true)
	cat := env.SeedCategory("stoves")
	staleAdminToken := env.Token(c, true)

	if _, err := env.DB.Exec(`UPDATE users SET userIsAdmin = 0 WHERE userId = ?`, c); err != nil {
		t.Fatal(err)
	}

	status, fresh, isAdmin := env.refresh(staleAdminToken)
	if status != http.StatusOK {
		t.Fatalf("refresh status %d", status)
	}
	if isAdmin {
		t.Error("refresh response claims admin for a demoted user")
	}
	if got := env.deleteCategory(fresh, cat); got != http.StatusForbidden {
		t.Fatalf("refreshed token of demoted admin reached an admin operation: %d", got)
	}
	if env.Count("gear_category", "categoryId = ?", cat) != 1 {
		t.Fatal("catalog record was deleted")
	}
}

func TestRefresh_PromotedUserGetsAdminToken(t *testing.T) {
	env := newAuthzEnv(t)
	b := env.SeedUser("bob", "bob@example.com", false)
	cat := env.SeedCategory("packs")
	old := env.Token(b, false)

	if got := env.deleteCategory(old, cat); got != http.StatusForbidden {
		t.Fatalf("precondition: non-admin token delete = %d", got)
	}
	if _, err := env.DB.Exec(`UPDATE users SET userIsAdmin = 1 WHERE userId = ?`, b); err != nil {
		t.Fatal(err)
	}
	status, fresh, isAdmin := env.refresh(old)
	if status != http.StatusOK || !isAdmin {
		t.Fatalf("refresh = %d admin=%v", status, isAdmin)
	}
	if got := env.deleteCategory(fresh, cat); got != http.StatusOK {
		t.Fatalf("promoted user's refreshed token cannot use admin operation: %d", got)
	}
}

func TestRefresh_DeletedAccountRefused(t *testing.T) {
	env := newAuthzEnv(t)
	b := env.SeedUser("bob", "bob@example.com", false)
	tok := env.Token(b, false)
	if _, err := env.DB.Exec(`DELETE FROM users WHERE userId = ?`, b); err != nil {
		t.Fatal(err)
	}
	if status, access, _ := env.refresh(tok); status != http.StatusUnauthorized || access != "" {
		t.Fatalf("status=%d token=%q, want 401 and no token", status, access)
	}
}

func TestRefresh_NeverResolvesAccountByEmail(t *testing.T) {
	env := newAuthzEnv(t)
	env.SeedUser("bob", "bob@example.com", false)
	// The middleware only lets numeric subjects through, so exercise the resolver directly.
	if u, err := resolveUserForSubject(context.Background(), env.DB, "bob@example.com"); err == nil {
		t.Fatalf("resolver fell back to email and returned %+v", u)
	}
	// A numeric subject that is not an account must not match by any other key either.
	if _, err := resolveUserForSubject(context.Background(), env.DB, "424242"); err == nil {
		t.Fatal("resolved a nonexistent user ID")
	}
}

// BR-006 AC-4: with colliding audience configuration, a token issued to a
// non-admin must still not authorize admin-only operations.
func TestAdminOperationsRefuseNonAdminTokenWhenAudiencesCollide(t *testing.T) {
	auth := harnessAuth()
	auth.JWTAdminAudience = auth.JWTAudience
	env := newAuthzEnvWithAuth(t, auth)
	b := env.SeedUser("bob", "bob@example.com", false)
	cat := env.SeedCategory("boots")

	if got := env.deleteCategory(env.Token(b, false), cat); got != http.StatusForbidden {
		t.Fatalf("non-admin token authorized an admin operation under equal audiences: %d", got)
	}
	if env.Count("gear_category", "categoryId = ?", cat) != 1 {
		t.Fatal("catalog record was deleted")
	}
}

// The service token it issues for a non-admin must not carry the admin audience.
func TestIssueServiceToken_AudienceMatchesStoredRole(t *testing.T) {
	env := newAuthzEnv(t)
	id := int64(7)
	for _, admin := range []bool{false, true} {
		tok, _, err := issueServiceToken(env.Auth, &models.User{UserID: &id, UserIsAdmin: admin})
		if err != nil {
			t.Fatal(err)
		}
		cat := env.SeedCategory("c")
		env.SeedUser(fmt.Sprintf("u%v", admin), fmt.Sprintf("u%v@example.com", admin), admin) // id irrelevant: middleware trusts token
		want := http.StatusForbidden
		if admin {
			want = http.StatusOK
		}
		if got := env.deleteCategory(tok, cat); got != want {
			t.Errorf("admin=%v token: delete = %d, want %d", admin, got, want)
		}
	}
}
