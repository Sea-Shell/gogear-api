package endpoints

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Adversarial regression tests for BR-014..BR-018 and OC-001 (loadouts, loadout
// items, public loadouts). They run through the real JWT middleware and the
// real route table (see authz_harness_test.go) against a migrated database.

const oldTimestamp = "2000-01-01T00:00:00.000Z"

func loPath(id int64, op string) string { return fmt.Sprintf("/api/v1/loadout/%d/%s", id, op) }
func itemPath(l, i int64, op string) string {
	return fmt.Sprintf("/api/v1/loadout/%d/item/%d/%s", l, i, op)
}

// seedLO inserts a loadout with a NULL slug when slug is "", and an old
// updatedAt so a later change is visible.
func (e *authzEnv) seedLO(userID int64, public bool, slug string) int64 {
	e.T.Helper()
	var s any
	if slug != "" {
		s = slug
	}
	res, err := e.DB.Exec(
		`INSERT INTO loadouts (userId, loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug, totalWeight, createdAt, updatedAt)
		 VALUES (?, 'orig-name', 'orig-desc', ?, ?, 0, ?, ?)`, userID, public, s, oldTimestamp, oldTimestamp)
	if err != nil {
		e.T.Fatalf("seed loadout: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedGearW inserts gear weighing weight and returns its ID.
func (e *authzEnv) seedGearW(weight int64) int64 {
	e.T.Helper()
	res, err := e.DB.Exec(
		`INSERT INTO gear (gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight) VALUES (1, 1, 1, 'g', ?)`, weight)
	if err != nil {
		e.T.Fatalf("seed gear: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedItem adds an item and keeps the loadout's stored total consistent.
func (e *authzEnv) seedItem(loadoutID, gearID, qty int64) int64 {
	e.T.Helper()
	res, err := e.DB.Exec(`INSERT INTO loadout_items (loadoutId, gearId, quantity, notes) VALUES (?, ?, ?, 'orig-notes')`, loadoutID, gearID, qty)
	if err != nil {
		e.T.Fatalf("seed item: %v", err)
	}
	id, _ := res.LastInsertId()
	if err := LoadoutRecalculateWeight(e.T.Context(), e.DB, loadoutID); err != nil {
		e.T.Fatal(err)
	}
	return id
}

// snapshot is the complete stored state of every loadout and item.
func (e *authzEnv) snapshot() string {
	e.T.Helper()
	var sb strings.Builder
	for _, q := range []string{
		`SELECT quote(loadoutId)||'|'||quote(userId)||'|'||quote(loadoutName)||'|'||quote(loadoutDescription)||'|'||quote(loadoutIsPublic)||'|'||quote(loadoutSlug)||'|'||quote(totalWeight)||'|'||quote(createdAt)||'|'||quote(updatedAt) FROM loadouts ORDER BY loadoutId`,
		`SELECT quote(loadoutItemId)||'|'||quote(loadoutId)||'|'||quote(gearId)||'|'||quote(quantity)||'|'||quote(notes) FROM loadout_items ORDER BY loadoutItemId`,
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

func (e *authzEnv) weight(loadoutID int64) int64 {
	e.T.Helper()
	var w int64
	if err := e.DB.QueryRow(`SELECT totalWeight FROM loadouts WHERE loadoutId = ?`, loadoutID).Scan(&w); err != nil {
		e.T.Fatal(err)
	}
	return w
}

// loFixture: alice owns L1 (items i1a, i1b) and L2 (item i2); bob owns L3 (item i3).
type loFixture struct {
	env            *authzEnv
	alice, bob     int64
	aTok, bTok     string
	l1, l2, l3     int64
	i1a, i1b       int64
	i2, i3         int64
	g100, g250     int64
	pubSlug, bSlug string
}

func newLoFixture(t *testing.T) *loFixture {
	env := newAuthzEnv(t)
	f := &loFixture{env: env, pubSlug: "alice-public", bSlug: "bob-private"}
	f.alice = env.SeedUser("alice", "alice@example.com", false)
	f.bob = env.SeedUser("bob", "bob@example.com", false)
	f.aTok, f.bTok = env.Token(f.alice, false), env.Token(f.bob, false)
	f.g100, f.g250 = env.seedGearW(100), env.seedGearW(250)
	f.l1 = env.seedLO(f.alice, true, f.pubSlug)
	f.l2 = env.seedLO(f.alice, false, "alice-private")
	f.l3 = env.seedLO(f.bob, false, f.bSlug)
	f.i1a = env.seedItem(f.l1, f.g100, 2) // 200
	f.i1b = env.seedItem(f.l1, f.g250, 1) // +250 = 450
	f.i2 = env.seedItem(f.l2, f.g100, 3)  // 300
	f.i3 = env.seedItem(f.l3, f.g250, 4)  // 1000
	return f
}

// ---- BR-014 AC-3 / BR-017: update is bound to the path loadout -------------

func TestLoadoutUpdate_BodyIDCannotRedirect(t *testing.T) {
	f := newLoFixture(t)
	before := f.env.snapshot()

	// A updates its own loadout, but the body names Bob's loadout.
	body := map[string]any{"loadout_id": f.l3, "loadout_name": "HACKED"}
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l1, "update"), f.aTok, body), http.StatusBadRequest)
	if got := f.env.snapshot(); got != before {
		t.Fatalf("conflicting body loadout_id changed stored data:\nbefore:\n%s\nafter:\n%s", before, got)
	}

	// Body user_id naming someone else is a conflict too.
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l1, "update"), f.aTok,
		map[string]any{"user_id": f.bob, "loadout_name": "x"}), http.StatusBadRequest)
	if got := f.env.snapshot(); got != before {
		t.Fatal("conflicting body user_id changed stored data")
	}
}

func TestLoadoutUpdate_OnlySuppliedFieldsChange_OwnerFixed(t *testing.T) {
	f := newLoFixture(t)

	// Matching body ID plus one field: only that field changes.
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l2, "update"), f.aTok,
		map[string]any{"loadout_id": f.l2, "loadout_name": "renamed"}), http.StatusOK)
	var name, desc, slug, updated string
	var public bool
	var owner int64
	if err := f.env.DB.QueryRow(`SELECT loadoutName, loadoutDescription, loadoutIsPublic, loadoutSlug, userId, updatedAt FROM loadouts WHERE loadoutId = ?`, f.l2).
		Scan(&name, &desc, &public, &slug, &owner, &updated); err != nil {
		t.Fatal(err)
	}
	if name != "renamed" || desc != "orig-desc" || public || slug != "alice-private" || owner != f.alice {
		t.Errorf("unexpected stored state: name=%q desc=%q public=%v slug=%q owner=%d", name, desc, public, slug, owner)
	}
	if updated == oldTimestamp {
		t.Error("updatedAt was not maintained")
	}

	// Body without loadout_id works the same.
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l2, "update"), f.aTok,
		map[string]any{"loadout_description": "new-desc", "loadout_is_public": true}), http.StatusOK)
	if err := f.env.DB.QueryRow(`SELECT loadoutName, loadoutDescription, loadoutIsPublic FROM loadouts WHERE loadoutId = ?`, f.l2).
		Scan(&name, &desc, &public); err != nil {
		t.Fatal(err)
	}
	if name != "renamed" || desc != "new-desc" || !public {
		t.Errorf("unexpected stored state: name=%q desc=%q public=%v", name, desc, public)
	}

	// Empty update is refused.
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l2, "update"), f.aTok, map[string]any{}), http.StatusBadRequest)
}

// ---- BR-014/015/018: foreign, unknown and wrong-parent are one identical 404 ----

func TestLoadout_ForeignAndUnknownGiveIdenticalNotFound(t *testing.T) {
	f := newLoFixture(t)
	const unknown = int64(987654)
	itemBody := map[string]any{"gear_id": f.g100, "quantity": 1}
	validUpdate := map[string]any{"loadout_name": "HACKED"}
	conflicting := map[string]any{"loadout_id": f.l1, "loadout_name": "HACKED"}

	type op struct {
		name, method string
		path         func(l int64) string
		body         any
	}
	ops := []op{
		{"get", http.MethodGet, func(l int64) string { return loPath(l, "get") }, nil},
		{"update", http.MethodPost, func(l int64) string { return loPath(l, "update") }, validUpdate},
		{"update conflicting body", http.MethodPost, func(l int64) string { return loPath(l, "update") }, conflicting},
		{"delete", http.MethodDelete, func(l int64) string { return loPath(l, "delete") }, nil},
		{"import", http.MethodPost, func(l int64) string { return loPath(l, "import") }, map[string]any{"gear_ids": []int64{f.g100}}},
		{"item insert", http.MethodPut, func(l int64) string { return loPath(l, "item/insert") }, itemBody},
		{"item list", http.MethodGet, func(l int64) string { return loPath(l, "item/list") }, nil},
	}
	adminAlice := f.env.Token(f.alice, true) // P-3: no admin exception

	before := f.env.snapshot()
	for _, tok := range []struct{ who, token string }{{"alice", f.aTok}, {"admin alice", adminAlice}} {
		for _, o := range ops {
			foreign := f.env.Do(o.method, o.path(f.l3), tok.token, o.body)
			missing := f.env.Do(o.method, o.path(unknown), tok.token, o.body)
			if foreign.Code != http.StatusNotFound {
				t.Errorf("%s %s foreign: status %d, want 404; body %s", tok.who, o.name, foreign.Code, foreign.Body)
			}
			if foreign.Code != missing.Code || foreign.Body.String() != missing.Body.String() {
				t.Errorf("%s %s: foreign and unknown differ:\nforeign %d %s\nunknown %d %s",
					tok.who, o.name, foreign.Code, foreign.Body, missing.Code, missing.Body)
			}
			if strings.Contains(foreign.Body.String(), "bob") || strings.Contains(foreign.Body.String(), f.bSlug) {
				t.Errorf("%s %s: refusal leaks Bob's data: %s", tok.who, o.name, foreign.Body)
			}
		}
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("refused requests changed stored data:\nbefore:\n%s\nafter:\n%s", before, got)
	}

	// Item update/delete through a foreign or unknown loadout, or with an unknown item.
	for _, tc := range []struct {
		name, method, path string
		body               any
	}{
		{"update foreign loadout+its item", http.MethodPost, itemPath(f.l3, f.i3, "update"), map[string]any{"quantity": 9}},
		{"delete foreign loadout+its item", http.MethodDelete, itemPath(f.l3, f.i3, "delete"), nil},
		{"update own loadout+foreign item", http.MethodPost, itemPath(f.l1, f.i3, "update"), map[string]any{"quantity": 9}},
		{"delete own loadout+foreign item", http.MethodDelete, itemPath(f.l1, f.i3, "delete"), nil},
		{"update own loadout+sibling loadout item", http.MethodPost, itemPath(f.l1, f.i2, "update"), map[string]any{"quantity": 9}},
		{"delete own loadout+sibling loadout item", http.MethodDelete, itemPath(f.l1, f.i2, "delete"), nil},
		{"update unknown", http.MethodPost, itemPath(unknown, unknown, "update"), map[string]any{"quantity": 9}},
		{"delete unknown", http.MethodDelete, itemPath(unknown, unknown, "delete"), nil},
		{"update own loadout+unknown item", http.MethodPost, itemPath(f.l1, unknown, "update"), map[string]any{"quantity": 9}},
		{"delete own loadout+unknown item", http.MethodDelete, itemPath(f.l1, unknown, "delete"), nil},
	} {
		w := f.env.Do(tc.method, tc.path, f.aTok, tc.body)
		ref := f.env.Do(tc.method, itemPath(unknown, unknown, strings.Split(tc.path, "/")[len(strings.Split(tc.path, "/"))-1]), f.aTok, tc.body)
		if w.Code != http.StatusNotFound || w.Code != ref.Code || w.Body.String() != ref.Body.String() {
			t.Errorf("%s: got %d %s, want the unknown-record 404 %d %s", tc.name, w.Code, w.Body, ref.Code, ref.Body)
		}
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("refused item requests changed stored data:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

func TestLoadout_RequiresAuthentication(t *testing.T) {
	f := newLoFixture(t)
	before := f.env.snapshot()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/loadout/list"},
		{http.MethodGet, loPath(f.l1, "get")},
		{http.MethodPost, loPath(f.l1, "update")},
		{http.MethodDelete, loPath(f.l1, "delete")},
		{http.MethodDelete, itemPath(f.l1, f.i1a, "delete")},
	} {
		expectStatus(t, f.env.Do(tc.method, tc.path, "", map[string]any{"loadout_name": "x"}), http.StatusUnauthorized)
	}
	if f.env.snapshot() != before {
		t.Fatal("unauthenticated request changed data")
	}
}

func TestLoadoutList_OnlyOwn(t *testing.T) {
	f := newLoFixture(t)
	w := f.env.Do(http.MethodGet, "/api/v1/loadout/list", f.aTok, nil)
	expectStatus(t, w, http.StatusOK)
	var list []map[string]any
	decodeJSON(t, w, &list)
	if len(list) != 2 {
		t.Fatalf("want Alice's 2 loadouts, got %d: %s", len(list), w.Body)
	}
	for _, l := range list {
		if int64(l["user_id"].(float64)) != f.alice {
			t.Errorf("list contains a foreign loadout: %v", l)
		}
	}
}

// ---- BR-015 AC-3/AC-4: every pairing, with total weight checks --------------

func TestLoadoutItems_PairingsAndTotals(t *testing.T) {
	f := newLoFixture(t)
	before := f.env.snapshot()

	refused := []struct {
		name         string
		loadout, itm int64
	}{
		{"own loadout, foreign item", f.l1, f.i3},
		{"foreign loadout, its own item", f.l3, f.i3},
		{"foreign loadout, Alice's item", f.l3, f.i1a},
		{"own loadout, item of another own loadout", f.l1, f.i2},
		{"other own loadout, item of first", f.l2, f.i1a},
	}
	for _, tc := range refused {
		expectStatus(t, f.env.Do(http.MethodPost, itemPath(tc.loadout, tc.itm, "update"), f.aTok,
			map[string]any{"quantity": 9, "notes": "pwn"}), http.StatusNotFound)
		if got := f.env.snapshot(); got != before {
			t.Fatalf("%s: refused update changed data:\n%s", tc.name, got)
		}
		expectStatus(t, f.env.Do(http.MethodDelete, itemPath(tc.loadout, tc.itm, "delete"), f.aTok, nil), http.StatusNotFound)
		if got := f.env.snapshot(); got != before {
			t.Fatalf("%s: refused delete changed data:\n%s", tc.name, got)
		}
	}
	if f.env.weight(f.l1) != 450 || f.env.weight(f.l2) != 300 || f.env.weight(f.l3) != 1000 {
		t.Fatalf("totals changed: %d %d %d", f.env.weight(f.l1), f.env.weight(f.l2), f.env.weight(f.l3))
	}

	// Allowed: own loadout and own member item. Totals follow in the same operation.
	expectStatus(t, f.env.Do(http.MethodPost, itemPath(f.l1, f.i1a, "update"), f.aTok,
		map[string]any{"quantity": 5}), http.StatusOK)
	if w := f.env.weight(f.l1); w != 5*100+250 {
		t.Errorf("L1 total after update = %d, want 750", w)
	}
	var notes string
	f.env.DB.QueryRow(`SELECT notes FROM loadout_items WHERE loadoutItemId = ?`, f.i1a).Scan(&notes)
	if notes != "orig-notes" {
		t.Errorf("notes not supplied but changed to %q", notes)
	}
	expectStatus(t, f.env.Do(http.MethodDelete, itemPath(f.l1, f.i1b, "delete"), f.aTok, nil), http.StatusOK)
	if w := f.env.weight(f.l1); w != 500 {
		t.Errorf("L1 total after delete = %d, want 500", w)
	}
	if f.env.weight(f.l2) != 300 || f.env.weight(f.l3) != 1000 {
		t.Errorf("other loadouts' totals changed: L2=%d L3=%d", f.env.weight(f.l2), f.env.weight(f.l3))
	}
	if f.env.Count("loadout_items", "loadoutItemId = ?", f.i1b) != 0 {
		t.Error("deleted item still stored")
	}
}

func TestLoadoutItems_BodyConflictsRejected(t *testing.T) {
	f := newLoFixture(t)
	before := f.env.snapshot()
	for name, body := range map[string]map[string]any{
		"body item id of another item":   {"loadout_item_id": f.i3, "quantity": 9},
		"body loadout id of another one": {"loadout_id": f.l3, "quantity": 9},
	} {
		w := f.env.Do(http.MethodPost, itemPath(f.l1, f.i1a, "update"), f.aTok, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400; %s", name, w.Code, w.Body)
		}
	}
	// Insert with a body loadout_id naming Bob's loadout must not add to either.
	w := f.env.Do(http.MethodPut, loPath(f.l1, "item/insert"), f.aTok, map[string]any{"loadout_id": f.l3, "gear_id": f.g100, "quantity": 1})
	if w.Code != http.StatusBadRequest {
		t.Errorf("insert with foreign body loadout_id: status %d, want 400; %s", w.Code, w.Body)
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("conflicting bodies changed stored data:\n%s", got)
	}

	// Matching and absent IDs behave the same and succeed.
	expectStatus(t, f.env.Do(http.MethodPost, itemPath(f.l1, f.i1a, "update"), f.aTok,
		map[string]any{"loadout_item_id": f.i1a, "loadout_id": f.l1, "quantity": 3}), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodPut, loPath(f.l1, "item/insert"), f.aTok,
		map[string]any{"loadout_id": f.l1, "gear_id": f.g100, "quantity": 1}), http.StatusCreated)
}

// ---- Quantity: zero and negative are refused --------------------------------

func TestLoadoutItems_QuantityMustBePositive(t *testing.T) {
	f := newLoFixture(t)
	before := f.env.snapshot()
	for _, q := range []int64{0, -1, -1000} {
		expectStatus(t, f.env.Do(http.MethodPut, loPath(f.l1, "item/insert"), f.aTok,
			map[string]any{"gear_id": f.g100, "quantity": q}), http.StatusBadRequest)
		expectStatus(t, f.env.Do(http.MethodPost, itemPath(f.l1, f.i1a, "update"), f.aTok,
			map[string]any{"quantity": q}), http.StatusBadRequest)
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("rejected quantities changed data:\n%s", got)
	}
	if w := f.env.weight(f.l1); w < 0 {
		t.Fatalf("total weight went negative: %d", w)
	}

	// Absent quantity defaults to 1 (the column default).
	w := f.env.Do(http.MethodPut, loPath(f.l1, "item/insert"), f.aTok, map[string]any{"gear_id": f.g100})
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Quantity  int64 `json:"quantity"`
		LoadoutID int64 `json:"loadout_id"`
	}
	decodeJSON(t, w, &created)
	if created.Quantity != 1 || created.LoadoutID != f.l1 {
		t.Errorf("created item = %+v", created)
	}
}

// ---- OC-001 G-5 / COR-04: atomic import, item mutations --------------------

func TestImportLoadout_AtomicOnInsertFailure(t *testing.T) {
	f := newLoFixture(t)
	bad := f.env.seedGearW(1) // the trigger below rejects inserts of this gear
	if _, err := f.env.DB.Exec(`CREATE TRIGGER fail_gear BEFORE INSERT ON loadout_items WHEN NEW.gearId = ` + fmt.Sprint(bad) +
		` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	before := f.env.snapshot()
	w := f.env.Do(http.MethodPost, loPath(f.l2, "import"), f.aTok, map[string]any{"gear_ids": []int64{f.g100, f.g250, bad}})
	if w.Code < 500 {
		t.Fatalf("failed import must not succeed: %d %s", w.Code, w.Body)
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("partial import left effects:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

func TestImportLoadout_WeightRecalcFailureRollsBack(t *testing.T) {
	f := newLoFixture(t)
	if _, err := f.env.DB.Exec(`CREATE TRIGGER fail_recalc BEFORE UPDATE OF totalWeight ON loadouts
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	before := f.env.snapshot()
	w := f.env.Do(http.MethodPost, loPath(f.l2, "import"), f.aTok, map[string]any{"gear_ids": []int64{f.g100}})
	if w.Code < 500 {
		t.Fatalf("import with failed recalculation must not succeed: %d %s", w.Code, w.Body)
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("import kept its items although the total could not be recalculated:\n%s", got)
	}

	// The same for item insert, update and delete.
	for name, do := range map[string]func() int{
		"insert": func() int {
			return f.env.Do(http.MethodPut, loPath(f.l2, "item/insert"), f.aTok, map[string]any{"gear_id": f.g100, "quantity": 2}).Code
		},
		"update": func() int {
			return f.env.Do(http.MethodPost, itemPath(f.l2, f.i2, "update"), f.aTok, map[string]any{"quantity": 7}).Code
		},
		"delete": func() int {
			return f.env.Do(http.MethodDelete, itemPath(f.l2, f.i2, "delete"), f.aTok, nil).Code
		},
	} {
		if code := do(); code < 500 {
			t.Errorf("item %s with failed recalculation returned %d", name, code)
		}
		if got := f.env.snapshot(); got != before {
			t.Fatalf("item %s kept its change although the total could not be recalculated:\n%s", name, got)
		}
	}
}

func TestImportLoadout_Success(t *testing.T) {
	f := newLoFixture(t)
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l2, "import"), f.aTok,
		map[string]any{"gear_ids": []int64{f.g100, f.g250}}), http.StatusCreated)
	if w := f.env.weight(f.l2); w != 300+100+250 {
		t.Errorf("total after import = %d, want 650", w)
	}
}

// ---- COR-09: empty lists are [], never null ---------------------------------

func TestEmptyListsEncodeAsArray(t *testing.T) {
	env := newAuthzEnv(t)
	u := env.SeedUser("carol", "carol@example.com", false)
	tok := env.Token(u, false)
	priv := env.seedLO(u, false, "carol-private")
	pub := env.seedLO(u, true, "carol-public")

	empty := newAuthzEnv(t)
	lonely := empty.SeedUser("dave", "dave@example.com", false)

	for _, tc := range []struct {
		name string
		env  *authzEnv
		path string
		tok  string
	}{
		{"loadout list without loadouts", empty, "/api/v1/loadout/list", empty.Token(lonely, false)},
		{"private loadout item list", env, loPath(priv, "item/list"), tok},
		{"public loadout item list (owner)", env, loPath(pub, "item/list"), tok},
		{"public items by slug, anonymous", env, "/api/v1/public/loadout/carol-public/items", ""},
	} {
		w := tc.env.Do(http.MethodGet, tc.path, tc.tok, nil)
		expectStatus(t, w, http.StatusOK)
		if got := strings.TrimSpace(w.Body.String()); got != "[]" {
			t.Errorf("%s: body = %q, want []", tc.name, got)
		}
	}
}

// ---- Slugs: blank is stored as NULL; blank never makes a loadout reachable ----

func TestInsertLoadout_BlankSlugStoredAsNull(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)

	for i, tc := range []struct {
		tok  string
		body map[string]any
	}{
		{env.Token(a, false), map[string]any{"loadout_name": "A no slug", "loadout_is_public": true}},
		{env.Token(b, false), map[string]any{"loadout_name": "B no slug", "loadout_is_public": true}},
		{env.Token(b, false), map[string]any{"loadout_name": "B blank slug", "loadout_slug": ""}},
		{env.Token(a, false), map[string]any{"loadout_name": "A spaces slug", "loadout_slug": "   "}},
	} {
		w := env.Do(http.MethodPut, "/api/v1/loadout/insert", tc.tok, tc.body)
		if w.Code != http.StatusCreated {
			t.Fatalf("slugless insert #%d: status %d, want 201; %s", i, w.Code, w.Body)
		}
	}
	if n := env.Count("loadouts", "loadoutSlug IS NULL"); n != 4 {
		t.Errorf("%d loadouts have a NULL slug, want 4", n)
	}
	if n := env.Count("loadouts", "loadoutSlug = ''"); n != 0 {
		t.Errorf("%d loadouts stored a blank slug", n)
	}

	// Real slugs stay unique: the clash is a 409, not a 500, and changes nothing.
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/loadout/insert", env.Token(a, false),
		map[string]any{"loadout_name": "x", "loadout_slug": "taken"}), http.StatusCreated)
	expectStatus(t, env.Do(http.MethodPut, "/api/v1/loadout/insert", env.Token(b, false),
		map[string]any{"loadout_name": "y", "loadout_slug": "taken"}), http.StatusConflict)

	// The response never shows a made-up slug, and the owner is the caller.
	w := env.Do(http.MethodPut, "/api/v1/loadout/insert", env.Token(a, false), map[string]any{"loadout_name": "z"})
	expectStatus(t, w, http.StatusCreated)
	var got map[string]any
	decodeJSON(t, w, &got)
	if int64(got["user_id"].(float64)) != a || got["loadout_slug"] != "" {
		t.Errorf("created loadout = %v", got)
	}
}

func TestUpdateLoadout_BlankSlugClearsSlug(t *testing.T) {
	f := newLoFixture(t)
	other := f.env.seedLO(f.alice, true, "")
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l1, "update"), f.aTok, map[string]any{"loadout_slug": " "}), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l2, "update"), f.aTok, map[string]any{"loadout_slug": ""}), http.StatusOK)
	if n := f.env.Count("loadouts", "loadoutSlug IS NULL AND userId = ?", f.alice); n != 3 {
		t.Errorf("want 3 slugless loadouts for Alice (incl. %d), got %d", other, n)
	}
	// A former public loadout with no slug is not reachable.
	expectStatus(t, f.env.Do(http.MethodGet, "/api/v1/public/loadout/"+f.pubSlug, "", nil), http.StatusNotFound)
}

func TestPublicLoadout_BlankSlugNeverReachable(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	// A legacy row with a blank slug that is public.
	if _, err := env.DB.Exec(`INSERT INTO loadouts (userId, loadoutName, loadoutIsPublic, loadoutSlug) VALUES (?, 'legacy', 1, '')`, a); err != nil {
		t.Fatal(err)
	}
	env.seedLO(a, true, "")
	unknown := env.Do(http.MethodGet, "/api/v1/public/loadout/does-not-exist", "", nil)
	for _, slug := range []string{"", "%20", "%20%20", "null", "NULL", "%00"} {
		for _, suffix := range []string{"", "/items"} {
			w := env.Do(http.MethodGet, "/api/v1/public/loadout/"+slug+suffix, "", nil)
			if w.Code == http.StatusOK {
				t.Errorf("slug %q%s reached a loadout: %s", slug, suffix, w.Body)
			}
		}
	}
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown slug: %d", unknown.Code)
	}
}

// ---- BR-014 AC-4 / BR-017: owner comes from the token on insert ------------

func TestInsertLoadout_OwnerFromTokenOnly(t *testing.T) {
	env := newAuthzEnv(t)
	a := env.SeedUser("alice", "alice@example.com", false)
	b := env.SeedUser("bob", "bob@example.com", false)
	tok := env.Token(a, false)

	w := env.Do(http.MethodPut, "/api/v1/loadout/insert", tok, map[string]any{"user_id": b, "loadout_name": "for bob"})
	expectStatus(t, w, http.StatusBadRequest)
	if env.Count("loadouts", "") != 0 {
		t.Fatal("a loadout was created for a conflicting owner")
	}
	// Matching and absent user_id both create a loadout owned by the caller.
	for _, body := range []map[string]any{{"user_id": a, "loadout_name": "m"}, {"loadout_name": "n"}} {
		expectStatus(t, env.Do(http.MethodPut, "/api/v1/loadout/insert", tok, body), http.StatusCreated)
	}
	if env.Count("loadouts", "userId = ?", a) != 2 || env.Count("loadouts", "userId = ?", b) != 0 {
		t.Error("loadout owner not taken from the token")
	}
}

// ---- BR-016: public view --------------------------------------------------

func TestPublicLoadout_VisibilityLifecycle(t *testing.T) {
	f := newLoFixture(t)
	pubPath := "/api/v1/public/loadout/" + f.pubSlug

	// Public, no token: readable with items.
	w := f.env.Do(http.MethodGet, pubPath, "", nil)
	expectStatus(t, w, http.StatusOK)
	if strings.Contains(w.Body.String(), "user_id") {
		t.Errorf("public view exposes the owner: %s", w.Body)
	}
	w = f.env.Do(http.MethodGet, pubPath+"/items", "", nil)
	expectStatus(t, w, http.StatusOK)
	var items []map[string]any
	decodeJSON(t, w, &items)
	if len(items) != 2 {
		t.Errorf("want 2 public items, got %d", len(items))
	}

	// Private and unknown slugs are byte-identical, for both endpoints.
	for _, suffix := range []string{"", "/items"} {
		priv := f.env.Do(http.MethodGet, "/api/v1/public/loadout/"+f.bSlug+suffix, "", nil)
		unk := f.env.Do(http.MethodGet, "/api/v1/public/loadout/no-such-slug"+suffix, "", nil)
		if priv.Code != http.StatusNotFound || priv.Code != unk.Code || priv.Body.String() != unk.Body.String() {
			t.Errorf("private vs unknown%s differ: %d %q vs %d %q", suffix, priv.Code, priv.Body, unk.Code, unk.Body)
		}
	}

	// The public view is read-only.
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if w := f.env.Do(m, pubPath, "", map[string]any{"loadout_name": "x"}); w.Code == http.StatusOK || w.Code == http.StatusCreated {
			t.Errorf("%s on public view succeeded: %d", m, w.Code)
		}
	}

	// Owner makes it private: 404. Owner makes it public again, then deletes it: 404.
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l1, "update"), f.aTok, map[string]any{"loadout_is_public": false}), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodGet, pubPath, "", nil), http.StatusNotFound)
	expectStatus(t, f.env.Do(http.MethodGet, pubPath+"/items", "", nil), http.StatusNotFound)
	expectStatus(t, f.env.Do(http.MethodPost, loPath(f.l1, "update"), f.aTok, map[string]any{"loadout_is_public": true}), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodGet, pubPath, "", nil), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodDelete, loPath(f.l1, "delete"), f.aTok, nil), http.StatusOK)
	expectStatus(t, f.env.Do(http.MethodGet, pubPath, "", nil), http.StatusNotFound)
	if f.env.Count("loadout_items", "loadoutId = ?", f.l1) != 0 {
		t.Error("deleting a loadout left its items behind")
	}
}

func TestDeleteLoadout_OwnerDeletesOnlyOwn(t *testing.T) {
	f := newLoFixture(t)
	expectStatus(t, f.env.Do(http.MethodDelete, loPath(f.l3, "delete"), f.aTok, nil), http.StatusNotFound)
	if f.env.Count("loadouts", "loadoutId = ?", f.l3) != 1 || f.env.Count("loadout_items", "loadoutId = ?", f.l3) != 1 {
		t.Fatal("Bob's loadout or items were deleted by Alice")
	}
	expectStatus(t, f.env.Do(http.MethodDelete, loPath(f.l2, "delete"), f.aTok, nil), http.StatusOK)
	if f.env.Count("loadouts", "loadoutId = ?", f.l2) != 0 || f.env.Count("loadout_items", "loadoutId = ?", f.l2) != 0 {
		t.Fatal("own loadout or its items still stored")
	}
}

// ---- BR-014 AC-2: a foreign public loadout is public only on the public route ----

func TestLoadout_ForeignPublicNotReadableOrWritableThroughAuthenticatedRoutes(t *testing.T) {
	f := newLoFixture(t) // alice's l1 is public
	before := f.env.snapshot()

	// The public route does serve it, so the refusals below are about the route, not the data.
	expectStatus(t, f.env.Do(http.MethodGet, "/api/v1/public/loadout/"+f.pubSlug, "", nil), http.StatusOK)

	for _, o := range []struct {
		name, method, path string
		body               any
	}{
		{"get", http.MethodGet, loPath(f.l1, "get"), nil},
		{"item list", http.MethodGet, loPath(f.l1, "item/list"), nil},
		{"update", http.MethodPost, loPath(f.l1, "update"), map[string]any{"loadout_name": "HACKED"}},
		{"delete", http.MethodDelete, loPath(f.l1, "delete"), nil},
		{"import", http.MethodPost, loPath(f.l1, "import"), map[string]any{"gear_ids": []int64{f.g100}}},
		{"item insert", http.MethodPut, loPath(f.l1, "item/insert"), map[string]any{"gear_id": f.g100, "quantity": 1}},
		{"item update", http.MethodPost, itemPath(f.l1, f.i1a, "update"), map[string]any{"quantity": 9}},
		{"item delete", http.MethodDelete, itemPath(f.l1, f.i1a, "delete"), nil},
	} {
		w := f.env.Do(o.method, o.path, f.bTok, o.body)
		if w.Code != http.StatusNotFound {
			t.Errorf("bob %s on alice's public loadout: status %d, want 404; body %s", o.name, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "orig-name") || strings.Contains(w.Body.String(), f.pubSlug) {
			t.Errorf("bob %s: response leaks the loadout: %s", o.name, w.Body)
		}
	}

	// It does not appear in bob's own list either.
	w := f.env.Do(http.MethodGet, "/api/v1/loadout/list", f.bTok, nil)
	expectStatus(t, w, http.StatusOK)
	if strings.Contains(w.Body.String(), f.pubSlug) {
		t.Errorf("bob's list contains alice's public loadout: %s", w.Body)
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("refused requests changed stored data:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// ---- OC-001 G-2: the item query binds the owner itself ---------------------

func TestLoadoutItemsOwned_QueryBindsOwner(t *testing.T) {
	f := newLoFixture(t)
	ctx := t.Context()

	items, err := LoadoutItemsOwned(ctx, f.env.DB, f.l1, f.alice)
	if err != nil || len(*items) != 2 {
		t.Fatalf("owner read: %v, %v; want 2 items", items, err)
	}
	for _, who := range []int64{f.bob, 999999} {
		items, err := LoadoutItemsOwned(ctx, f.env.DB, f.l1, who)
		if err != nil || items == nil || len(*items) != 0 {
			t.Errorf("user %d read alice's items: %v, %v; want none", who, items, err)
		}
	}
}

// ---- COR-04: import checks that each insert added exactly one row ----------

func TestImportLoadout_SkippedInsertRollsBack(t *testing.T) {
	f := newLoFixture(t)
	skipped := f.env.seedGearW(1)
	// The trigger makes the insert of this gear a silent no-op: no error, zero rows.
	if _, err := f.env.DB.Exec(`CREATE TRIGGER skip_gear BEFORE INSERT ON loadout_items WHEN NEW.gearId = ` + fmt.Sprint(skipped) +
		` BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	before := f.env.snapshot()
	w := f.env.Do(http.MethodPost, loPath(f.l2, "import"), f.aTok, map[string]any{"gear_ids": []int64{f.g100, skipped}})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500; body %s", w.Code, w.Body)
	}
	if got := f.env.snapshot(); got != before {
		t.Fatalf("import with a skipped insert left effects:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}
