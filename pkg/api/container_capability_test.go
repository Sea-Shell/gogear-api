package endpoints

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Regression tests for BR-021 (the catalog category decides whether gear can be
// a container) and amended BR-020 AC-1/AC-5 (ADR-001). Settings are seeded by
// SQL here (the admin API for them is a later step); everything else goes
// through the real JWT middleware and a migrated database.

const cannotHold = "cannot hold items"

type capWorld struct {
	env            *authzEnv
	alice, bob     int64
	tokA, tokB     string
	topYes, topNo  int64
	gears          map[string]int64 // name -> catalog gear ID
	want           map[string]bool  // name -> expected container status
	cats           map[string]int64
	registrations  map[string]int64 // gear name -> alice's registration
	capableForLink int64            // alice's registration of a capable gear, to hold the others
}

// newCapWorld seeds the catalog so that each effective-setting case of BR-021
// AC-1 occurs, plus gear whose own top-category reference disagrees with its
// sub-category's (which must be ignored, AC-3).
//
//	topYes: container, topNo: not
//	capExplicit (topNo, explicit container)   -> gear explicitYes   capable
//	capInherit  (topYes, no setting)          -> gear inheritYes    capable
//	capOverride (topYes, explicit false)      -> gear overrideNo    not
//	capNone     (topNo, no setting)           -> gear inheritNo     not
//	gear refYesButNot: sub-category capNone, own top reference topYes -> not
//	gear refNoButCapable: sub-category capInherit, own top reference topNo -> capable
func newCapWorld(t *testing.T) *capWorld {
	env := newAuthzEnv(t)
	w := &capWorld{env: env, gears: map[string]int64{}, want: map[string]bool{}, cats: map[string]int64{}, registrations: map[string]int64{}}
	w.alice = env.SeedUser("alice", "alice@example.com", false)
	w.bob = env.SeedUser("bob", "bob@example.com", false)
	w.tokA, w.tokB = env.Token(w.alice, false), env.Token(w.bob, false)
	w.topYes, w.topNo = env.seedTopCategory("Bags"), env.seedTopCategory("Clothing")
	env.setTopContainer(w.topYes, true)

	cat := func(name string, top int64, explicit *bool) {
		res, err := env.DB.Exec(`INSERT INTO gear_category (categoryTopCategoryId, categoryName) VALUES (?, ?)`, top, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		env.setCategoryContainer(id, explicit)
		w.cats[name] = id
	}
	cat("capExplicit", w.topNo, boolPtr(true))
	cat("capInherit", w.topYes, nil)
	cat("capOverride", w.topYes, boolPtr(false))
	cat("capNone", w.topNo, nil)

	gear := func(name, category string, topRef int64, want bool) {
		res, err := env.DB.Exec(`INSERT INTO gear (gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight, gearHeight, gearLength, gearWidth, gearStatus)
			VALUES (?, ?, 1, ?, 100, 1, 2, 3, 1)`, topRef, w.cats[category], "capw-"+name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		w.gears[name], w.want[name] = id, want
	}
	gear("explicitYes", "capExplicit", w.topNo, true)
	gear("inheritYes", "capInherit", w.topYes, true)
	gear("overrideNo", "capOverride", w.topYes, false)
	gear("inheritNo", "capNone", w.topNo, false)
	gear("refYesButNot", "capNone", w.topYes, false)
	gear("refNoButCapable", "capInherit", w.topNo, true)

	for name, g := range w.gears {
		w.registrations[name] = env.SeedRegistration(w.alice, g)
	}
	w.capableForLink = w.registrations["explicitYes"]
	return w
}

// link puts content inside container as alice.
func (w *capWorld) link(container, content int64) *httptest.ResponseRecorder {
	return w.env.Do(http.MethodPut, "/api/v1/container/insert", w.tokA,
		map[string]any{"user_container_id": container, "user_gear_registration_id": content})
}

func (w *capWorld) wantIDs(m map[string]int64, status bool) []int64 {
	var out []int64
	for n, id := range m {
		if w.want[n] == status {
			out = append(out, id)
		}
	}
	return out
}

func sameSet(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	m := map[int64]int{}
	for _, g := range got {
		m[g]++
	}
	for _, x := range want {
		m[x]--
	}
	for _, c := range m {
		if c != 0 {
			return false
		}
	}
	return true
}

func gearIDsOf(items []map[string]any) []int64 {
	var ids []int64
	for _, it := range items {
		ids = append(ids, int64(it["gear_id"].(float64)))
	}
	return ids
}

// checkReported asserts that every endpoint that reports gear_is_container
// reports exactly w.want, for the gear list, search and get, the registration
// list and get, and the container contents.
func (w *capWorld) checkReported(t *testing.T, label string) {
	t.Helper()
	e := w.env
	byGear := func(items []map[string]any) map[int64]bool {
		m := map[int64]bool{}
		for _, it := range items {
			m[int64(it["gear_id"].(float64))] = it["gear_is_container"].(bool)
		}
		return m
	}
	check := func(where string, got map[int64]bool) {
		t.Helper()
		for name, g := range w.gears {
			v, ok := got[g]
			if !ok {
				t.Errorf("%s: %s: gear %s missing from the response", label, where, name)
				continue
			}
			if v != w.want[name] {
				t.Errorf("%s: %s: gear %s reports gear_is_container=%v, want %v", label, where, name, v, w.want[name])
			}
		}
	}

	r := e.Do(http.MethodGet, "/api/v1/gear/list?limit=100", w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ := page(t, r)
	check("gear list", byGear(items))

	r = e.Do(http.MethodGet, "/api/v1/gear/search?searchString=capw-&searchType=startswith&limit=100", w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ = page(t, r)
	check("gear search", byGear(items))

	for name, g := range w.gears {
		r = e.Do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", g), w.tokA, nil)
		expectStatus(t, r, http.StatusOK)
		var one map[string]any
		decodeJSON(t, r, &one)
		if one["gear_is_container"] != w.want[name] {
			t.Errorf("%s: gear get: gear %s reports %v, want %v", label, name, one["gear_is_container"], w.want[name])
		}
	}

	r = e.Do(http.MethodGet, ugList(w.alice, "limit=100"), w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ = page(t, r)
	check("registration list", byGear(items))

	for name, reg := range w.registrations {
		r = e.Do(http.MethodGet, ugPath(reg, "get"), w.tokA, nil)
		expectStatus(t, r, http.StatusOK)
		var one map[string]any
		decodeJSON(t, r, &one)
		if one["gear_is_container"] != w.want[name] {
			t.Errorf("%s: registration get: gear %s reports %v, want %v", label, name, one["gear_is_container"], w.want[name])
		}
	}
}

// BR-021 AC-1, AC-3, AC-4: the status every endpoint reports is the effective
// setting of the gear's sub-category (explicit, else the top category's, else
// no), whatever the gear's own top-category reference says, and it follows
// every catalog change at once (the three AC-1 examples are explicitYes,
// inheritYes and overrideNo).
func TestContainerCapability_ReportedStatusIsTheEffectiveSetting(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	w.checkReported(t, "initial")

	// Contents of a container report the content's own status.
	contents := []string{"inheritYes", "overrideNo", "inheritNo", "refNoButCapable"}
	for _, n := range contents {
		expectStatus(t, w.link(w.capableForLink, w.registrations[n]), http.StatusOK)
	}
	r := e.Do(http.MethodGet, ctList(w.capableForLink, "limit=100"), w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ := page(t, r)
	if len(items) != len(contents) {
		t.Fatalf("container contents = %v", regIDs(items))
	}
	for _, it := range items {
		var name string
		for n, g := range w.gears {
			if int64(it["gear_id"].(float64)) == g {
				name = n
			}
		}
		if it["gear_is_container"] != w.want[name] {
			t.Errorf("container contents: gear %s reports %v, want %v", name, it["gear_is_container"], w.want[name])
		}
	}

	// A top category stops being a container: inherited gear follows, the
	// explicit sub-category setting does not.
	e.setTopContainer(w.topYes, false)
	w.want["inheritYes"], w.want["refNoButCapable"] = false, false
	w.checkReported(t, "top category no longer a container")

	// The explicit "not a container" is removed: the sub-category falls back to its top.
	e.setTopContainer(w.topYes, true)
	e.setCategoryContainer(w.cats["capOverride"], nil)
	w.want["inheritYes"], w.want["refNoButCapable"], w.want["overrideNo"] = true, true, true
	w.checkReported(t, "explicit setting removed")

	// A sub-category moves to another top category.
	if _, err := e.DB.Exec(`UPDATE gear_category SET categoryTopCategoryId = ? WHERE categoryId = ?`, w.topNo, w.cats["capInherit"]); err != nil {
		t.Fatal(err)
	}
	w.want["inheritYes"], w.want["refNoButCapable"] = false, false
	w.checkReported(t, "sub-category moved to a non-container top")

	// A gear moves to another sub-category.
	if _, err := e.DB.Exec(`UPDATE gear SET gearCategoryId = ? WHERE gearId = ?`, w.cats["capExplicit"], w.gears["inheritNo"]); err != nil {
		t.Fatal(err)
	}
	w.want["inheritNo"] = true
	w.checkReported(t, "gear moved to a container sub-category")
}

// BR-021 AC-7 / P-16: container=true|false select exactly by the derived
// status, in the items and in the total; other values are unchanged (gear 400,
// registrations ignore them).
func TestContainerCapability_FiltersSelectByDerivedStatus(t *testing.T) {
	w := newCapWorld(t)
	e := w.env

	verify := func(label string) {
		t.Helper()
		for _, tc := range []struct {
			q      string
			status bool
		}{{"container=true", true}, {"container=false", false}} {
			wantGear, wantRegs := w.wantIDs(w.gears, tc.status), w.wantIDs(w.registrations, tc.status)

			r := e.Do(http.MethodGet, "/api/v1/gear/list?limit=100&"+tc.q, w.tokA, nil)
			expectStatus(t, r, http.StatusOK)
			items, total := page(t, r)
			// The gear list also holds the base catalog's gear (none): only the seeded ones.
			if !sameSet(gearIDsOf(items), wantGear) || total != len(wantGear) {
				t.Errorf("%s: gear list %s = %v total %d, want %v", label, tc.q, gearIDsOf(items), total, wantGear)
			}

			r = e.Do(http.MethodGet, ugList(w.alice, "limit=100&"+tc.q), w.tokA, nil)
			expectStatus(t, r, http.StatusOK)
			items, total = page(t, r)
			if !sameSet(regIDs(items), wantRegs) || total != len(wantRegs) {
				t.Errorf("%s: registration list %s = %v total %d, want %v", label, tc.q, regIDs(items), total, wantRegs)
			}

			// Paging does not break the count: one item per page, the total stays.
			r = e.Do(http.MethodGet, "/api/v1/gear/list?limit=1&"+tc.q, w.tokA, nil)
			expectStatus(t, r, http.StatusOK)
			if _, total = page(t, r); total != len(wantGear) {
				t.Errorf("%s: gear list %s limit=1 total %d, want %d", label, tc.q, total, len(wantGear))
			}
		}
	}
	verify("initial")
	e.setTopContainer(w.topYes, false)
	w.want["inheritYes"], w.want["refNoButCapable"] = false, false
	verify("after the top category lost its setting")
	e.setCategoryContainer(w.cats["capNone"], boolPtr(true))
	w.want["inheritNo"], w.want["refYesButNot"] = true, true
	verify("after a sub-category was set")

	// Other values are unchanged: gear refuses them, registrations ignore them.
	expectStatus(t, e.Do(http.MethodGet, "/api/v1/gear/list?container=maybe", w.tokA, nil), http.StatusBadRequest)
	r := e.Do(http.MethodGet, ugList(w.alice, "limit=100&container=maybe"), w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	if items, total := page(t, r); len(items) != len(w.registrations) || total != len(w.registrations) {
		t.Errorf("container=maybe on registrations = %d items, total %d, want all %d", len(items), total, len(w.registrations))
	}
}

// BR-020 AC-1/AC-5, P-17: a link into a registration whose gear is not
// container-capable is 409 with a message about the caller's own registration,
// and changes nothing. Any registration may be content.
func TestContainerCapability_LinkIntoNotCapableIs409AndChangesNothing(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	plainBox := w.registrations["inheritNo"]
	content := w.registrations["overrideNo"]
	before := e.ugSnapshot()

	got := w.link(plainBox, content)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) {
		t.Fatalf("status %d body %s, want 409 with %q", got.Code, got.Body.String(), cannotHold)
	}
	// The same for gear whose sub-category setting is an explicit "no", and for
	// gear whose own top-category reference says "container" (ignored, AC-3).
	for _, n := range []string{"overrideNo", "refYesButNot"} {
		got = w.link(w.registrations[n], w.registrations["inheritYes"])
		if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) {
			t.Errorf("%s as container: status %d body %s, want 409 with %q", n, got.Code, got.Body.String(), cannotHold)
		}
	}
	if e.ugSnapshot() != before {
		t.Fatal("a refused link changed stored data")
	}

	// A capable container accepts any registration as content, capable or not.
	expectStatus(t, w.link(w.capableForLink, content), http.StatusOK)
	expectStatus(t, w.link(w.capableForLink, w.registrations["inheritYes"]), http.StatusOK)
	// A registration that is itself capable but sits inside another container can hold items too.
	expectStatus(t, w.link(w.registrations["inheritYes"], w.registrations["inheritNo"]), http.StatusOK)
}

// BR-018 / BR-020 AC-5 / D-9: 401 and 400 for a malformed body, then 404 for
// anything not the caller's, then 400 for a self-link, then the not-capable 409,
// and only then the other 409s. The not-capable 409 never reveals another
// user's data.
func TestContainerCapability_StatusPrecedence(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	plain := w.registrations["inheritNo"]
	box := w.capableForLink
	x := w.registrations["overrideNo"]
	bobPlain := e.SeedRegistration(w.bob, w.gears["inheritNo"])
	bobBox := e.SeedRegistration(w.bob, w.gears["explicitYes"])

	// 404: a foreign or unknown registration, even when the container is not capable.
	notFound := w.link(bobPlain, x)
	expectStatus(t, notFound, http.StatusNotFound)
	for name, l := range map[string][2]int64{
		"foreign non-capable container": {bobPlain, x},
		"foreign capable container":     {bobBox, x},
		"foreign content":               {plain, bobPlain},
		"unknown container":             {99999, x},
		"unknown content":               {plain, 99998},
	} {
		got := w.link(l[0], l[1])
		if got.Code != http.StatusNotFound || got.Body.String() != notFound.Body.String() {
			t.Errorf("%s: %d %s, want the identical 404", name, got.Code, got.Body.String())
		}
	}
	// 400: a self-link of a registration that cannot hold items.
	expectStatus(t, w.link(plain, plain), http.StatusBadRequest)

	// not-capable 409 beats "already in another container" (BR-020 AC-5 example).
	expectStatus(t, w.link(box, x), http.StatusOK)
	got := w.link(plain, x)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) || strings.Contains(got.Body.String(), "another container") {
		t.Errorf("not-capable vs another container: %d %s", got.Code, got.Body.String())
	}
	// ... and beats "already in this container" and the cycle 409.
	e.setCategoryContainer(w.cats["capExplicit"], boolPtr(false))
	got = w.link(box, x) // x is already in box, but box can no longer hold items
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) {
		t.Errorf("not-capable vs same container: %d %s", got.Code, got.Body.String())
	}
	got = w.link(x, box) // would be a cycle, but x cannot hold items either
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) {
		t.Errorf("not-capable vs cycle: %d %s", got.Code, got.Body.String())
	}
	// With a capable container the other 409s are unchanged.
	e.setCategoryContainer(w.cats["capExplicit"], boolPtr(true))
	got = w.link(box, x)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "already in this container") {
		t.Errorf("same container: %d %s", got.Code, got.Body.String())
	}
	other := e.SeedRegistration(w.alice, w.gears["inheritYes"])
	got = w.link(other, x)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "another container") {
		t.Errorf("another container: %d %s", got.Code, got.Body.String())
	}
	got = w.link(x, box)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) {
		t.Errorf("x cannot hold items: %d %s", got.Code, got.Body.String())
	}
	w2 := e.SeedRegistration(w.alice, w.gears["refNoButCapable"])
	expectStatus(t, w.link(box, w2), http.StatusOK)
	got = w.link(w2, box) // w2 is capable and inside box: a real cycle
	if got.Code != http.StatusConflict || strings.Contains(got.Body.String(), cannotHold) {
		t.Errorf("cycle: %d %s, want the cycle 409", got.Code, got.Body.String())
	}
	if v := e.storedViolations(); v != "" {
		t.Fatal(v)
	}
}

// BR-021 AC-6 / P-15: a link that exists stays listed and can be removed after
// its container loses capability; a new link into it is 409.
func TestContainerCapability_LossOfCapabilityKeepsExistingLinks(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	box := w.capableForLink
	a, b := w.registrations["overrideNo"], w.registrations["inheritNo"]
	expectStatus(t, w.link(box, a), http.StatusOK)

	e.setCategoryContainer(w.cats["capExplicit"], boolPtr(false))
	r := e.Do(http.MethodGet, ctList(box, ""), w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	items, _ := page(t, r)
	if !sameIDs(regIDs(items), []int64{a}) {
		t.Fatalf("contents after the container lost capability = %v, want [%d]", regIDs(items), a)
	}
	got := w.link(box, b)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), cannotHold) {
		t.Fatalf("new link into the former container: %d %s", got.Code, got.Body.String())
	}
	if e.Count("user_container_registration", "userContainerId = ?", box) != 1 {
		t.Fatal("the refused link was stored, or the existing one lost")
	}
	// The registration still reports the new status.
	r = e.Do(http.MethodGet, ugPath(box, "get"), w.tokA, nil)
	expectStatus(t, r, http.StatusOK)
	var one map[string]any
	decodeJSON(t, r, &one)
	if one["gear_is_container"] != false {
		t.Errorf("former container reports gear_is_container=%v", one["gear_is_container"])
	}
	// The existing link can be removed.
	expectStatus(t, e.Do(http.MethodDelete, ctDelete(e.linkID(box, a)), w.tokA, nil), http.StatusOK)
	if e.Count("user_container_registration", "userContainerId = ?", box) != 0 {
		t.Fatal("link not removed")
	}
}

// BR-021 AC-8: a setting change and link requests into that container run at
// the same time, many times over. Once the change is committed, no link into
// the now not-capable container is stored (every request that started after
// the commit is 409), and the number of stored links equals the number of 200s.
func TestContainerCapability_SettingChangeVersusConcurrentLinks(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	const rounds, workers, perWorker = 40, 4, 40
	totalOK, totalRefusedAfter := 0, 0

	for round := 0; round < rounds; round++ {
		category := e.seedContainerCategory(w.topNo)
		res, err := e.DB.Exec(`INSERT INTO gear (gearTopCategoryId, gearCategoryId, gearManufactureId, gearName, gearWeight) VALUES (?, ?, 1, 'race', 1)`, w.topNo, category)
		if err != nil {
			t.Fatal(err)
		}
		gearID, _ := res.LastInsertId()
		box := e.SeedRegistration(w.alice, gearID)
		contents := make([][]int64, workers)
		for i := range contents {
			for j := 0; j < perWorker; j++ {
				contents[i] = append(contents[i], e.SeedRegistration(w.alice, w.gears["inheritNo"]))
			}
		}

		var changed atomic.Bool
		var completed atomic.Int64
		var okCount, afterCount, refusedAfter atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(mine []int64) {
				defer wg.Done()
				<-start
				sentAfter := 0
				for _, content := range mine {
					after := changed.Load() // read before the request starts
					got := w.link(box, content)
					completed.Add(1)
					switch {
					case got.Code == http.StatusOK:
						okCount.Add(1)
						if after {
							t.Errorf("round %d: link %d into the container was accepted after the setting change was committed", round, content)
						}
					case got.Code == http.StatusConflict && strings.Contains(got.Body.String(), cannotHold):
						if after {
							refusedAfter.Add(1)
						}
					default:
						t.Errorf("round %d: status %d %s", round, got.Code, got.Body.String())
					}
					if after {
						afterCount.Add(1)
						if sentAfter++; sentAfter >= 3 {
							return
						}
					}
				}
			}(contents[i])
		}
		wg.Add(1)
		go func() { // the second connection: an admin changes the setting
			defer wg.Done()
			<-start
			deadline := time.Now().Add(10 * time.Second)
			for completed.Load() < 1 && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if _, err := e.DB.Exec(`UPDATE gear_category SET categoryIsContainer = 0 WHERE categoryId = ?`, category); err != nil {
				t.Errorf("round %d: setting change: %v", round, err)
			}
			changed.Store(true)
		}()
		close(start)
		wg.Wait()

		if stored := int64(e.Count("user_container_registration", "userContainerId = ?", box)); stored != okCount.Load() {
			t.Fatalf("round %d: %d links stored, %d requests answered 200", round, stored, okCount.Load())
		}
		totalOK += int(okCount.Load())
		totalRefusedAfter += int(refusedAfter.Load())
	}
	// The test must really race: links went in before the change and were refused after it.
	if totalOK == 0 || totalRefusedAfter == 0 {
		t.Fatalf("the race never happened: %d links accepted before the change, %d refused after it", totalOK, totalRefusedAfter)
	}
}

// The database repeats the rule without the API: a direct insert or re-point
// into a not-capable container is refused, one into a capable container works
// (ADR-001 D-7).
func TestContainerCapability_DatabaseRefusesNotCapableLinkTargets(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	insert := `INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (?, ?)`
	x := w.registrations["overrideNo"]
	if _, err := e.DB.Exec(insert, w.registrations["inheritNo"], x); err == nil || !strings.Contains(err.Error(), cannotHold) {
		t.Fatalf("direct insert into a not-capable container: %v", err)
	}
	if _, err := e.DB.Exec(insert, w.capableForLink, x); err != nil {
		t.Fatalf("direct insert into a capable container: %v", err)
	}
	if _, err := e.DB.Exec(`UPDATE user_container_registration SET userContainerId = ? WHERE userGearRegistrationId = ?`,
		w.registrations["inheritNo"], x); err == nil || !strings.Contains(err.Error(), cannotHold) {
		t.Fatalf("re-pointing a link into a not-capable container: %v", err)
	}
}

// ADR-001 D-4: gear_is_container is read-only. Creating or updating gear that
// sends it does not store it; the response and later reads report the derived
// status.
func TestContainerCapability_GearWritesCannotSetTheStatus(t *testing.T) {
	w := newCapWorld(t)
	e := w.env
	admin := e.SeedUser("root", "root@example.com", true)
	tok := e.Token(admin, true)

	create := func(category int64, sent bool) map[string]any {
		t.Helper()
		r := e.Do(http.MethodPut, "/api/v1/gear/insert", tok, map[string]any{
			"gear_top_category_id": w.topNo, "gear_category_id": category, "gear_manufacture_id": 1,
			"gear_name": "new", "gear_weight": 5, "gear_is_container": sent,
		})
		expectStatus(t, r, http.StatusOK)
		var out map[string]any
		decodeJSON(t, r, &out)
		return out
	}
	// A client that says "container" for gear in a not-capable category.
	notCap := create(w.cats["capNone"], true)
	if notCap["gear_is_container"] != false {
		t.Errorf("create in a not-capable category reports %v", notCap["gear_is_container"])
	}
	// A client that says "not a container" for gear in a capable category.
	cap := create(w.cats["capExplicit"], false)
	if cap["gear_is_container"] != true {
		t.Errorf("create in a capable category reports %v", cap["gear_is_container"])
	}
	// Reads agree, and nothing was stored under the old name.
	for _, g := range []map[string]any{notCap, cap} {
		id := int64(g["gear_id"].(float64))
		r := e.Do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", id), w.tokA, nil)
		expectStatus(t, r, http.StatusOK)
		var one map[string]any
		decodeJSON(t, r, &one)
		if one["gear_is_container"] != g["gear_is_container"] {
			t.Errorf("get reports %v, create reported %v", one["gear_is_container"], g["gear_is_container"])
		}
		if n := e.Count("gear", "gearId = ? AND gearIsContainerLegacy = 0", id); n != 1 {
			t.Errorf("gear %d stored an old-style flag", id)
		}
	}

	// An update cannot change it either: the field is not an updatable field.
	id := int64(notCap["gear_id"].(float64))
	before := e.catalogSnapshot()
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", id), tok, map[string]any{"gear_is_container": true}), http.StatusBadRequest)
	if e.catalogSnapshot() != before {
		t.Fatal("a refused update changed the catalog")
	}
	expectStatus(t, e.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", id), tok, map[string]any{"gear_name": "renamed", "gear_is_container": true}), http.StatusOK)
	r := e.Do(http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", id), w.tokA, nil)
	var one map[string]any
	decodeJSON(t, r, &one)
	if one["gear_is_container"] != false || one["gear_name"] != "renamed" {
		t.Errorf("after the update: %v", one)
	}
}
