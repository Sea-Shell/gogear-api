package endpoints

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Adversarial regression tests for BR-020 (container nesting): any registration
// can hold any other at any depth, a registration has at most one direct
// container, and nothing is ever inside itself, also under concurrency. All
// through the real JWT middleware and a migrated database.

type nestWorld struct {
	env     *authzEnv
	alice   int64
	bob     int64
	tokA    string
	tokB    string
	plain   int64 // catalog gear with gearIsContainer = 0
	boxGear int64 // catalog gear with gearIsContainer = 1
}

func newNestWorld(t *testing.T) *nestWorld {
	env := newAuthzEnv(t)
	w := &nestWorld{env: env}
	w.alice = env.SeedUser("alice", "alice@example.com", false)
	w.bob = env.SeedUser("bob", "bob@example.com", false)
	w.tokA, w.tokB = env.Token(w.alice, false), env.Token(w.bob, false)
	w.plain = env.seedGearX(false, false, 1, 1)
	w.boxGear = env.seedGearX(true, false, 1, 1)
	return w
}

// link asks the API to put content inside container as alice.
func (w *nestWorld) link(container, content int64) *httptest.ResponseRecorder {
	return w.linkAs(w.tokA, container, content)
}

func (w *nestWorld) linkAs(tok string, container, content int64) *httptest.ResponseRecorder {
	return w.env.Do(http.MethodPut, "/api/v1/container/insert", tok,
		map[string]any{"user_container_id": container, "user_gear_registration_id": content})
}

// regs seeds n plain-gear registrations for alice.
func (w *nestWorld) regs(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = w.env.SeedRegistration(w.alice, w.plain)
	}
	return out
}

// directContents lists what the API reports as directly inside container.
func (w *nestWorld) directContents(t *testing.T, container int64) []int64 {
	t.Helper()
	r := w.env.Do(http.MethodGet, ctList(container, "limit=100"), w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ := page(t, r)
	return regIDs(items)
}

// storedViolations walks every stored link and reports a rule it breaks:
// self-link, several direct containers, a cycle, or links across owners.
func (e *authzEnv) storedViolations() string {
	e.T.Helper()
	parent := map[int64]int64{}
	rows, err := e.DB.Query(`SELECT userContainerId, userGearRegistrationId FROM user_container_registration`)
	if err != nil {
		e.T.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c, x int64
		if err := rows.Scan(&c, &x); err != nil {
			e.T.Fatal(err)
		}
		if c == x {
			return fmt.Sprintf("self-link on %d", x)
		}
		if p, dup := parent[x]; dup {
			return fmt.Sprintf("registration %d has containers %d and %d", x, p, c)
		}
		parent[x] = c
	}
	for x := range parent {
		seen := map[int64]bool{x: true}
		for cur, ok := parent[x]; ok; cur, ok = parent[cur] {
			if seen[cur] {
				return fmt.Sprintf("registration %d is inside itself", x)
			}
			seen[cur] = true
		}
	}
	var cross int
	if err := e.DB.QueryRow(`SELECT COUNT(*) FROM user_container_registration l
		JOIN user_gear_registrations c ON c.userGearRegistrationId = l.userContainerId
		JOIN user_gear_registrations x ON x.userGearRegistrationId = l.userGearRegistrationId
		WHERE c.userId <> x.userId`).Scan(&cross); err != nil {
		e.T.Fatal(err)
	}
	if cross > 0 {
		return "link across owners"
	}
	return ""
}

// AC-1: the catalog flag neither allows nor prevents a role.
func TestContainerNesting_AnyRegistrationCanContainAnyOther(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	plainA, plainB := e.SeedRegistration(w.alice, w.plain), e.SeedRegistration(w.alice, w.plain)
	flagged := e.SeedRegistration(w.alice, w.boxGear)
	flagged2 := e.SeedRegistration(w.alice, w.boxGear)

	expectStatus(t, w.link(plainA, plainB), http.StatusOK)    // non-container gear holds non-container gear
	expectStatus(t, w.link(plainA, flagged), http.StatusOK)   // ... and container gear
	expectStatus(t, w.link(flagged, flagged2), http.StatusOK) // container gear holds container gear
	// content that itself holds others, and non-container gear that sits inside
	// container gear and holds items:
	expectStatus(t, w.link(flagged2, e.SeedRegistration(w.alice, w.plain)), http.StatusOK)
	if got := w.directContents(t, plainA); !sameIDs(got, []int64{plainB, flagged}) {
		t.Fatalf("contents of plain container = %v", got)
	}
	if v := e.storedViolations(); v != "" {
		t.Fatal(v)
	}
}

// AC-2: no depth limit. A chain of 25, every link succeeds, each container
// lists exactly its direct content; deleting a middle registration leaves its
// child uncontained and the rest of the chain intact (AC-7).
func TestContainerNesting_DeepChainAndDeleteMiddle(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	r := w.regs(25)
	for i := 0; i+1 < len(r); i++ {
		if got := w.link(r[i], r[i+1]); got.Code != http.StatusOK {
			t.Fatalf("link %d into %d: %d %s", r[i+1], r[i], got.Code, got.Body.String())
		}
	}
	for i := 0; i+1 < len(r); i++ {
		if got := w.directContents(t, r[i]); !sameIDs(got, []int64{r[i+1]}) {
			t.Fatalf("contents of level %d = %v, want [%d]", i, got, r[i+1])
		}
	}
	if got := w.directContents(t, r[24]); len(got) != 0 {
		t.Fatalf("leaf holds %v", got)
	}
	// The deepest registration can still not be put around the top.
	expectStatus(t, w.link(r[24], r[0]), http.StatusConflict)

	mid := r[12]
	expectStatus(t, e.Do(http.MethodDelete, ugPath(mid, "delete"), w.tokA, nil), http.StatusOK)
	if n := e.Count("user_container_registration", "userGearRegistrationId = ?", r[13]); n != 0 {
		t.Fatalf("the deleted container's child still has %d container links", n)
	}
	if n := e.Count("user_gear_registrations", "userGearRegistrationId = ?", r[13]); n != 1 {
		t.Fatal("the child registration was removed with its container")
	}
	for i := 13; i+1 < len(r); i++ { // the chain below the deleted one is intact
		if e.Count("user_container_registration", "userContainerId = ? AND userGearRegistrationId = ?", r[i], r[i+1]) != 1 {
			t.Fatalf("link %d->%d lost", r[i], r[i+1])
		}
	}
	for i := 0; i < 11; i++ { // and above
		if e.Count("user_container_registration", "userContainerId = ? AND userGearRegistrationId = ?", r[i], r[i+1]) != 1 {
			t.Fatalf("link %d->%d lost", r[i], r[i+1])
		}
	}
	if n := e.Count("user_container_registration", ""); n != 22 { // 24 links minus the two that involved mid
		t.Fatalf("links = %d, want 22", n)
	}
	// With the middle gone, the former top can be put into the former leaf's subtree.
	expectStatus(t, w.link(r[24], r[0]), http.StatusOK)
}

// AC-3: a second direct container is 409 and changes nothing; moving needs the
// old link removed first.
func TestContainerNesting_SecondDirectContainerRefused(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	r := w.regs(3)
	c, d, x := r[0], r[1], r[2]
	expectStatus(t, w.link(c, x), http.StatusOK)
	before := e.ugSnapshot()

	for name, container := range map[string]int64{"another container": d, "the same container again": c} {
		got := w.link(container, x)
		if got.Code != http.StatusConflict {
			t.Fatalf("%s: status %d, want 409; body %s", name, got.Code, got.Body.String())
		}
	}
	if e.ugSnapshot() != before {
		t.Fatal("a refused link changed stored links")
	}

	linkID := e.linkID(c, x)
	expectStatus(t, e.Do(http.MethodDelete, ctDelete(linkID), w.tokA, nil), http.StatusOK)
	expectStatus(t, w.link(d, x), http.StatusOK)
	if e.Count("user_container_registration", "userGearRegistrationId = ?", x) != 1 || e.linkID(d, x) == 0 {
		t.Fatal("move did not leave exactly the new link")
	}
}

// AC-4: indirect cycles at depth 3 and more are refused with 409.
func TestContainerNesting_IndirectCyclesRefused(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	r := w.regs(6) // r0 holds r1 holds r2 holds r3 holds r4 holds r5
	for i := 0; i+1 < len(r); i++ {
		expectStatus(t, w.link(r[i], r[i+1]), http.StatusOK)
	}
	before := e.ugSnapshot()
	for _, tc := range []struct {
		name               string
		container, content int64
	}{
		{"depth 2 ring", r[1], r[0]},
		{"depth 3 ring", r[2], r[0]},
		{"depth 4 ring", r[3], r[0]},
		{"top into bottom", r[5], r[0]},
		{"middle into bottom", r[5], r[2]},
		{"middle into its own grandchild", r[3], r[1]},
	} {
		got := w.link(tc.container, tc.content)
		if got.Code != http.StatusConflict {
			t.Errorf("%s: status %d, want 409; body %s", tc.name, got.Code, got.Body.String())
		}
	}
	if e.ugSnapshot() != before {
		t.Fatal("a refused link changed stored links")
	}
	// A registration may still be put into an unrelated branch.
	other := w.regs(1)[0]
	expectStatus(t, w.link(other, r[0]), http.StatusOK) // the whole chain moves into `other`
	if v := e.storedViolations(); v != "" {
		t.Fatal(v)
	}
}

// AC-4/AC-5: a self-link is 400 and is decided before any 409; ownership (404)
// before both, with no hint about another user's links.
func TestContainerNesting_StatusPrecedence(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	r := w.regs(3)
	expectStatus(t, w.link(r[0], r[1]), http.StatusOK)
	expectStatus(t, w.link(r[1], r[2]), http.StatusOK)

	// Self-link of a registration that already has a container: still 400.
	expectStatus(t, w.link(r[1], r[1]), http.StatusBadRequest)
	expectStatus(t, w.link(r[2], r[2]), http.StatusBadRequest)

	// Bob's chain: Alice gets one identical 404 whether the request would be a
	// cycle, a second container, a self-link or an unknown ID.
	b := []int64{e.SeedRegistration(w.bob, w.plain), e.SeedRegistration(w.bob, w.plain), e.SeedRegistration(w.bob, w.plain)}
	e.SeedContainerLink(b[0], b[1])
	e.SeedContainerLink(b[1], b[2])
	want := w.link(99998, 99999)
	expectStatus(t, want, http.StatusNotFound)
	for name, c := range map[string][2]int64{
		"cycle in bob's chain":      {b[2], b[0]},
		"second container of bob's": {b[0], b[2]},
		"bob self-link":             {b[1], b[1]},
		"mixed owners":              {r[0], b[0]},
	} {
		got := w.link(c[0], c[1])
		if got.Code != http.StatusNotFound || got.Body.String() != want.Body.String() {
			t.Errorf("%s: %d %s, want the identical 404", name, got.Code, got.Body.String())
		}
	}
}

// AC-6: concurrent requests can never store a cycle or a second container.
func TestContainerNesting_ConcurrentRingFormingInsertsStoreAtMostOne(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	for round := 0; round < 25; round++ {
		pair := w.regs(2)
		a, b := pair[0], pair[1]
		codes := make([]int, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i, pair := range [][2]int64{{a, b}, {b, a}} { // A into B, B into A
			wg.Add(1)
			go func(i int, pair [2]int64) {
				defer wg.Done()
				<-start
				codes[i] = w.link(pair[0], pair[1]).Code
			}(i, pair)
		}
		close(start)
		wg.Wait()
		ok, conflict := 0, 0
		for _, c := range codes {
			switch c {
			case http.StatusOK:
				ok++
			case http.StatusConflict:
				conflict++
			default:
				t.Fatalf("round %d: unexpected status %v", round, codes)
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatalf("round %d: statuses %v, want exactly one 200 and one 409", round, codes)
		}
		if n := e.Count("user_container_registration", "userContainerId IN (?, ?) AND userGearRegistrationId IN (?, ?)", a, b, a, b); n != 1 {
			t.Fatalf("round %d: %d links stored between the pair, want 1", round, n)
		}
	}
	if v := e.storedViolations(); v != "" {
		t.Fatal(v)
	}
}

// A longer ring formed by three concurrent requests: at most two succeed.
func TestContainerNesting_ConcurrentThreeRingStoresNoCycle(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	for round := 0; round < 15; round++ {
		r := []int64{e.SeedRegistration(w.alice, w.plain), e.SeedRegistration(w.alice, w.plain), e.SeedRegistration(w.alice, w.plain)}
		var wg sync.WaitGroup
		start := make(chan struct{})
		var mu sync.Mutex
		okCount := 0
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				if w.link(r[i], r[(i+1)%3]).Code == http.StatusOK {
					mu.Lock()
					okCount++
					mu.Unlock()
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if okCount != 2 {
			t.Fatalf("round %d: %d of 3 ring links succeeded, want exactly 2", round, okCount)
		}
	}
	if v := e.storedViolations(); v != "" {
		t.Fatal(v)
	}
}

// AC-6: X into C and X into D at the same time leave X with one container.
func TestContainerNesting_ConcurrentSecondContainerStoresOne(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	for round := 0; round < 25; round++ {
		r := w.regs(3)
		c, d, x := r[0], r[1], r[2]
		codes := make([]int, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i, container := range []int64{c, d} {
			wg.Add(1)
			go func(i int, container int64) {
				defer wg.Done()
				<-start
				codes[i] = w.link(container, x).Code
			}(i, container)
		}
		close(start)
		wg.Wait()
		if n := e.Count("user_container_registration", "userGearRegistrationId = ?", x); n != 1 {
			t.Fatalf("round %d: statuses %v, %d links stored for x, want 1", round, codes, n)
		}
		if !(codes[0]+codes[1] == http.StatusOK+http.StatusConflict) {
			t.Fatalf("round %d: statuses %v, want one 200 and one 409", round, codes)
		}
	}
}

// The database enforces the rules without the handler (second connection, a
// handler that forgot a check): the unique index and the cycle trigger refuse.
func TestContainerNesting_DatabaseRefusesViolationsOnItsOwn(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	r := w.regs(4)
	e.SeedContainerLink(r[0], r[1])
	e.SeedContainerLink(r[1], r[2])
	e.SeedContainerLink(r[2], r[3])

	insert := `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (?, ?)`
	if _, err := e.DB.Exec(insert, r[3], r[0]); err == nil {
		t.Error("the database stored a depth-4 cycle")
	}
	if _, err := e.DB.Exec(insert, r[2], r[0]); err == nil {
		t.Error("the database stored a depth-3 cycle")
	}
	if _, err := e.DB.Exec(insert, r[3], r[1]); err == nil {
		t.Error("the database stored a second direct container")
	}
	if _, err := e.DB.Exec(insert, r[1], r[1]); err == nil {
		t.Error("the database stored a self-link")
	}
	// Re-pointing an existing link so that it closes a ring is refused as well:
	// moving r1 into r3 would make r3 hold r1, r1 hold r2 and r2 hold r3.
	free := e.SeedRegistration(w.alice, w.plain)
	if _, err := e.DB.Exec(`UPDATE user_container_registration SET userContainerId = ? WHERE containerRegistrationId = ?`, r[3], e.linkID(r[0], r[1])); err == nil {
		t.Error("the database accepted an update that closes a ring")
	}
	if v := e.storedViolations(); v != "" {
		t.Fatal(v)
	}
	// Valid changes still work.
	if _, err := e.DB.Exec(insert, free, r[0]); err != nil {
		t.Errorf("a valid link was refused: %v", err)
	}
}

// Deleting a registration leaves its contents in the inventory, uncontained,
// with whatever they hold still inside them; cross-checked through the list API.
func TestContainerNesting_DeletedContainerLeavesContentsUncontained(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	r := w.regs(4)
	box, a, b, deep := r[0], r[1], r[2], r[3]
	expectStatus(t, w.link(box, a), http.StatusOK)
	expectStatus(t, w.link(box, b), http.StatusOK)
	expectStatus(t, w.link(a, deep), http.StatusOK)
	expectStatus(t, e.Do(http.MethodDelete, ugPath(box, "delete"), w.tokA, nil), http.StatusOK)

	resp := e.Do(http.MethodGet, ugList(w.alice, "limit=100"), w.tokA, nil)
	expectStatus(t, resp, http.StatusOK)
	items, _ := page(t, resp)
	byID := map[int64]map[string]any{}
	for _, it := range items {
		byID[int64(it["usergear_registration_id"].(float64))] = it
	}
	if len(byID) != 3 {
		t.Fatalf("remaining registrations = %v, want a, b, deep", regIDs(items))
	}
	for _, id := range []int64{a, b} {
		if byID[id]["container_registration_id"] != nil {
			t.Errorf("registration %d still reports a container: %v", id, byID[id]["container_registration_id"])
		}
	}
	if got := byID[deep]["container_registration_id"]; got != float64(a) {
		t.Errorf("deep content lost its container: %v, want %d", got, a)
	}
	// The freed registration can now be put into another container.
	expectStatus(t, w.link(deep, b), http.StatusOK)
}

// The list filter `container` keeps meaning "catalog container gear"; nesting
// does not depend on it, and a registration inside a container still lists
// once with its container.
func TestContainerNesting_ListFilterIsStillTheCatalogFlag(t *testing.T) {
	w := newNestWorld(t)
	e := w.env
	flagged := e.SeedRegistration(w.alice, w.boxGear)
	plainBox := e.SeedRegistration(w.alice, w.plain)
	inside := e.SeedRegistration(w.alice, w.plain)
	expectStatus(t, w.link(plainBox, inside), http.StatusOK)
	expectStatus(t, w.link(flagged, plainBox), http.StatusOK)

	ids := func(q string) []int64 {
		r := e.Do(http.MethodGet, ugList(w.alice, q), w.tokA, nil)
		expectStatus(t, r, http.StatusOK)
		items, _ := page(t, r)
		return regIDs(items)
	}
	if got := ids("container=true"); !sameIDs(got, []int64{flagged}) {
		t.Errorf("container=true = %v, want only the flagged gear %d", got, flagged)
	}
	if got := ids("container=false"); !sameIDs(got, []int64{plainBox, inside}) {
		t.Errorf("container=false = %v, want %v", got, []int64{plainBox, inside})
	}
	if got := ids("limit=100"); !sameIDs(got, []int64{flagged, plainBox, inside}) {
		t.Errorf("unfiltered = %v", got)
	}
	// The registration of a non-container gear that holds items lists its contents.
	if got := w.directContents(t, plainBox); !sameIDs(got, []int64{inside}) {
		t.Errorf("contents of a non-container gear = %v", got)
	}
}
