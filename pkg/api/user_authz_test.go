package endpoints

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Adversarial regression tests for BR-007..BR-011, BR-017, BR-018 (users).

func userPath(id int64, op string) string { return fmt.Sprintf("/api/v1/users/%d/%s", id, op) }

// ---- BR-007: directory visibility ------------------------------------------

func TestUserDirectory_NonAdminCannotSeeOthersEmailOrAdmin(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob-secret@example.com", true)
	// Google-created accounts use the email as username and name.
	g := env.SeedUser("gina-google@example.com", "gina-google@example.com", false)
	if _, err := env.DB.Exec(`UPDATE users SET userName = ? WHERE userId = ?`, "Gina-Google@Example.com", g); err != nil {
		t.Fatal(err)
	}
	tok := env.Token(a, false)

	for _, tc := range []struct{ name, path string }{
		{"list", "/api/v1/users/list"},
		{"get other", userPath(b, "get")},
		{"get google other", userPath(g, "get")},
	} {
		w := env.Do(http.MethodGet, tc.path, tok, nil)
		expectStatus(t, w, http.StatusOK)
		body := strings.ToLower(w.Body.String())
		for _, leak := range []string{"bob-secret@example.com", "gina-google@example.com", "hash-of-"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s: response leaks %q: %s", tc.name, leak, w.Body.String())
			}
		}
	}

	// Other users' entries carry no admin-status key at all; only the caller's does.
	var list struct {
		Items []map[string]any `json:"items"`
	}
	w := env.Do(http.MethodGet, "/api/v1/users/list", tok, nil)
	decodeJSON(t, w, &list)
	if len(list.Items) != 3 {
		t.Fatalf("want 3 users listed, got %d: %s", len(list.Items), w.Body.String())
	}
	for _, item := range list.Items {
		id := int64(item["user_id"].(float64))
		_, hasEmail := item["user_email"]
		_, hasAdmin := item["user_is_admin"]
		if id == a {
			if !hasEmail || !hasAdmin || item["user_email"] != "alice@example.com" || item["user_is_admin"] != false {
				t.Errorf("own entry must include email and admin status: %v", item)
			}
		} else if hasEmail || hasAdmin {
			t.Errorf("entry for user %d reveals email/admin status: %v", id, item)
		}
	}
}

func TestUserDirectory_OwnProfileShowsEmailAndAdmin(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	w := env.Do(http.MethodGet, userPath(a, "get"), env.Token(a, false), nil)
	expectStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeJSON(t, w, &got)
	if got["user_email"] != "alice@example.com" {
		t.Errorf("own email missing: %v", got)
	}
	if v, ok := got["user_is_admin"]; !ok || v != false {
		t.Errorf("own admin status missing: %v", got)
	}
	if _, ok := got["user_password"]; ok {
		t.Errorf("password must never be exposed: %v", got)
	}
}

func TestUserDirectory_AdminSeesEverything(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	c := env.SeedUser("carol", "carol@example.com", true)

	for _, path := range []string{userPath(a, "get"), "/api/v1/users/list"} {
		w := env.Do(http.MethodGet, path, env.Token(c, true), nil)
		expectStatus(t, w, http.StatusOK)
		body := w.Body.String()
		if !strings.Contains(body, "alice@example.com") || !strings.Contains(body, `"user_is_admin"`) {
			t.Errorf("%s: admin must see email and admin status: %s", path, body)
		}
		if strings.Contains(body, "hash-of-") {
			t.Errorf("%s: password leaked: %s", path, body)
		}
	}
}

func TestUserDirectory_NonAdminSearchIsNoEmailOracle(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	env.SeedUser("bob", "bob-secret@example.com", false)
	env.SeedUser("gina-google@example.com", "gina-google@example.com", false)
	tok := env.Token(a, false)

	// Searching on the email field is an admin capability.
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list?email=secret", tok, nil), http.StatusForbidden)

	// Searching username/name must not match text that is redacted from the response.
	w := env.Do(http.MethodGet, "/api/v1/users/list?user=gina-google", tok, nil)
	expectStatus(t, w, http.StatusOK)
	var list struct {
		TotalItemCount int `json:"total_item_count"`
	}
	decodeJSON(t, w, &list)
	if list.TotalItemCount != 0 {
		t.Errorf("search matched a username that is an email: %s", w.Body.String())
	}
}

func TestUserDirectory_RequiresToken(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/users/list", "", nil), http.StatusUnauthorized)
	expectStatus(t, env.Do(http.MethodGet, userPath(a, "get"), "", nil), http.StatusUnauthorized)
}

func TestGetUser_MalformedAndUnknownID(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	tok := env.Token(a, false)

	w := env.Do(http.MethodGet, "/api/v1/users/abc/get", tok, nil)
	expectStatus(t, w, http.StatusBadRequest)
	var e map[string]any
	decodeJSON(t, w, &e) // exactly one JSON document: no fall-through after the 400

	expectStatus(t, env.Do(http.MethodGet, userPath(9999, "get"), tok, nil), http.StatusNotFound)
}

// ---- BR-008 / BR-017: profile update -----------------------------------------

func TestUpdateUser_OtherUsersProfileRefused(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	c := env.SeedUser("carol", "carol@example.com", true)
	before := env.User(b)

	body := map[string]any{"user_id": b, "user_username": "pwned", "user_name": "pwned", "user_email": "pwned@evil.test"}
	// non-admin, via B's path
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "update"), env.Token(a, false), body), http.StatusForbidden)
	// non-admin, own path but body targets B (identifier never redirects the update)
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), env.Token(a, false), body), http.StatusBadRequest)
	// admin gets no exception (P-3)
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "update"), env.Token(c, true), body), http.StatusForbidden)

	if after := env.User(b); after != before {
		t.Fatalf("victim changed: before %+v after %+v", before, after)
	}
	if got := env.User(a); got.Username != "alice" || got.Email != "alice@example.com" {
		t.Fatalf("caller changed by refused request: %+v", got)
	}
}

func TestUpdateUser_CannotSetAdminStatus(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	tok := env.Token(a, false)

	for _, body := range []map[string]any{
		{"user_username": "alice", "user_is_admin": true},
		{"user_id": a, "user_username": "alice", "user_name": "A", "user_email": "alice@example.com", "user_is_admin": true},
	} {
		expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, body), http.StatusBadRequest)
		if env.User(a).IsAdmin {
			t.Fatalf("non-admin made themselves admin with %v", body)
		}
	}

	// A refreshed token must still be a non-admin token.
	w := env.Do(http.MethodPost, "/auth/refresh", tok, nil)
	expectStatus(t, w, http.StatusOK)
	var r struct {
		AccessToken string `json:"access_token"`
	}
	decodeJSON(t, w, &r)
	c := env.SeedCategory("hiking")
	expectStatus(t, env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/category/%d/delete", c), r.AccessToken, nil), http.StatusForbidden)
}

func TestUpdateUser_AdminCannotChangeAdminStatusThroughProfileUpdate(t *testing.T) {
	env := newAuthzEnv(t)
	c := env.SeedUser("carol", "carol@example.com", true)
	// An admin demoting themselves through profile update is also refused (BR-017 AC-3).
	expectStatus(t, env.Do(http.MethodPost, userPath(c, "update"), env.Token(c, true),
		map[string]any{"user_username": "carol", "user_is_admin": false}), http.StatusBadRequest)
	if !env.User(c).IsAdmin {
		t.Fatal("admin status changed through profile update")
	}
}

func TestUpdateUser_SelfUpdateKeepsPasswordAndOmittedFields(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", true)
	tok := env.Token(a, true)

	// Full body with matching id and matching admin status proceeds.
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, map[string]any{
		"user_id": a, "user_username": "alice2", "user_name": "Alice Two", "user_email": "alice2@example.com", "user_is_admin": true,
	}), http.StatusOK)
	got := env.User(a)
	if got.Username != "alice2" || got.Name != "Alice Two" || got.Email != "alice2@example.com" {
		t.Fatalf("self update not applied: %+v", got)
	}
	if got.Password != "hash-of-alice" {
		t.Fatalf("update wiped or changed the password: %q", got.Password)
	}
	if !got.IsAdmin {
		t.Fatal("self update changed admin status")
	}

	// Partial body: omitted fields are left alone.
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, map[string]any{"user_name": "Only Name"}), http.StatusOK)
	got = env.User(a)
	if got.Name != "Only Name" || got.Username != "alice2" || got.Email != "alice2@example.com" || got.Password != "hash-of-alice" {
		t.Fatalf("partial update disturbed other fields: %+v", got)
	}

	// Blank identity-bearing fields are rejected.
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, map[string]any{"user_email": "  "}), http.StatusBadRequest)
	if env.User(a).Email != "alice2@example.com" {
		t.Fatal("blank email stored")
	}
}

func TestUpdateUser_RequiresToken(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), "", map[string]any{"user_name": "x"}), http.StatusUnauthorized)
	if env.User(a).Name != "alice Full" {
		t.Fatal("unauthenticated update changed data")
	}
}

// ---- BR-011: direct creation ---------------------------------------------------

func TestInsertUser_AdminOnlyAndNeverAdmin(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	c := env.SeedUser("carol", "carol@example.com", true)
	base := env.Count("users", "")

	newUser := map[string]any{"user_username": "dave", "user_name": "Dave", "user_email": "dave@example.com"}

	// non-admin: refused, nothing created (even asking for admin)
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", env.Token(a, false), newUser), http.StatusForbidden)
	withAdmin := map[string]any{"user_username": "eve", "user_name": "Eve", "user_email": "eve@example.com", "user_is_admin": true}
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", env.Token(a, false), withAdmin), http.StatusForbidden)
	if n := env.Count("users", ""); n != base {
		t.Fatalf("non-admin created accounts: %d -> %d", base, n)
	}

	// admin asking for admin: refused, no admin account created
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", env.Token(c, true), withAdmin), http.StatusBadRequest)
	if n := env.Count("users", ""); n != base {
		t.Fatalf("rejected create stored a row: %d -> %d", base, n)
	}

	// admin, plain create: a non-admin account exists, with no password material
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", env.Token(c, true), newUser), http.StatusOK)
	if n := env.Count("users", "userEmail = 'dave@example.com' AND userIsAdmin = 0"); n != 1 {
		t.Fatalf("expected one non-admin dave, got %d", n)
	}
	// explicit admin=false is the same as omitted
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", env.Token(c, true),
		map[string]any{"user_username": "fay", "user_email": "fay@example.com", "user_is_admin": false}), http.StatusOK)
	if n := env.Count("users", "userIsAdmin = 1"); n != 1 {
		t.Fatalf("admin count changed: %d", n)
	}
	// missing required fields
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", env.Token(c, true), map[string]any{"user_name": "x"}), http.StatusBadRequest)
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/users/insert", "", newUser), http.StatusUnauthorized)
}

// ---- BR-010: dedicated admin-status operation -------------------------------------

func TestSetUserAdmin_AdminOnly(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	c := env.SeedUser("carol", "carol@example.com", true)
	on := map[string]any{"user_is_admin": true}

	// non-admin: refused for self and others
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "admin"), env.Token(a, false), on), http.StatusForbidden)
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "admin"), env.Token(a, false), on), http.StatusForbidden)
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "admin"), "", on), http.StatusUnauthorized)
	if env.User(a).IsAdmin || env.User(b).IsAdmin {
		t.Fatal("non-admin changed admin status")
	}

	// admin grants: stored value changes, next refresh is an admin token
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "admin"), env.Token(c, true), on), http.StatusOK)
	if !env.User(b).IsAdmin {
		t.Fatal("grant not stored")
	}
	cat := env.SeedCategory("tents")
	w := env.Do(http.MethodPost, "/auth/refresh", env.Token(b, false), nil)
	expectStatus(t, w, http.StatusOK)
	var r struct {
		AccessToken string `json:"access_token"`
	}
	decodeJSON(t, w, &r)
	expectStatus(t, env.Do(http.MethodDelete, fmt.Sprintf("/api/v1/category/%d/delete", cat), r.AccessToken, nil), http.StatusOK)

	// admin revokes
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "admin"), env.Token(c, true), map[string]any{"user_is_admin": false}), http.StatusOK)
	if env.User(b).IsAdmin {
		t.Fatal("revoke not stored")
	}

	// bad input
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "admin"), env.Token(c, true), map[string]any{}), http.StatusBadRequest)
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "admin"), env.Token(c, true), `{"user_is_admin":"yes"}`), http.StatusBadRequest)
	expectStatus(t, env.Do(http.MethodPost, "/api/v1/users/xyz/admin", env.Token(c, true), on), http.StatusBadRequest)
	expectStatus(t, env.Do(http.MethodPost, userPath(9999, "admin"), env.Token(c, true), on), http.StatusNotFound)
}

// ---- BR-009: deletion -------------------------------------------------------------

type tenantData struct{ regs, links, loadouts, items int }

func (e *authzEnv) data(userID int64) tenantData {
	return tenantData{
		regs:     e.Count("user_gear_registrations", "userId = ?", userID),
		links:    e.Count("user_container_registration", "userContainerId IN (SELECT userGearRegistrationId FROM user_gear_registrations WHERE userId = ?) OR userGearRegistrationId IN (SELECT userGearRegistrationId FROM user_gear_registrations WHERE userId = ?)", userID, userID),
		loadouts: e.Count("loadouts", "userId = ?", userID),
		items:    e.Count("loadout_items", "loadoutId IN (SELECT loadoutId FROM loadouts WHERE userId = ?)", userID),
	}
}

// tenantGear returns two catalog gear IDs shared by every seeded tenant,
// creating them on first use (foreign keys need real gear rows).
func tenantGear(env *authzEnv) (int64, int64) {
	var ids []int64
	rows, err := env.DB.Query(`SELECT gearId FROM gear WHERE gearName = 'g' ORDER BY gearId LIMIT 2`)
	if err != nil {
		env.T.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			env.T.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	for len(ids) < 2 {
		ids = append(ids, env.seedGearW(100))
	}
	return ids[0], ids[1]
}

// seedTenant gives a user a container with content, a public loadout and an item.
func seedTenant(env *authzEnv, userID int64, slug string) {
	g1, g2 := tenantGear(env)
	box := env.SeedRegistration(userID, env.capGear())
	thing := env.SeedRegistration(userID, g2)
	env.SeedContainerLink(box, thing)
	lo := seedLoadout(env.T, env.DB, userID, true, slug)
	seedLoadoutItem(env.T, env.DB, lo, g1)
	seedLoadoutItem(env.T, env.DB, lo, g2)
}

func TestDeleteUser_OnlyOwnAccount(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	c := env.SeedUser("carol", "carol@example.com", true)
	seedTenant(env, a, "alice-pub")
	seedTenant(env, b, "bob-pub")
	bUser, bData := env.User(b), env.data(b)
	if bData != (tenantData{2, 1, 1, 2}) {
		t.Fatalf("seed unexpected: %+v", bData)
	}

	expectStatus(t, env.Do(http.MethodDelete, userPath(b, "delete"), env.Token(a, false), nil), http.StatusForbidden)
	expectStatus(t, env.Do(http.MethodDelete, userPath(b, "delete"), env.Token(c, true), nil), http.StatusForbidden)
	expectStatus(t, env.Do(http.MethodDelete, userPath(b, "delete"), "", nil), http.StatusUnauthorized)

	if env.User(b) != bUser || env.data(b) != bData {
		t.Fatal("refused delete changed the target's account or data")
	}
}

func TestDeleteUser_SelfRemovesAllOwnDataAndNothingElse(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	seedTenant(env, a, "alice-pub")
	seedTenant(env, b, "bob-pub")
	// Bob also links into one of Alice's containers? Not possible to own it, but
	// a stray link that involves Alice's registration must go with it.
	shared, _ := tenantGear(env)
	aReg := env.SeedRegistration(a, shared)
	bReg := env.SeedRegistration(b, env.capGear())
	env.SeedContainerLink(bReg, aReg)
	bUser := env.User(b)
	bBefore := env.data(b)
	bBefore.links-- // the stray link disappears with Alice's registration

	w := env.Do(http.MethodDelete, userPath(a, "delete"), env.Token(a, false), nil)
	expectStatus(t, w, http.StatusOK)

	if env.User(a).Exists {
		t.Error("account still exists")
	}
	if d := env.data(a); d != (tenantData{}) {
		t.Errorf("owned data remains: %+v", d)
	}
	if env.User(b) != bUser {
		t.Error("other user's account changed")
	}
	if got := env.data(b); got != bBefore {
		t.Errorf("other user's data changed: %+v want %+v", got, bBefore)
	}
	if n := env.Count("loadout_items", "detachedGearId = ?", shared); n != 1 {
		t.Errorf("other user's item referencing shared gear affected: %d", n)
	}
	// public slug 404s now
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/public/loadout/alice-pub", "", nil), http.StatusNotFound)
	expectStatus(t, env.Do(http.MethodGet, "/api/v1/public/loadout/bob-pub", "", nil), http.StatusOK)
	// a still-valid token of the deleted account cannot be refreshed
	expectStatus(t, env.Do(http.MethodPost, "/auth/refresh", env.Token(a, false), nil), http.StatusUnauthorized)
}

func TestDeleteUser_AllOrNothingOnFailure(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	seedTenant(env, a, "alice-pub")
	seedTenant(env, b, "bob-pub")
	aUser, aData, bData := env.User(a), env.data(a), env.data(b)

	// Fail at the last step, after every dependent delete has run.
	if _, err := env.DB.Exec(`CREATE TRIGGER fail_user_delete BEFORE DELETE ON users BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}

	w := env.Do(http.MethodDelete, userPath(a, "delete"), env.Token(a, false), nil)
	if w.Code < 500 {
		t.Fatalf("failed delete must not look successful or refused: %d %s", w.Code, w.Body.String())
	}
	if env.User(a) != aUser || env.data(a) != aData {
		t.Fatalf("partial deletion: user=%+v data=%+v want data %+v", env.User(a), env.data(a), aData)
	}
	if env.data(b) != bData {
		t.Fatal("other user's data changed")
	}
}

// A username or name that looks like an email address is hidden from other
// non-admin users even when it is not the user's stored email (Google
// sign-in, changed email), and search cannot reveal it.
func TestUserDirectory_AnyEmailLikeNameIsHiddenAndNotSearchable(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	d := env.SeedUser("dave@old-mail.test", "dave@new-mail.test", false)
	if _, err := env.DB.Exec(`UPDATE users SET userName = ? WHERE userId = ?`, "Dave at old-mail@work", d); err != nil {
		t.Fatal(err)
	}
	admin := env.SeedUser("root", "root@example.com", true)
	tok := env.Token(a, false)

	for _, p := range []string{userPath(d, "get"), "/api/v1/users/list"} {
		w := env.Do(http.MethodGet, p, tok, nil)
		expectStatus(t, w, http.StatusOK)
		if strings.Contains(w.Body.String(), "old-mail") {
			t.Errorf("GET %s discloses an email-like value: %s", p, w.Body.String())
		}
	}
	for _, q := range []string{"user=old-mail", "username=old-mail", "user=dave%40old", "username=work"} {
		w := env.Do(http.MethodGet, "/api/v1/users/list?"+q, tok, nil)
		expectStatus(t, w, http.StatusOK)
		var list struct {
			TotalItemCount int `json:"total_item_count"`
		}
		decodeJSON(t, w, &list)
		if list.TotalItemCount != 0 {
			t.Errorf("search %q matched a hidden value: %s", q, w.Body.String())
		}
	}

	// Admins and the user themself still see and can search it.
	for who, tk := range map[string]string{"admin": env.Token(admin, true), "self": env.Token(d, false)} {
		w := env.Do(http.MethodGet, userPath(d, "get"), tk, nil)
		expectStatus(t, w, http.StatusOK)
		if !strings.Contains(w.Body.String(), "dave@old-mail.test") {
			t.Errorf("%s no longer sees the username: %s", who, w.Body.String())
		}
	}
	w := env.Do(http.MethodGet, "/api/v1/users/list?user=old-mail", env.Token(admin, true), nil)
	if !strings.Contains(w.Body.String(), "dave@old-mail.test") {
		t.Errorf("admin search lost the value: %s", w.Body.String())
	}
}

// The search and the response must hide exactly the same values. A stored
// email without '@' and with non-ASCII capitals used to be hidden by the
// response (Go folds case with Unicode rules) but not by the search (SQLite's
// lower() folds ASCII only), so a name could be found by a search while the
// response blanked it. Both sides now hide a value only when it contains '@'.
func TestUserDirectory_SearchAndResponseAgreeOnNonASCIIText(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	d := env.SeedUser("\u00e5sa", "\u00c5SA", false) // username "åsa", stored email "ÅSA": no '@'
	if _, err := env.DB.Exec(`UPDATE users SET userName = ? WHERE userId = ?`, "\u00e5sa", d); err != nil {
		t.Fatal(err)
	}
	tok := env.Token(a, false)

	for _, q := range []string{"username=sa", "user=sa", "username=%C3%A5sa", "user=%C3%A5sa"} {
		w := env.Do(http.MethodGet, "/api/v1/users/list?"+q, tok, nil)
		expectStatus(t, w, http.StatusOK)
		var list struct {
			TotalItemCount int `json:"total_item_count"`
		}
		decodeJSON(t, w, &list)
		shown := strings.Contains(w.Body.String(), "\u00e5sa")
		if matched := list.TotalItemCount > 0; matched != shown {
			t.Errorf("search %q: matched=%v but response shows the value=%v; body: %s", q, matched, shown, w.Body.String())
		}
	}
	// A value with '@' stays hidden on both sides.
	e := env.SeedUser("erik@mail.test", "erik@mail.test", false)
	w := env.Do(http.MethodGet, "/api/v1/users/list?username=erik", tok, nil)
	expectStatus(t, w, http.StatusOK)
	if strings.Contains(w.Body.String(), "erik") {
		t.Errorf("email-like username disclosed (user %d): %s", e, w.Body.String())
	}
}

// An admin-status change is attributable: the log entry names the acting
// admin and the target.
func TestSetUserAdmin_LogNamesActingAdmin(t *testing.T) {
	env := newAuthzEnv(t)
	root := env.SeedUser("root", "root@example.com", true)
	u := env.SeedUser("alice", "alice@example.com", false)
	expectStatus(t, env.Do(http.MethodPost, userPath(u, "admin"), env.Token(root, true), map[string]any{"user_is_admin": true}), http.StatusOK)

	for _, e := range env.Logs.All() {
		f := e.ContextMap()
		if e.Message == "admin status changed" {
			if f["actor_user_id"] != root || f["target_user_id"] != u || f["user_is_admin"] != true {
				t.Fatalf("log fields = %v, want actor %d target %d", f, root, u)
			}
			return
		}
	}
	t.Fatalf("no admin-change log entry: %v", env.Logs.All())
}

// ---- BR-004 AC-9: admin-only clearing of the edited-email mark ---------------

func (e *authzEnv) emailEdited(id int64) bool {
	e.T.Helper()
	var v bool
	if err := e.DB.QueryRow(`SELECT userEmailEdited FROM users WHERE userId = ?`, id).Scan(&v); err != nil {
		e.T.Fatalf("read userEmailEdited of %d: %v", id, err)
	}
	return v
}

func TestClearEmailEdited_AdminOnlyAndChangesNothingElse(t *testing.T) {
	env := newAuthzEnv(t)
	admin := env.SeedUser("root", "root@example.com", true)
	a := env.SeedUser("alice", "alice@example.com", false)
	env.bind(a, "sub-alice")
	if _, err := env.DB.Exec(`UPDATE users SET userEmailEdited = 1 WHERE userId = ?`, a); err != nil {
		t.Fatal(err)
	}
	before := env.User(a)
	path := userPath(a, "email-edited/clear")

	// Refused: no token, a non-admin (the account's own owner included), an
	// admin-audience-less token minted for the admin's id.
	expectStatus(t, env.Do(http.MethodPost, path, "", nil), http.StatusUnauthorized)
	expectStatus(t, env.Do(http.MethodPost, path, env.Token(a, false), nil), http.StatusForbidden)
	expectStatus(t, env.Do(http.MethodPost, path, env.Token(admin, false), nil), http.StatusForbidden)
	if !env.emailEdited(a) {
		t.Fatal("a refused request cleared the mark")
	}

	expectStatus(t, env.Do(http.MethodPost, path, env.Token(admin, true), nil), http.StatusOK)
	if env.emailEdited(a) {
		t.Fatal("the admin's request did not clear the mark")
	}
	if got := env.User(a); got != before {
		t.Fatalf("clearing changed the account: %+v -> %+v", before, got)
	}
	if s := env.subjectOf(a); !s.Valid || s.String != "sub-alice" {
		t.Fatalf("clearing changed the Google subject: %+v", s)
	}

	// Clearing an account without the mark succeeds and changes nothing.
	b := env.SeedUser("bob", "bob@example.com", false)
	bBefore := env.User(b)
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "email-edited/clear"), env.Token(admin, true), nil), http.StatusOK)
	if env.User(b) != bBefore || env.emailEdited(b) {
		t.Fatal("clearing an unmarked account changed it")
	}

	// A later email change marks again.
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), env.Token(a, false), map[string]any{"user_email": "alice-new@example.com"}), http.StatusOK)
	if !env.emailEdited(a) {
		t.Fatal("email change after clearing did not mark the account again")
	}

	// Errors: unknown user 404, malformed id 400.
	expectStatus(t, env.Do(http.MethodPost, userPath(9999, "email-edited/clear"), env.Token(admin, true), nil), http.StatusNotFound)
	expectStatus(t, env.Do(http.MethodPost, "/api/v1/users/xyz/email-edited/clear", env.Token(admin, true), nil), http.StatusBadRequest)
}

// The point of AC-9: after clearing, an unbound account is eligible for the
// backfill again (BR-004 AC-8); before, the sign-in is refused (403).
func TestClearEmailEdited_RestoresBackfillEligibility(t *testing.T) {
	env := newAuthzEnv(t)
	admin := env.SeedUser("root", "root@example.com", true)
	a := env.SeedUser("alice", "old@example.com", false)
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), env.Token(a, false), map[string]any{"user_email": "alice@gmail.example"}), http.StatusOK)

	if res := env.signIn(verified("sub-A", "alice@gmail.example")); res.Code != http.StatusForbidden {
		t.Fatalf("marked account: sign-in = %d, want 403", res.Code)
	}
	if env.subjectOf(a).Valid {
		t.Fatal("marked account was bound")
	}
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "email-edited/clear"), env.Token(admin, true), nil), http.StatusOK)
	if res := env.signIn(verified("sub-A", "alice@gmail.example")); res.Code != http.StatusOK || res.UserID != a {
		t.Fatalf("after clearing: %+v, want sign-in as %d", res, a)
	}
	if s := env.subjectOf(a); !s.Valid || s.String != "sub-A" {
		t.Fatalf("subject = %+v", s)
	}
}

// The acting admin and the target are logged.
func TestClearEmailEdited_LogNamesActingAdminAndTarget(t *testing.T) {
	env := newAuthzEnv(t)
	root := env.SeedUser("root", "root@example.com", true)
	u := env.SeedUser("alice", "alice@example.com", false)
	expectStatus(t, env.Do(http.MethodPost, userPath(u, "email-edited/clear"), env.Token(root, true), nil), http.StatusOK)
	for _, e := range env.Logs.All() {
		if e.Message == "edited-email mark cleared" {
			f := e.ContextMap()
			if f["actor_user_id"] != root || f["target_user_id"] != u {
				t.Fatalf("log fields = %v, want actor %d target %d", f, root, u)
			}
			return
		}
	}
	t.Fatalf("no log entry: %v", env.Logs.All())
}

// ---- BR-008 AC-4..AC-6: what a profile update may and may not change ----------

// Whatever the body carries, a profile update never changes the Google subject,
// admin status, user ID, password or the edited-email mark, except that a real
// email change sets the mark.
func TestUpdateUser_NeverTouchesSubjectIDAdminOrMarkExceptViaEmail(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", true)
	env.bind(a, "sub-alice")
	tok := env.Token(a, true)

	hostile := map[string]any{
		"user_id": a, "user_is_admin": true,
		"googleSubject": "sub-attacker", "google_subject": "sub-attacker", "GoogleSubject": "sub-attacker",
		"userEmailEdited": false, "user_email_edited": false, "email_edited": false,
		"userId": a + 100, "userIsAdmin": false, "userPassword": "x",
		"user_username": "alice2", "user_name": "Alice Two",
	}
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, hostile), http.StatusOK)
	got := env.User(a)
	if got.Username != "alice2" || got.Name != "Alice Two" || got.Email != "alice@example.com" {
		t.Fatalf("profile fields not applied or email disturbed: %+v", got)
	}
	if got.Password != "hash-of-alice" || !got.IsAdmin {
		t.Fatalf("password or admin status changed: %+v", got)
	}
	if s := env.subjectOf(a); !s.Valid || s.String != "sub-alice" {
		t.Fatalf("Google subject changed: %+v", s)
	}
	if env.emailEdited(a) {
		t.Fatal("an update that left the email alone set the mark")
	}
	if env.Count("users", "userId = ?", a+100) != 0 || env.Count("users", "") != 1 {
		t.Fatal("user ID changed or a user appeared")
	}

	// Changing the email sets the mark and still changes nothing else.
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, map[string]any{"user_email": "other@example.com", "googleSubject": "sub-attacker"}), http.StatusOK)
	if got := env.User(a); got.Email != "other@example.com" || !got.IsAdmin {
		t.Fatalf("after email change: %+v", got)
	}
	if !env.emailEdited(a) {
		t.Fatal("email change did not set the mark")
	}
	if s := env.subjectOf(a); s.String != "sub-alice" {
		t.Fatalf("Google subject changed by email update: %+v", s)
	}

	// The user cannot clear the mark: not by body fields, not by restoring the old email.
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, map[string]any{"user_email": "alice@example.com", "userEmailEdited": false, "user_email_edited": false}), http.StatusOK)
	if !env.emailEdited(a) {
		t.Fatal("the user cleared the edited-email mark through a profile update")
	}
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), tok, map[string]any{"user_name": "Same Email Resent", "user_email": "alice@example.com"}), http.StatusOK)
	if !env.emailEdited(a) {
		t.Fatal("re-sending the email cleared the mark")
	}
}

// A user without a bound subject stays unbound after any update.
func TestUpdateUser_UnboundAccountStaysUnbound(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	expectStatus(t, env.Do(http.MethodPost, userPath(a, "update"), env.Token(a, false), map[string]any{"googleSubject": "sub-x", "user_name": "A"}), http.StatusOK)
	if env.subjectOf(a).Valid {
		t.Fatal("a profile update bound a Google subject")
	}
}
