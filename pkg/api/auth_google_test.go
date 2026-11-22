package endpoints

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Google sign-in (BR-003 to BR-005) through the real handler and a migrated
// database. Only Google's token validation is replaced: the test chooses the
// claims the "validated" token carries.

const googleBody = `{"id_token":"opaque"}`

type googleResult struct {
	Code   int
	UserID int64
	Admin  bool
	Email  string
}

// signIn presents a Google token with the given claims and decodes the answer.
func (e *authzEnv) signIn(id *GoogleIdentity) googleResult {
	e.T.Helper()
	e.google = id
	w := e.Do(http.MethodPost, "/auth/google/callback", "", googleBody)
	res := googleResult{Code: w.Code}
	if w.Code == http.StatusOK {
		var out struct {
			AccessToken string `json:"access_token"`
			User        struct {
				ID    int64  `json:"id"`
				Email string `json:"email"`
				Admin bool   `json:"is_admin"`
			} `json:"user"`
		}
		decodeJSON(e.T, w, &out)
		if out.AccessToken == "" {
			e.T.Fatalf("200 without an access token: %s", w.Body.String())
		}
		res.UserID, res.Admin, res.Email = out.User.ID, out.User.Admin, out.User.Email
	} else if body := w.Body.String(); strings.Contains(body, "access_token") {
		e.T.Fatalf("refused sign-in (%d) carries a token: %s", w.Code, body)
	}
	return res
}

func (e *authzEnv) subjectOf(id int64) sql.NullString {
	e.T.Helper()
	var s sql.NullString
	if err := e.DB.QueryRow(`SELECT googleSubject FROM users WHERE userId = ?`, id).Scan(&s); err != nil {
		e.T.Fatalf("read subject of %d: %v", id, err)
	}
	return s
}

func (e *authzEnv) bind(id int64, sub string) {
	e.T.Helper()
	if _, err := e.DB.Exec(`UPDATE users SET googleSubject = ? WHERE userId = ?`, sub, id); err != nil {
		e.T.Fatalf("bind: %v", err)
	}
}

func verified(sub, email string) *GoogleIdentity {
	return &GoogleIdentity{Subject: sub, Email: email, EmailVerified: true, Name: "G User"}
}

func TestGoogleSignIn_BoundSubjectSignsIntoItsAccount(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	env.bind(a, "sub-A")
	env.bind(b, "sub-B")

	// BR-003 AC-1: the token's email differs from the stored one, and is not
	// verified: the subject alone decides.
	res := env.signIn(&GoogleIdentity{Subject: "sub-A", Email: "alice.new@gmail.com", EmailVerified: false})
	if res.Code != http.StatusOK || res.UserID != a {
		t.Fatalf("got %+v, want account %d", res, a)
	}
	// Even a token that carries Bob's email reaches Alice by subject.
	res = env.signIn(verified("sub-A", "bob@example.com"))
	if res.Code != http.StatusOK || res.UserID != a {
		t.Fatalf("got %+v, want account %d", res, a)
	}
	if got := env.User(a).Email; got != "alice@example.com" {
		t.Fatalf("sign-in rewrote the stored email to %q", got)
	}
}

func TestGoogleSignIn_BoundAccountIsNeverReachedByEmail(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", true)
	env.bind(a, "sub-A")
	before := env.Count("users", "")

	// BR-003 AC-2 and BR-005 AC-2: another subject with A's verified email.
	res := env.signIn(verified("sub-T", "alice@example.com"))
	if res.Code != http.StatusForbidden {
		t.Fatalf("got %+v, want 403", res)
	}
	if env.Count("users", "") != before {
		t.Fatal("a refused sign-in created an account")
	}
	if got := env.subjectOf(a); got.String != "sub-A" {
		t.Fatalf("binding of A changed to %q", got.String)
	}
	if env.Count("users", "googleSubject = 'sub-T'") != 0 {
		t.Fatal("subject T was bound")
	}
}

func TestGoogleSignIn_TokenWithoutSubjectIsRefused(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	before := env.Count("users", "")

	for _, sub := range []string{"", "   "} {
		res := env.signIn(&GoogleIdentity{Subject: sub, Email: "alice@example.com", EmailVerified: true})
		if res.Code == http.StatusOK {
			t.Fatalf("sub %q: sign-in succeeded as %d", sub, res.UserID)
		}
	}
	if env.Count("users", "") != before || env.subjectOf(a).Valid {
		t.Fatal("a subject-less token linked or created an account")
	}
	// An invalid token is refused too.
	env.google = nil
	if w := env.Do(http.MethodPost, "/auth/google/callback", "", googleBody); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: status %d", w.Code)
	}
}

func TestGoogleSignIn_BackfillBindsOnce(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", true) // legacy, unbound admin
	other := env.SeedUser("bob", "bob@example.com", false)

	res := env.signIn(verified("sub-A", "alice@example.com"))
	if res.Code != http.StatusOK || res.UserID != a {
		t.Fatalf("backfill: got %+v, want account %d", res, a)
	}
	if got := env.subjectOf(a); got.String != "sub-A" {
		t.Fatalf("stored subject = %+v", got)
	}
	if env.subjectOf(other).Valid {
		t.Fatal("another account was bound")
	}

	// Later sign-ins go by subject: a changed Google email still works.
	res = env.signIn(verified("sub-A", "alice.moved@gmail.com"))
	if res.Code != http.StatusOK || res.UserID != a {
		t.Fatalf("by subject: got %+v", res)
	}

	// Once bound it is not eligible again: a second subject with the same
	// verified email is refused and the binding stays.
	res = env.signIn(verified("sub-X", "alice@example.com"))
	if res.Code != http.StatusForbidden {
		t.Fatalf("second subject: got %+v, want 403", res)
	}
	if got := env.subjectOf(a); got.String != "sub-A" {
		t.Fatalf("binding replaced by %q", got.String)
	}
}

func TestGoogleSignIn_AdminStatusComesFromStoredAccount(t *testing.T) {
	env := newAuthzEnv(t)
	admin := env.SeedUser("root", "root@example.com", true)
	user := env.SeedUser("pleb", "pleb@example.com", false)
	env.bind(admin, "sub-root")
	env.bind(user, "sub-pleb")

	// Backfilled and bound sign-ins alike: claims cannot add or remove admin.
	id := verified("sub-pleb", "root@example.com")
	id.Name = "admin"
	if res := env.signIn(id); res.UserID != user || res.Admin {
		t.Fatalf("pleb: got %+v", res)
	}
	if res := env.signIn(verified("sub-root", "pleb@example.com")); res.UserID != admin || !res.Admin {
		t.Fatalf("root: got %+v", res)
	}
	// Demotion is respected at the next sign-in.
	if _, err := env.DB.Exec(`UPDATE users SET userIsAdmin = 0 WHERE userId = ?`, admin); err != nil {
		t.Fatal(err)
	}
	if res := env.signIn(verified("sub-root", "root@example.com")); res.Admin {
		t.Fatalf("demoted admin signed in as admin: %+v", res)
	}
}

func TestGoogleSignIn_UnverifiedEmailRefused(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	before := env.Count("users", "")

	for _, id := range []*GoogleIdentity{
		{Subject: "sub-N", Email: "alice@example.com", EmailVerified: false}, // would backfill
		{Subject: "sub-N", Email: "new@example.com", EmailVerified: false},   // would create
	} {
		if res := env.signIn(id); res.Code != http.StatusForbidden {
			t.Fatalf("%+v: got %+v, want 403", id, res)
		}
	}
	if env.Count("users", "") != before || env.subjectOf(a).Valid {
		t.Fatal("an unverified sign-in linked or created an account")
	}
}

func TestGoogleSignIn_FirstSignInCreatesNonAdminAccount(t *testing.T) {
	env := newAuthzEnv(t)

	res := env.signIn(verified("sub-new", "new@example.com"))
	if res.Code != http.StatusOK || res.Admin || res.Email != "new@example.com" {
		t.Fatalf("got %+v", res)
	}
	if got := env.subjectOf(res.UserID); got.String != "sub-new" {
		t.Fatalf("created account subject = %+v", got)
	}
	if env.User(res.UserID).IsAdmin {
		t.Fatal("created account is admin")
	}
	if again := env.signIn(verified("sub-new", "other@example.com")); again.UserID != res.UserID {
		t.Fatalf("second sign-in reached %d, want %d", again.UserID, res.UserID)
	}
	if env.Count("users", "") != 1 {
		t.Fatalf("accounts = %d, want 1", env.Count("users", ""))
	}
}

func TestGoogleSignIn_DuplicateUnboundEmailsRefused(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "shared@example.com", false)
	b := env.SeedUser("alice2", "shared@example.com", false)
	before := env.Count("users", "")

	if res := env.signIn(verified("sub-S", "shared@example.com")); res.Code != http.StatusForbidden {
		t.Fatalf("got %+v, want 403", res)
	}
	if env.subjectOf(a).Valid || env.subjectOf(b).Valid || env.Count("users", "") != before {
		t.Fatal("a duplicate-email sign-in linked or created an account")
	}
}

func TestGoogleSignIn_EmailMatchIsExactAfterTrimming(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "Alice@Example.com", false)
	before := env.Count("users", "")

	// P-7: the match is case-sensitive, so a case-different email never binds
	// the legacy account; no account uses that exact email, so a new one is created.
	if res := env.signIn(verified("sub-1", "alice@example.com")); res.Code != http.StatusOK || res.UserID == a {
		t.Fatalf("case-different email reached the legacy account: %+v", res)
	}
	if env.subjectOf(a).Valid || env.Count("users", "") != before+1 {
		t.Fatal("case-different email bound the legacy account")
	}

	// Surrounding whitespace is ignored on both sides.
	b := env.SeedUser("bob", "  bob@example.com ", false)
	if res := env.signIn(verified("sub-2", " bob@example.com\t")); res.Code != http.StatusOK || res.UserID != b {
		t.Fatalf("trimmed email: got %+v, want %d", res, b)
	}
}

// A user can change their own profile email to anyone's (BR-008). An account
// whose email was changed must never be bound by that email, otherwise the
// owner of the planted email would sign into the planter's account, or the
// planter would capture the owner's first sign-in.
func TestGoogleSignIn_PlantedEmailCannotCaptureSignIn(t *testing.T) {
	env := newAuthzEnv(t)
	mallory := env.SeedUser("mallory", "mallory@example.com", false)
	tok := env.Token(mallory, false)

	w := env.Do(http.MethodPost, "/api/v1/users/"+itoa(mallory)+"/update", tok, `{"user_email":"victim@example.com"}`)
	expectStatus(t, w, http.StatusOK)
	if got := env.User(mallory).Email; got != "victim@example.com" {
		t.Fatalf("plant failed, email = %q", got)
	}
	before := env.Count("users", "")

	// The victim has no account yet and signs in for the first time.
	res := env.signIn(verified("sub-victim", "victim@example.com"))
	if res.Code != http.StatusForbidden {
		t.Fatalf("got %+v, want 403", res)
	}
	if env.subjectOf(mallory).Valid || env.Count("users", "") != before {
		t.Fatal("planted email was linked or an account was created")
	}
}

func TestGoogleSignIn_PlantedEmailDoesNotBlockTheRealAccount(t *testing.T) {
	env := newAuthzEnv(t)
	victim := env.SeedUser("victim", "victim@example.com", false) // legacy, unbound
	mallory := env.SeedUser("mallory", "mallory@example.com", false)

	expectStatus(t, env.Do(http.MethodPost, "/api/v1/users/"+itoa(mallory)+"/update", env.Token(mallory, false),
		`{"user_email":"victim@example.com"}`), http.StatusOK)

	res := env.signIn(verified("sub-victim", "victim@example.com"))
	if res.Code != http.StatusOK || res.UserID != victim {
		t.Fatalf("got %+v, want the victim's own account %d", res, victim)
	}
	if env.subjectOf(mallory).Valid {
		t.Fatal("the planted account was bound")
	}
}

func TestGoogleSignIn_ProfileUpdateWithSameEmailKeepsBackfillEligibility(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)

	expectStatus(t, env.Do(http.MethodPost, "/api/v1/users/"+itoa(a)+"/update", env.Token(a, false),
		`{"user_name":"Alice Renamed","user_email":"alice@example.com"}`), http.StatusOK)

	if res := env.signIn(verified("sub-A", "alice@example.com")); res.Code != http.StatusOK || res.UserID != a {
		t.Fatalf("got %+v, want %d", res, a)
	}
}

func itoa(n int64) string { return fmt.Sprint(n) }

// BR-004 AC-7: two subjects racing for one eligible account bind at most one.
func TestGoogleSignIn_RacingSubjectsBindAtMostOne(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)

	const n = 6
	// Distinct identities straight into the sign-in logic (the HTTP harness
	// shares one injected identity).
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		id := verified(fmt.Sprintf("race-%d", i), "alice@example.com")
		go func() {
			_, err := signInWithGoogle(context.Background(), env.DB, id)
			done <- err
		}()
	}
	ok := 0
	for i := 0; i < n; i++ {
		if err := <-done; err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d racing sign-ins succeeded, want 1", ok)
	}
	if env.Count("users", "googleSubject IS NOT NULL") != 1 || !env.subjectOf(a).Valid {
		t.Fatal("expected exactly the one account bound")
	}
}

// BR-004 AC-4: whitespace is stripped identically on both sides, so a stored
// email with a tab or newline still binds instead of creating a duplicate.
func TestGoogleSignIn_StoredEmailWithAnyWhitespaceStillMatches(t *testing.T) {
	for name, stored := range map[string]string{"tab": "v@example.com\t", "newline": "\nv@example.com", "nbsp": "v@example.com "} {
		t.Run(name, func(t *testing.T) {
			env := newAuthzEnv(t)
			a := env.SeedUser("vera", stored, false)
			before := env.Count("users", "")
			if res := env.signIn(verified("sub-V", "v@example.com")); res.Code != http.StatusOK || res.UserID != a {
				t.Fatalf("got %+v, want the stored account %d", res, a)
			}
			if env.Count("users", "") != before {
				t.Fatal("a duplicate account was created")
			}
		})
	}
}

// Emails written through the API are stored trimmed.
func TestUserWrites_EmailStoredTrimmed(t *testing.T) {
	env := newAuthzEnv(t)
	admin := env.SeedUser("root", "root@example.com", true)
	tok := env.Token(admin, true)
	expectStatus(t, env.Do(http.MethodPost, userPath(admin, "update"), tok, map[string]any{"user_email": "\troot2@example.com\n"}), http.StatusOK)
	if got := env.User(admin).Email; got != "root2@example.com" {
		t.Fatalf("updated email stored as %q", got)
	}
	var edited bool
	if err := env.DB.QueryRow(`SELECT userEmailEdited FROM users WHERE userId = ?`, admin).Scan(&edited); err != nil || !edited {
		t.Fatalf("real change must set the edited flag: %v %v", edited, err)
	}
	// Re-sending the same email with whitespace is not a change.
	b := env.SeedUser("bob", "bob@example.com", false)
	expectStatus(t, env.Do(http.MethodPost, userPath(b, "update"), env.Token(b, false), map[string]any{"user_email": "bob@example.com\t"}), http.StatusOK)
	if err := env.DB.QueryRow(`SELECT userEmailEdited FROM users WHERE userId = ?`, b).Scan(&edited); err != nil || edited {
		t.Fatalf("whitespace-only resubmission set the edited flag: %v %v", edited, err)
	}
}

func TestValidateGoogleIDToken_RefusesEmptyAudience(t *testing.T) {
	for _, aud := range []string{"", "  "} {
		if _, err := validateGoogleIDToken(context.Background(), "x.y.z", aud); err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("audience %q: err = %v", aud, err)
		}
	}
}
