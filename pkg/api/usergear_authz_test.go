package endpoints

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Adversarial regression tests for BR-012, BR-013, BR-017, BR-018 and OC-001 as
// they apply to gear registrations (inventory) and container links. They run
// through the real JWT middleware and the real route table (see
// authz_harness_test.go) against a migrated database.
//
// Container nesting, cycles, duplicates and what may hold gear are open
// question Q-4 and deliberately have no tests here beyond the self-link
// refusal.

func ugList(user int64, query string) string {
	p := fmt.Sprintf("/api/v1/usergear/%d/list", user)
	if query != "" {
		p += "?" + query
	}
	return p
}
func ugPath(reg int64, op string) string {
	return fmt.Sprintf("/api/v1/usergear/registration/%d/%s", reg, op)
}
func ctList(container int64, query string) string {
	p := fmt.Sprintf("/api/v1/container/%d/list", container)
	if query != "" {
		p += "?" + query
	}
	return p
}
func ctDelete(link int64) string { return fmt.Sprintf("/api/v1/container/%d/delete", link) }

// seedGearX inserts catalog gear. nullDims leaves height, length, width and
// status NULL (the schema allows it).
func (e *authzEnv) seedGearX(isContainer, nullDims bool, categoryID, topCategoryID int64) int64 {
	e.T.Helper()
	c := 0
	if isContainer {
		c = 1
	}
	var res interface {
		LastInsertId() (int64, error)
	}
	var err error
	if nullDims {
		res, err = e.DB.Exec(`INSERT INTO gear (gearTopCategoryId, gearCategoryId, gearManufactureId, gearIsContainer, gearName, gearWeight) VALUES (?, ?, 1, ?, 'nulls', 100)`, topCategoryID, categoryID, c)
	} else {
		res, err = e.DB.Exec(`INSERT INTO gear (gearTopCategoryId, gearCategoryId, gearManufactureId, gearIsContainer, gearName, gearWeight, gearHeight, gearLength, gearWidth, gearStatus) VALUES (?, ?, 1, ?, 'full', 100, 1, 2, 3, 1)`, topCategoryID, categoryID, c)
	}
	if err != nil {
		e.T.Fatalf("seed gear: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (e *authzEnv) setMaxWeight(reg int64, w int) {
	e.T.Helper()
	if _, err := e.DB.Exec(`UPDATE user_gear_registrations SET maxContainerWeight = ? WHERE userGearRegistrationId = ?`, w, reg); err != nil {
		e.T.Fatal(err)
	}
}

// ugSnapshot is the complete stored state of registrations and container links.
func (e *authzEnv) ugSnapshot() string {
	e.T.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT quote(userGearRegistrationId)||'|'||quote(gearId)||'|'||quote(userId)||'|'||quote(maxContainerWeight) FROM user_gear_registrations ORDER BY userGearRegistrationId`,
		`SELECT quote(containerRegistrationId)||'|'||quote(userContainerId)||'|'||quote(userGearRegistrationId) FROM user_container_registration ORDER BY containerRegistrationId`,
	} {
		rows, err := e.DB.Query(q)
		if err != nil {
			e.T.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				e.T.Fatal(err)
			}
			sb.WriteString(s + "\n")
		}
		rows.Close()
		sb.WriteString("--\n")
	}
	return sb.String()
}

func (e *authzEnv) linkID(container, content int64) int64 {
	e.T.Helper()
	var id int64
	if err := e.DB.QueryRow(`SELECT containerRegistrationId FROM user_container_registration WHERE userContainerId = ? AND userGearRegistrationId = ?`, container, content).Scan(&id); err != nil {
		e.T.Fatalf("link %d->%d: %v", container, content, err)
	}
	return id
}

// page decodes a ResponsePayload and returns its items and total count.
func page(t *testing.T, w *httptest.ResponseRecorder) ([]map[string]any, int) {
	t.Helper()
	var p struct {
		Total int              `json:"total_item_count"`
		Items []map[string]any `json:"items"`
	}
	decodeJSON(t, w, &p)
	if p.Items == nil {
		t.Fatalf("items is null, want []; body: %s", w.Body.String())
	}
	return p.Items, p.Total
}

func regIDs(items []map[string]any) []int64 {
	var ids []int64
	for _, it := range items {
		ids = append(ids, int64(it["usergear_registration_id"].(float64)))
	}
	return ids
}

func sameIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type ugWorld struct {
	env                *authzEnv
	alice, bob, carol  int64 // carol is an admin
	tokA, tokB, tokC   string
	gear               int64
	aReg1, aReg2, bReg int64
}

func newUGWorld(t *testing.T) *ugWorld {
	env := newAuthzEnv(t)
	w := &ugWorld{env: env}
	w.alice = env.SeedUser("alice", "alice@example.com", false)
	w.bob = env.SeedUser("bob", "bob@example.com", false)
	w.carol = env.SeedUser("carol", "carol@example.com", true)
	w.tokA = env.Token(w.alice, false)
	w.tokB = env.Token(w.bob, false)
	w.tokC = env.Token(w.carol, true)
	w.gear = env.seedGearX(false, false, 1, 1)
	w.aReg1 = env.SeedRegistration(w.alice, w.gear)
	w.aReg2 = env.SeedRegistration(w.alice, w.gear)
	w.bReg = env.SeedRegistration(w.bob, w.gear)
	env.setMaxWeight(w.bReg, 777)
	return w
}

// --- BR-012 listing ---------------------------------------------------------

func TestUserGearList_OwnerOnly_NoAdminException(t *testing.T) {
	w := newUGWorld(t)
	e := w.env

	// Own list: only own registrations.
	r := e.Do("GET", ugList(w.alice, ""), w.tokA, nil)
	expectStatus(t, r, 200)
	items, total := page(t, r)
	if !sameIDs(regIDs(items), []int64{w.aReg1, w.aReg2}) || total != 2 {
		t.Fatalf("alice list = %v total %d", regIDs(items), total)
	}

	before := e.ugSnapshot()
	// Another user's list is 403 for users and admins alike (P-3).
	for name, tok := range map[string]string{"user": w.tokA, "admin": w.tokC} {
		r := e.Do("GET", ugList(w.bob, ""), tok, nil)
		if r.Code != 403 {
			t.Fatalf("%s listing bob: status %d, want 403; body: %s", name, r.Code, r.Body.String())
		}
		if strings.Contains(r.Body.String(), "777") {
			t.Fatalf("%s: response leaks bob's data: %s", name, r.Body.String())
		}
	}
	// Admin listing themself is allowed (and empty).
	r = e.Do("GET", ugList(w.carol, ""), w.tokC, nil)
	expectStatus(t, r, 200)
	if items, total := page(t, r); len(items) != 0 || total != 0 {
		t.Fatalf("carol list = %v total %d", items, total)
	}

	expectStatus(t, e.Do("GET", ugList(w.alice, ""), "", nil), 401)
	expectStatus(t, e.Do("GET", "/api/v1/usergear/abc/list", w.tokA, nil), 400)
	if e.ugSnapshot() != before {
		t.Fatal("list changed stored data")
	}
}

func TestUserGearList_FiltersAreBoundNeverWidening(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	e.SeedCategory("c1") // id 1 is the category of the default gear
	cat := e.SeedCategory("c2")
	other := e.seedGearX(true, false, cat, 1)
	aBox := e.SeedRegistration(w.alice, other)
	e.SeedRegistration(w.bob, other)

	for _, q := range []string{
		"category=" + url.QueryEscape("0 OR 1=1"),
		"topCategory=" + url.QueryEscape("1) OR (1=1"),
		"manufacture=" + url.QueryEscape("1 OR 1=1"),
		"category=" + url.QueryEscape("0 UNION SELECT 1"),
		"page=" + url.QueryEscape("1 OR 1=1"),
		"limit=" + url.QueryEscape("30;DROP TABLE users"),
	} {
		r := e.Do("GET", ugList(w.alice, q), w.tokA, nil)
		if r.Code >= 500 {
			t.Fatalf("%q: status %d: %s", q, r.Code, r.Body.String())
		}
		if r.Code == 200 {
			items, _ := page(t, r)
			for _, id := range regIDs(items) {
				if id == w.bReg || id > aBox {
					t.Fatalf("%q returned a foreign registration %d", q, id)
				}
			}
		}
	}

	// Numeric filters still narrow, owner-scoped.
	r := e.Do("GET", ugList(w.alice, fmt.Sprintf("category=%d", cat)), w.tokA, nil)
	expectStatus(t, r, 200)
	items, total := page(t, r)
	if !sameIDs(regIDs(items), []int64{aBox}) || total != 1 {
		t.Fatalf("category filter = %v total %d", regIDs(items), total)
	}
	r = e.Do("GET", ugList(w.alice, "container=true"), w.tokA, nil)
	expectStatus(t, r, 200)
	if items, _ := page(t, r); !sameIDs(regIDs(items), []int64{aBox}) {
		t.Fatalf("container=true = %v", regIDs(items))
	}
}

func TestUserGearList_RegistrationInSeveralContainersIsNotDuplicated(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	boxGear := e.seedGearX(true, false, 1, 1)
	box1 := e.SeedRegistration(w.alice, boxGear)
	box2 := e.SeedRegistration(w.alice, boxGear)
	e.SeedContainerLink(box1, w.aReg1)
	e.SeedContainerLink(box2, w.aReg1)

	r := e.Do("GET", ugList(w.alice, "limit=100"), w.tokA, nil)
	expectStatus(t, r, 200)
	items, total := page(t, r)
	if !sameIDs(regIDs(items), []int64{w.aReg1, w.aReg2, box1, box2}) || total != 4 {
		t.Fatalf("list = %v total %d, want each registration once", regIDs(items), total)
	}
}

func TestUserGearList_ForeignContainerLinkNotDisclosed(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	// Legacy bad link: alice's registration sits in bob's container.
	bBox := e.SeedRegistration(w.bob, w.gear)
	e.SeedContainerLink(bBox, w.aReg1)

	r := e.Do("GET", ugList(w.alice, ""), w.tokA, nil)
	expectStatus(t, r, 200)
	if strings.Contains(r.Body.String(), fmt.Sprintf(`"container_registration_id": %d`, bBox)) {
		t.Fatalf("response discloses bob's container id: %s", r.Body.String())
	}
}

func TestUserGearList_PagesAreDeterministic(t *testing.T) {
	w := newUGWorld(t)
	r := w.env.Do("GET", ugList(w.alice, "limit=1&page=2"), w.tokA, nil)
	expectStatus(t, r, 200)
	items, total := page(t, r)
	if !sameIDs(regIDs(items), []int64{w.aReg2}) || total != 2 {
		t.Fatalf("page 2 = %v total %d", regIDs(items), total)
	}
}

// --- BR-012 get -------------------------------------------------------------

func TestUserGearGet_OwnerOnly_IdenticalNotFound(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	before := e.ugSnapshot()

	r := e.Do("GET", ugPath(w.aReg1, "get"), w.tokA, nil)
	expectStatus(t, r, 200)
	if !strings.Contains(r.Body.String(), `"gear_name": "full"`) {
		t.Fatalf("own get body: %s", r.Body.String())
	}

	foreign := e.Do("GET", ugPath(w.bReg, "get"), w.tokA, nil)
	unknown := e.Do("GET", ugPath(99999, "get"), w.tokA, nil)
	expectStatus(t, foreign, 404)
	expectStatus(t, unknown, 404)
	if foreign.Body.String() != unknown.Body.String() {
		t.Fatalf("foreign and unknown differ:\n%s\n%s", foreign.Body.String(), unknown.Body.String())
	}
	if strings.Contains(foreign.Body.String(), "777") || strings.Contains(foreign.Body.String(), "bob") {
		t.Fatalf("foreign get leaks data: %s", foreign.Body.String())
	}
	// No admin exception.
	expectStatus(t, e.Do("GET", ugPath(w.aReg1, "get"), w.tokC, nil), 404)
	expectStatus(t, e.Do("GET", ugPath(w.aReg1, "get"), "", nil), 401)
	expectStatus(t, e.Do("GET", "/api/v1/usergear/registration/abc/get", w.tokA, nil), 400)
	if e.ugSnapshot() != before {
		t.Fatal("get changed stored data")
	}
}

func TestUserGearGet_InjectedFiltersCannotWiden(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	for _, q := range []string{
		"category=" + url.QueryEscape("0 OR 1=1"),
		"topCategory=" + url.QueryEscape("1 OR 1=1"),
		"category=abc",
		"category=1&topCategory=1",
	} {
		// Foreign registration stays 404 whatever the filter says.
		r := e.Do("GET", ugPath(w.bReg, "get")+"?"+q, w.tokA, nil)
		if r.Code != 404 {
			t.Fatalf("foreign get with %q: status %d: %s", q, r.Code, r.Body.String())
		}
		// Own registration never errors.
		r = e.Do("GET", ugPath(w.aReg1, "get")+"?"+q, w.tokA, nil)
		if r.Code != 200 {
			t.Fatalf("own get with %q: status %d: %s", q, r.Code, r.Body.String())
		}
	}
}

func TestUserGearGet_NullGearColumns(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	g := e.seedGearX(false, true, 1, 1)
	reg := e.SeedRegistration(w.alice, g)
	expectStatus(t, e.Do("GET", ugPath(reg, "get"), w.tokA, nil), 200)
	r := e.Do("GET", ugList(w.alice, ""), w.tokA, nil)
	expectStatus(t, r, 200)
	if items, _ := page(t, r); len(items) != 3 {
		t.Fatalf("list with NULL gear columns = %v", regIDs(items))
	}
}

// --- BR-012 / BR-017 insert -------------------------------------------------

func TestUserGearInsert_OwnerIsAlwaysCaller(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	base := e.Count("user_gear_registrations", "")

	// Body names another owner: refused, nothing created (also for admins).
	for name, tok := range map[string]string{"user": w.tokA, "admin": w.tokC} {
		before := e.ugSnapshot()
		r := e.Do("PUT", "/api/v1/usergear/insert", tok, map[string]any{"usergear_gear_id": w.gear, "usergear_user_id": w.bob})
		if r.Code != 400 {
			t.Fatalf("%s naming bob: status %d, want 400; body: %s", name, r.Code, r.Body.String())
		}
		if e.ugSnapshot() != before {
			t.Fatalf("%s naming bob changed stored data", name)
		}
	}
	if e.Count("user_gear_registrations", "userId = ? AND userGearRegistrationId > ?", w.bob, w.bReg) != 0 {
		t.Fatal("a registration owned by bob was created")
	}

	// Owner omitted and owner matching are the same.
	expectStatus(t, e.Do("PUT", "/api/v1/usergear/insert", w.tokA, map[string]any{"usergear_gear_id": w.gear}), 200)
	expectStatus(t, e.Do("PUT", "/api/v1/usergear/insert", w.tokA, map[string]any{"usergear_gear_id": w.gear, "usergear_user_id": w.alice}), 200)
	if got := e.Count("user_gear_registrations", "userId = ?", w.alice); got != 4 {
		t.Fatalf("alice registrations = %d, want 4", got)
	}
	// A body registration ID never selects the stored ID.
	expectStatus(t, e.Do("PUT", "/api/v1/usergear/insert", w.tokA, map[string]any{"usergear_gear_id": w.gear, "usergear_registration_id": w.bReg}), 200)
	if e.Count("user_gear_registrations", "userGearRegistrationId = ? AND userId = ?", w.bReg, w.bob) != 1 {
		t.Fatal("bob's registration was disturbed")
	}
	if got := e.Count("user_gear_registrations", ""); got != base+3 {
		t.Fatalf("registrations = %d, want %d", got, base+3)
	}
	expectStatus(t, e.Do("PUT", "/api/v1/usergear/insert", "", map[string]any{"usergear_gear_id": w.gear}), 401)
	expectStatus(t, e.Do("PUT", "/api/v1/usergear/insert", w.tokA, "{not json"), 400)
}

// --- BR-012 / BR-017 update -------------------------------------------------

func TestUserGearUpdate_OwnerBoundAndBodyIdsNeverTakeEffect(t *testing.T) {
	w := newUGWorld(t)
	e := w.env

	// Foreign and unknown targets: identical 404, nothing changed, also for an
	// admin, and 404 wins over a conflicting body value (BR-018 AC-4).
	before := e.ugSnapshot()
	body := map[string]any{"max_container_weight": 1, "usergear_user_id": w.alice, "usergear_registration_id": w.aReg1}
	foreign := e.Do("POST", ugPath(w.bReg, "update"), w.tokA, body)
	unknown := e.Do("POST", ugPath(99999, "update"), w.tokA, body)
	expectStatus(t, foreign, 404)
	expectStatus(t, unknown, 404)
	if foreign.Body.String() != unknown.Body.String() {
		t.Fatalf("foreign and unknown differ:\n%s\n%s", foreign.Body.String(), unknown.Body.String())
	}
	expectStatus(t, e.Do("POST", ugPath(w.bReg, "update"), w.tokC, map[string]any{"max_container_weight": 1}), 404)
	expectStatus(t, e.Do("POST", ugPath(w.aReg1, "update"), "", map[string]any{"max_container_weight": 1}), 401)
	if e.ugSnapshot() != before {
		t.Fatal("refused update changed stored data")
	}

	// Conflicting body identifiers on an owned registration: 400, no change.
	for name, b := range map[string]map[string]any{
		"owner":    {"max_container_weight": 5, "usergear_user_id": w.bob},
		"reg id":   {"max_container_weight": 5, "usergear_registration_id": w.bReg},
		"alice id": {"max_container_weight": 5, "usergear_registration_id": w.aReg2},
	} {
		r := e.Do("POST", ugPath(w.aReg1, "update"), w.tokA, b)
		if r.Code != 400 {
			t.Fatalf("%s conflict: status %d, want 400; body: %s", name, r.Code, r.Body.String())
		}
	}
	if e.ugSnapshot() != before {
		t.Fatal("conflicting update changed stored data")
	}

	// Matching and absent identifiers are the same and update the path target only.
	expectStatus(t, e.Do("POST", ugPath(w.aReg1, "update"), w.tokA, map[string]any{"max_container_weight": 5, "usergear_user_id": w.alice, "usergear_registration_id": w.aReg1}), 200)
	expectStatus(t, e.Do("POST", ugPath(w.aReg2, "update"), w.tokA, map[string]any{"max_container_weight": 6}), 200)
	var mw1, mw2, mwB, owner int64
	e.DB.QueryRow(`SELECT maxContainerWeight FROM user_gear_registrations WHERE userGearRegistrationId = ?`, w.aReg1).Scan(&mw1)
	e.DB.QueryRow(`SELECT maxContainerWeight FROM user_gear_registrations WHERE userGearRegistrationId = ?`, w.aReg2).Scan(&mw2)
	e.DB.QueryRow(`SELECT maxContainerWeight, userId FROM user_gear_registrations WHERE userGearRegistrationId = ?`, w.bReg).Scan(&mwB, &owner)
	if mw1 != 5 || mw2 != 6 || mwB != 777 || owner != w.bob {
		t.Fatalf("stored = %d %d %d owner %d", mw1, mw2, mwB, owner)
	}

	// An omitted weight keeps the stored value; an explicit null clears it.
	expectStatus(t, e.Do("POST", ugPath(w.aReg1, "update"), w.tokA, map[string]any{}), 200)
	e.DB.QueryRow(`SELECT maxContainerWeight FROM user_gear_registrations WHERE userGearRegistrationId = ?`, w.aReg1).Scan(&mw1)
	if mw1 != 5 {
		t.Fatalf("omitted weight changed value to %d", mw1)
	}
	expectStatus(t, e.Do("POST", ugPath(w.aReg1, "update"), w.tokA, `{"max_container_weight": null}`), 200)
	if e.Count("user_gear_registrations", "userGearRegistrationId = ? AND maxContainerWeight IS NULL", w.aReg1) != 1 {
		t.Fatal("explicit null did not clear the weight")
	}
	// The gear and owner of a registration never change.
	other := e.seedGearX(true, false, 1, 1)
	expectStatus(t, e.Do("POST", ugPath(w.aReg1, "update"), w.tokA, map[string]any{"usergear_gear_id": other, "max_container_weight": 9}), 200)
	if e.Count("user_gear_registrations", "userGearRegistrationId = ? AND gearId = ? AND userId = ?", w.aReg1, w.gear, w.alice) != 1 {
		t.Fatal("update changed gear or owner")
	}
}

// --- BR-012 / OC-001 delete -------------------------------------------------

func TestUserGearDelete_OwnerOnly_RefusedDeleteKeepsLinks(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	bBox := e.SeedRegistration(w.bob, w.gear)
	e.SeedContainerLink(bBox, w.bReg)
	e.SeedContainerLink(w.bReg, bBox) // bob's registration also holds content
	before := e.ugSnapshot()

	foreign := e.Do("DELETE", ugPath(w.bReg, "delete"), w.tokA, nil)
	unknown := e.Do("DELETE", ugPath(99999, "delete"), w.tokA, nil)
	expectStatus(t, foreign, 404)
	expectStatus(t, unknown, 404)
	if foreign.Body.String() != unknown.Body.String() {
		t.Fatalf("foreign and unknown differ:\n%s\n%s", foreign.Body.String(), unknown.Body.String())
	}
	if strings.Contains(foreign.Body.String(), "bob") {
		t.Fatalf("leaks owner: %s", foreign.Body.String())
	}
	expectStatus(t, e.Do("DELETE", ugPath(w.bReg, "delete"), w.tokC, nil), 404) // no admin exception
	expectStatus(t, e.Do("DELETE", ugPath(w.bReg, "delete"), "", nil), 401)
	if e.ugSnapshot() != before {
		t.Fatal("refused delete changed registrations or container links")
	}
}

func TestUserGearDelete_OwnDeleteRemovesItsLinksOnly(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	box := e.SeedRegistration(w.alice, w.gear)
	e.SeedContainerLink(box, w.aReg1)
	e.SeedContainerLink(box, w.aReg2)
	bBox := e.SeedRegistration(w.bob, w.gear)
	e.SeedContainerLink(bBox, w.bReg)

	expectStatus(t, e.Do("DELETE", ugPath(box, "delete"), w.tokA, nil), 200)
	if e.Count("user_gear_registrations", "userGearRegistrationId = ?", box) != 0 {
		t.Fatal("registration still stored")
	}
	if e.Count("user_container_registration", "userContainerId = ? OR userGearRegistrationId = ?", box, box) != 0 {
		t.Fatal("links of the deleted registration remain")
	}
	if e.Count("user_gear_registrations", "userGearRegistrationId IN (?, ?)", w.aReg1, w.aReg2) != 2 {
		t.Fatal("contents were deleted")
	}
	if e.Count("user_container_registration", "userContainerId = ?", bBox) != 1 {
		t.Fatal("bob's link was removed")
	}
	expectStatus(t, e.Do("DELETE", ugPath(box, "delete"), w.tokA, nil), 404) // already gone
}

func TestUserGearDelete_IsAtomic(t *testing.T) {
	w := newUGWorld(t)
	e := w.env
	box := e.SeedRegistration(w.alice, w.gear)
	e.SeedContainerLink(box, w.aReg1)
	before := e.ugSnapshot()
	if _, err := e.DB.Exec(`CREATE TRIGGER fail_reg_delete BEFORE DELETE ON user_gear_registrations BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	r := e.Do("DELETE", ugPath(box, "delete"), w.tokA, nil)
	if r.Code != 500 {
		t.Fatalf("status %d, want 500; body: %s", r.Code, r.Body.String())
	}
	if e.ugSnapshot() != before {
		t.Fatal("failed delete left a partial effect (links removed without the registration)")
	}
}

// --- BR-013 containers ------------------------------------------------------

type ctWorld struct {
	*ugWorld
	aBox, bBox int64
	aThing     int64 // alice's content
}

func newCTWorld(t *testing.T) *ctWorld {
	w := &ctWorld{ugWorld: newUGWorld(t)}
	boxGear := w.env.seedGearX(true, false, 1, 1)
	w.aBox = w.env.SeedRegistration(w.alice, boxGear)
	w.bBox = w.env.SeedRegistration(w.bob, boxGear)
	w.aThing = w.aReg1
	return w
}

func TestContainerList_OwnerScoped(t *testing.T) {
	w := newCTWorld(t)
	e := w.env
	e.SeedContainerLink(w.aBox, w.aReg2)
	e.SeedContainerLink(w.aBox, w.aThing)
	e.SeedContainerLink(w.aBox, w.bReg) // legacy cross-owner link (BR-013 AC-4)
	e.SeedContainerLink(w.bBox, w.bReg)
	before := e.ugSnapshot()

	r := e.Do("GET", ctList(w.aBox, ""), w.tokA, nil)
	expectStatus(t, r, 200)
	items, total := page(t, r)
	if !sameIDs(regIDs(items), []int64{w.aReg2, w.aThing}) || total != 2 {
		t.Fatalf("list = %v total %d, want alice's two contents only", regIDs(items), total)
	}
	if strings.Contains(r.Body.String(), "777") || strings.Contains(r.Body.String(), `"bob"`) {
		t.Fatalf("response leaks bob's data: %s", r.Body.String())
	}

	// Foreign and unknown containers: identical 404; admin gets no exception.
	foreign := e.Do("GET", ctList(w.bBox, ""), w.tokA, nil)
	unknown := e.Do("GET", ctList(99999, ""), w.tokA, nil)
	expectStatus(t, foreign, 404)
	expectStatus(t, unknown, 404)
	if foreign.Body.String() != unknown.Body.String() {
		t.Fatalf("foreign and unknown differ:\n%s\n%s", foreign.Body.String(), unknown.Body.String())
	}
	expectStatus(t, e.Do("GET", ctList(w.bBox, ""), w.tokC, nil), 404)
	expectStatus(t, e.Do("GET", ctList(w.aBox, ""), w.tokC, nil), 404)
	expectStatus(t, e.Do("GET", ctList(w.aBox, ""), "", nil), 401)
	expectStatus(t, e.Do("GET", "/api/v1/container/abc/list", w.tokA, nil), 400)
	if e.ugSnapshot() != before {
		t.Fatal("list changed stored data")
	}
}

func TestContainerList_EmptyAndPaged(t *testing.T) {
	w := newCTWorld(t)
	e := w.env
	r := e.Do("GET", ctList(w.aBox, ""), w.tokA, nil)
	expectStatus(t, r, 200)
	if items, total := page(t, r); len(items) != 0 || total != 0 {
		t.Fatalf("empty container = %v total %d", items, total)
	}

	e.SeedContainerLink(w.aBox, w.aReg2)
	e.SeedContainerLink(w.aBox, w.aReg1)
	r = e.Do("GET", ctList(w.aBox, "limit=1&page=2"), w.tokA, nil)
	expectStatus(t, r, 200)
	items, total := page(t, r)
	if !sameIDs(regIDs(items), []int64{w.aReg1}) || total != 2 {
		t.Fatalf("page 2 = %v total %d (ordered by link id)", regIDs(items), total)
	}
	// Injected paging values are never 5xx.
	if r := e.Do("GET", ctList(w.aBox, "page="+url.QueryEscape("1 OR 1=1")), w.tokA, nil); r.Code != 400 {
		t.Fatalf("bad page: status %d, want 400: %s", r.Code, r.Body.String())
	}
}

func TestContainerInsert_OnlyOwnOwnCreatesLink(t *testing.T) {
	w := newCTWorld(t)
	e := w.env
	post := func(tok string, container, content int64) *httptest.ResponseRecorder {
		return e.Do("PUT", "/api/v1/container/insert", tok, map[string]any{"user_container_id": container, "user_gear_registration_id": content})
	}
	before := e.ugSnapshot()

	// foreign/own, own/foreign, foreign/foreign, unknown: identical 404, nothing created.
	var bodies []string
	for name, c := range map[string][2]int64{
		"foreign container": {w.bBox, w.aThing},
		"foreign content":   {w.aBox, w.bReg},
		"both foreign":      {w.bBox, w.bReg},
		"unknown":           {99999, 99998},
	} {
		r := post(w.tokA, c[0], c[1])
		if r.Code != 404 {
			t.Fatalf("%s: status %d, want 404; body: %s", name, r.Code, r.Body.String())
		}
		bodies = append(bodies, r.Body.String())
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatalf("refusals differ: %q vs %q", b, bodies[0])
		}
	}
	// Admin gets no exception.
	expectStatus(t, post(w.tokC, w.bBox, w.bReg), 404)
	expectStatus(t, post(w.tokC, w.aBox, w.aThing), 404)
	expectStatus(t, post("", w.aBox, w.aThing), 401)
	if e.ugSnapshot() != before {
		t.Fatal("refused insert created a link")
	}

	// Self-link and incomplete bodies are 400.
	expectStatus(t, post(w.tokA, w.aBox, w.aBox), 400)
	expectStatus(t, e.Do("PUT", "/api/v1/container/insert", w.tokA, map[string]any{"user_container_id": w.aBox}), 400)
	expectStatus(t, e.Do("PUT", "/api/v1/container/insert", w.tokA, "{bad"), 400)
	// A foreign self-link reveals nothing: 404.
	expectStatus(t, post(w.tokA, w.bBox, w.bBox), 404)
	if e.ugSnapshot() != before {
		t.Fatal("refused insert created a link")
	}

	expectStatus(t, post(w.tokA, w.aBox, w.aThing), 200)
	if e.Count("user_container_registration", "userContainerId = ? AND userGearRegistrationId = ?", w.aBox, w.aThing) != 1 {
		t.Fatal("own/own link was not created")
	}
	if e.Count("user_container_registration", "") != 1 {
		t.Fatal("unexpected extra links")
	}
}

func TestContainerDelete_OwnerOfContainerOnly(t *testing.T) {
	w := newCTWorld(t)
	e := w.env
	e.SeedContainerLink(w.aBox, w.aThing)
	e.SeedContainerLink(w.aBox, w.bReg) // legacy cross-owner link in alice's container
	e.SeedContainerLink(w.bBox, w.bReg)
	aLink := e.linkID(w.aBox, w.aThing)
	legacy := e.linkID(w.aBox, w.bReg)
	bLink := e.linkID(w.bBox, w.bReg)
	before := e.ugSnapshot()

	foreign := e.Do("DELETE", ctDelete(bLink), w.tokA, nil)
	unknown := e.Do("DELETE", ctDelete(99999), w.tokA, nil)
	expectStatus(t, foreign, 404)
	expectStatus(t, unknown, 404)
	if foreign.Body.String() != unknown.Body.String() {
		t.Fatalf("foreign and unknown differ:\n%s\n%s", foreign.Body.String(), unknown.Body.String())
	}
	expectStatus(t, e.Do("DELETE", ctDelete(bLink), w.tokC, nil), 404)
	expectStatus(t, e.Do("DELETE", ctDelete(aLink), w.tokC, nil), 404)
	expectStatus(t, e.Do("DELETE", ctDelete(aLink), "", nil), 401)
	expectStatus(t, e.Do("DELETE", "/api/v1/container/abc/delete", w.tokA, nil), 400)
	if e.ugSnapshot() != before {
		t.Fatal("refused delete removed a link")
	}

	expectStatus(t, e.Do("DELETE", ctDelete(aLink), w.tokA, nil), 200)
	expectStatus(t, e.Do("DELETE", ctDelete(legacy), w.tokA, nil), 200) // alice owns the container
	if e.Count("user_container_registration", "") != 1 || e.Count("user_container_registration", "containerRegistrationId = ?", bLink) != 1 {
		t.Fatal("wrong links remain")
	}
	if e.Count("user_gear_registrations", "") != 5 {
		t.Fatal("deleting a link deleted a registration")
	}
	expectStatus(t, e.Do("DELETE", ctDelete(aLink), w.tokA, nil), 404) // already gone
}
