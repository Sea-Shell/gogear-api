package endpoints

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
)

// Adversarial regression tests for BR-019 (permanent first admin) and its
// refusals in BR-009 AC-6, BR-010 AC-2 and BR-018, through the real JWT
// middleware and a migrated database.

// permanentAdmin returns the stored permanent admin, or 0 when none is designated.
func (e *authzEnv) permanentAdmin() int64 {
	e.T.Helper()
	var id int64
	err := e.DB.QueryRow(`SELECT userId FROM permanent_admin`).Scan(&id)
	if err == sql.ErrNoRows {
		return 0
	}
	if err != nil {
		e.T.Fatalf("read permanent admin: %v", err)
	}
	return id
}

func setAdmin(env *authzEnv, actor, target int64, admin bool) int {
	return env.Do(http.MethodPost, userPath(target, "admin"), env.Token(actor, true), map[string]any{"user_is_admin": admin}).Code
}

// permWorld: user 1 is an ordinary account, 2 is the first admin (permanent),
// 3 and 4 are later admins.
type permWorld struct {
	env                *authzEnv
	plain, first, a, b int64
}

func newPermWorld(t *testing.T) *permWorld {
	env := newAuthzEnv(t)
	w := &permWorld{env: env}
	w.plain = env.SeedUser("plain", "plain@example.com", false)
	w.first = env.SeedUser("first", "first@example.com", true)
	w.a = env.SeedUser("adminA", "a@example.com", true)
	w.b = env.SeedUser("adminB", "b@example.com", true)
	if got := env.permanentAdmin(); got != w.first {
		t.Fatalf("permanent admin = %d, want the first admin %d", got, w.first)
	}
	return w
}

// AC-3, AC-4: nobody can revoke the permanent admin, not another admin and not
// the permanent admin; the refusal is 403 and changes nothing.
func TestPermanentAdmin_RevocationRefusedForEveryCaller(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	before := env.User(w.first)

	for name, actor := range map[string]int64{"other admin": w.a, "itself": w.first} {
		if code := setAdmin(env, actor, w.first, false); code != http.StatusForbidden {
			t.Fatalf("%s revoking the permanent admin: status %d, want 403", name, code)
		}
	}
	if got := env.User(w.first); got != before || !got.IsAdmin {
		t.Fatalf("refused revoke changed the account: %+v -> %+v", before, got)
	}
	// The next refresh still yields an admin token (AC-3, BR-006).
	r := env.Do(http.MethodPost, "/auth/refresh", env.Token(w.first, false), nil)
	expectStatus(t, r, http.StatusOK)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	decodeJSON(t, r, &tok)
	cat := env.SeedCategory("tents")
	expectStatus(t, env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/category/%d/delete", cat), tok.AccessToken, nil), http.StatusOK)
}

// Precedence (BR-018 AC-4): a non-admin is refused as non-admin, and an unknown
// user is still 404.
func TestPermanentAdmin_RefusalPrecedence(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	if code := env.Do(http.MethodPost, userPath(w.first, "admin"), env.Token(w.plain, false), map[string]any{"user_is_admin": false}).Code; code != http.StatusForbidden {
		t.Fatalf("non-admin: %d", code)
	}
	if code := setAdmin(env, w.a, 9999, false); code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", code)
	}
	expectStatus(t, env.Do(http.MethodPost, userPath(w.first, "admin"), "", map[string]any{"user_is_admin": false}), http.StatusUnauthorized)
}

// AC-4: granting admin status to the permanent admin succeeds and changes nothing.
func TestPermanentAdmin_GrantIsNoOpSuccess(t *testing.T) {
	w := newPermWorld(t)
	before := w.env.User(w.first)
	if code := setAdmin(w.env, w.a, w.first, true); code != http.StatusOK {
		t.Fatalf("grant to permanent admin: %d", code)
	}
	if got := w.env.User(w.first); got != before {
		t.Fatalf("grant changed the account: %+v -> %+v", before, got)
	}
	if w.env.permanentAdmin() != w.first {
		t.Fatal("designation moved")
	}
}

// AC-7, P-10: other admins have no protection, and may revoke themselves.
func TestPermanentAdmin_OtherAdminsCanBeRevokedIncludingSelf(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	if code := setAdmin(env, w.first, w.a, false); code != http.StatusOK || env.User(w.a).IsAdmin {
		t.Fatalf("permanent admin revoking another admin: %d admin=%v", code, env.User(w.a).IsAdmin)
	}
	if code := setAdmin(env, w.b, w.b, false); code != http.StatusOK || env.User(w.b).IsAdmin {
		t.Fatalf("admin revoking itself: %d admin=%v", code, env.User(w.b).IsAdmin)
	}
	if env.permanentAdmin() != w.first || !env.User(w.first).IsAdmin {
		t.Fatal("permanent admin affected")
	}
}

// AC-2: the designation never moves: not by promoting a lower ID, not by
// demoting other admins, not by re-granting.
func TestPermanentAdmin_DesignationNeverMoves(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	if code := setAdmin(env, w.first, w.plain, true); code != http.StatusOK { // lower ID than first
		t.Fatalf("promote: %d", code)
	}
	setAdmin(env, w.first, w.a, false)
	setAdmin(env, w.first, w.plain, false)
	setAdmin(env, w.first, w.a, true)
	if got := env.permanentAdmin(); got != w.first {
		t.Fatalf("designation moved to %d", got)
	}
}

// AC-5: the permanent admin cannot delete their own account (403); account and
// data stay. Others cannot delete it either (BR-009 AC-1, AC-2).
func TestPermanentAdmin_CannotBeDeleted(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	seedTenant(env, w.first, "first-pub")
	user, data := env.User(w.first), env.data(w.first)
	if data == (tenantData{}) {
		t.Fatal("seed produced no data")
	}

	for name, actor := range map[string]int64{"itself": w.first, "other admin": w.a} {
		if code := env.Do(http.MethodDelete, userPath(w.first, "delete"), env.Token(actor, true), nil).Code; code != http.StatusForbidden {
			t.Fatalf("%s deleting the permanent admin: %d, want 403", name, code)
		}
	}
	if env.User(w.first) != user || env.data(w.first) != data {
		t.Fatal("refused delete changed the account or its data")
	}
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/public/loadout/first-pub", "", nil), http.StatusOK)

	// Other admins can still delete themselves.
	if code := env.Do(http.MethodDelete, userPath(w.a, "delete"), env.Token(w.a, true), nil).Code; code != http.StatusOK {
		t.Fatalf("ordinary admin self-delete: %d", code)
	}
}

// Refused attempts on the permanent admin are logged with actor and target.
func TestPermanentAdmin_RefusalsAreLogged(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	setAdmin(env, w.a, w.first, false)
	env.Do(http.MethodDelete, userPath(w.first, "delete"), env.Token(w.first, true), nil)

	want := map[string]int64{
		"refused to revoke the permanent admin": w.a,
		"refused to delete the permanent admin": w.first,
	}
	for _, e := range env.Logs.All() {
		actor, ok := want[e.Message]
		if !ok {
			continue
		}
		f := e.ContextMap()
		if f["actor_user_id"] != actor || f["target_user_id"] != w.first {
			t.Fatalf("%q fields = %v, want actor %d target %d", e.Message, f, actor, w.first)
		}
		delete(want, e.Message)
	}
	if len(want) != 0 {
		t.Fatalf("missing log entries %v in %v", want, env.Logs.All())
	}
}

// AC-6: profile update cannot demote the permanent admin either.
func TestPermanentAdmin_ProfileUpdateCannotDemote(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	before := env.User(w.first)
	expectStatus(t, env.Do(http.MethodPost, userPath(w.first, "update"), env.Token(w.first, true), map[string]any{"user_is_admin": false}), http.StatusBadRequest)
	if env.User(w.first) != before {
		t.Fatal("profile update changed the permanent admin")
	}
}

// AC-1: with no admin yet, the first account that becomes admin, by any means,
// is the permanent admin; later admins never replace it, even if it is removed
// outside the API (AC-8).
func TestPermanentAdmin_FirstAccountToBecomeAdminIsDesignated(t *testing.T) {
	env := newAuthzEnv(t)
	u1 := env.SeedUser("one", "one@example.com", false)
	u2 := env.SeedUser("two", "two@example.com", false)
	if env.permanentAdmin() != 0 {
		t.Fatal("designated with no admin")
	}
	// Becomes admin outside the API.
	if _, err := env.DB.Exec(`UPDATE users SET userIsAdmin = 1 WHERE userId = ?`, u2); err != nil {
		t.Fatal(err)
	}
	if got := env.permanentAdmin(); got != u2 {
		t.Fatalf("permanent = %d, want %d", got, u2)
	}
	if code := setAdmin(env, u2, u1, true); code != http.StatusOK {
		t.Fatalf("grant: %d", code)
	}
	if got := env.permanentAdmin(); got != u2 {
		t.Fatalf("designation moved to %d", got)
	}

	// AC-8: removed outside the API, nobody takes its place.
	if _, err := env.DB.Exec(`DELETE FROM users WHERE userId = ?`, u2); err != nil {
		t.Fatal(err)
	}
	u3 := env.SeedUser("three", "three@example.com", true)
	if got := env.permanentAdmin(); got != u2 || got == u3 {
		t.Fatalf("designation = %d after removal, want it to stay %d", got, u2)
	}
}

// OC-001/BR-018 style: the check and the change are one operation, so a burst
// of revocations by other admins can never demote the permanent admin.
func TestPermanentAdmin_ConcurrentRevokesNeverDemote(t *testing.T) {
	w := newPermWorld(t)
	env := w.env
	done := make(chan int, 40)
	for i := 0; i < 40; i++ {
		go func(i int) {
			actor := []int64{w.a, w.b, w.first}[i%3]
			done <- setAdmin(env, actor, w.first, i%5 == 0)
		}(i)
	}
	for i := 0; i < 40; i++ {
		if code := <-done; code != http.StatusOK && code != http.StatusForbidden {
			t.Fatalf("unexpected status %d", code)
		}
	}
	if !env.User(w.first).IsAdmin {
		t.Fatal("permanent admin was demoted")
	}
}
