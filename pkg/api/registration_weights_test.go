package endpoints

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Milestone B1: the owner's measured weight with its source (BR-026), the
// closet count (BR-036), and what they do to every loadout that uses the
// registration (BR-029 AC-1 to AC-3, BR-030 AC-1). Everything is made through
// the real routes and the real JWT middleware, on the `board` fixture of
// loadout_full_test.go; SQL only reads stored state back.

// regGet reads a registration as tok and requires want.
func (b *board) regGet(tok string, reg int64, want int) models.UserGear {
	b.env.T.Helper()
	w := b.env.Do(http.MethodGet, ugPath(reg, "get"), tok, nil)
	expectStatus(b.env.T, w, want)
	var g models.UserGear
	if want == http.StatusOK {
		decodeJSON(b.env.T, w, &g)
	}
	return g
}

// regUpdate posts body (a map or a raw JSON string) to the registration's update.
func (b *board) regUpdate(tok string, reg int64, body any, want int) {
	b.env.T.Helper()
	expectStatus(b.env.T, b.env.Do(http.MethodPost, ugPath(reg, "update"), tok, body), want)
}

// measure sets the measured weight and source of a registration (200).
func (b *board) measure(tok string, reg int64, grams int, source string) {
	b.env.T.Helper()
	b.regUpdate(tok, reg, map[string]any{"measured_weight": grams, "measured_weight_source": source}, http.StatusOK)
}

// unmeasure removes the measured weight (200).
func (b *board) unmeasure(tok string, reg int64) {
	b.env.T.Helper()
	b.regUpdate(tok, reg, `{"measured_weight": null, "measured_weight_source": null}`, http.StatusOK)
}

// newestReg is the highest registration ID in tok's list.
func (b *board) newestReg(tok string, owner int64) int64 {
	b.env.T.Helper()
	r := b.env.Do(http.MethodGet, ugList(owner, "limit=100"), tok, nil)
	expectStatus(b.env.T, r, http.StatusOK)
	items, _ := page(b.env.T, r)
	var id int64
	for _, it := range items {
		id = max(id, int64(it["usergear_registration_id"].(float64)))
	}
	return id
}

func (b *board) catalogWeightInDB(gear int64) int64 {
	b.env.T.Helper()
	var w sqlNullInt
	if err := b.env.DB.QueryRow(`SELECT gearWeight FROM gear WHERE gearId = ?`, gear).Scan(&w); err != nil {
		b.env.T.Fatal(err)
	}
	return w.V
}

// sqlNullInt reads an INTEGER column that may be NULL as 0.
type sqlNullInt struct{ V int64 }

func (n *sqlNullInt) Scan(src any) error {
	if src == nil {
		n.V = 0
		return nil
	}
	f, ok := src.(int64)
	if !ok {
		return fmt.Errorf("not an integer: %v", src)
	}
	n.V = f
	return nil
}

func wantWeight(t *testing.T, what string, unit *int64, source string, wantUnit int64, wantSource string) {
	t.Helper()
	if unit == nil || *unit != wantUnit || source != wantSource {
		got := "none"
		if unit != nil {
			got = fmt.Sprint(*unit)
		}
		t.Errorf("%s: unit_weight %s source %q, want %d %q", what, got, source, wantUnit, wantSource)
	}
}

// ---- BR-026 -------------------------------------------------------------------

// BR-026 verification semantics. As A, set 687 g weighed on a registration used
// in two loadouts: both loadouts report 687 g and source weighed on their next
// read; the catalog gear and B's registration of the same gear are unchanged.
// Change to 650 g guess, then remove it: the loadouts report 650 g guess, then
// the catalog weight with source catalog (the reported name of "maker's spec").
func TestMeasuredWeight_SetChangeRemoveAndEveryLoadoutFollows(t *testing.T) {
	b := newBoard(t)
	gear := b.gear("Tent", 700, false)
	reg := b.alicesReg(gear)
	bobsReg := b.register(b.bTok, b.bob, gear)

	l1, l2 := b.newLoadout(b.user, "L1"), b.newLoadout(b.user, "L2")
	i1, i2 := b.addItem(b.user, l1, reg, 1), b.addItem(b.user, l2, reg, 2)

	// Before: the registration reports the catalog weight and no measured weight.
	g := b.regGet(b.user, reg, http.StatusOK)
	if g.MeasuredWeight != nil || g.MeasuredWeightSource != nil || g.ClosetCount != 1 {
		t.Errorf("new registration = measured %v %v, count %d; want none and 1", g.MeasuredWeight, g.MeasuredWeightSource, g.ClosetCount)
	}
	wantWeight(t, "registration before", g.UnitWeight, g.WeightSource, 700, "catalog")

	b.measure(b.user, reg, 687, "weighed")

	g = b.regGet(b.user, reg, http.StatusOK)
	if g.MeasuredWeight == nil || *g.MeasuredWeight != 687 || g.MeasuredWeightSource == nil || *g.MeasuredWeightSource != "weighed" {
		t.Errorf("registration measured = %v %v, want 687 weighed", g.MeasuredWeight, g.MeasuredWeightSource)
	}
	wantWeight(t, "registration", g.UnitWeight, g.WeightSource, 687, "weighed")
	if g.GearWeight == nil || *g.GearWeight != 700 {
		t.Errorf("gear_weight = %v, want the catalog 700 unchanged", g.GearWeight)
	}
	for _, c := range []struct{ lo, item, qty int64 }{{l1, i1, 1}, {l2, i2, 2}} {
		f := b.full(b.user, c.lo)
		it := itemOf(t, f, c.item)
		wantWeight(t, fmt.Sprintf("loadout %d item", c.lo), it.UnitWeight, it.WeightSource, 687, "weighed")
		if it.CatalogWeight == nil || *it.CatalogWeight != 700 || it.MeasuredWeight == nil || *it.MeasuredWeight != 687 ||
			it.MeasuredWeightSource == nil || *it.MeasuredWeightSource != "weighed" || it.ClosetCount == nil || *it.ClosetCount != 1 {
			t.Errorf("loadout %d item = catalog %v measured %v %v count %v", c.lo, it.CatalogWeight, it.MeasuredWeight, it.MeasuredWeightSource, it.ClosetCount)
		}
		if f.Loadout.TotalWeight != 687*c.qty || b.summary(b.user, c.lo).TotalWeight != 687*c.qty {
			t.Errorf("loadout %d total = %d, want %d", c.lo, f.Loadout.TotalWeight, 687*c.qty)
		}
	}
	// The catalog gear and B's registration of the same gear are unchanged (AC-3).
	if got := b.catalogWeightInDB(gear); got != 700 {
		t.Errorf("catalog weight = %d, want 700", got)
	}
	bg := b.regGet(b.bTok, bobsReg, http.StatusOK)
	if bg.MeasuredWeight != nil {
		t.Errorf("another owner's registration got a measured weight: %v", *bg.MeasuredWeight)
	}
	wantWeight(t, "bob's registration", bg.UnitWeight, bg.WeightSource, 700, "catalog")

	b.measure(b.user, reg, 650, "guess")
	it := itemOf(t, b.full(b.user, l1), i1)
	wantWeight(t, "after the change", it.UnitWeight, it.WeightSource, 650, "guess")

	b.unmeasure(b.user, reg)
	g = b.regGet(b.user, reg, http.StatusOK)
	if g.MeasuredWeight != nil || g.MeasuredWeightSource != nil {
		t.Errorf("after removing: measured %v %v, want none", g.MeasuredWeight, g.MeasuredWeightSource)
	}
	wantWeight(t, "registration after removing", g.UnitWeight, g.WeightSource, 700, "catalog")
	for _, c := range []struct{ lo, item int64 }{{l1, i1}, {l2, i2}} {
		it := itemOf(t, b.full(b.user, c.lo), c.item)
		wantWeight(t, fmt.Sprintf("loadout %d after removing", c.lo), it.UnitWeight, it.WeightSource, 700, "catalog")
		if it.MeasuredWeight != nil || it.MeasuredWeightSource != nil {
			t.Errorf("loadout %d item still reports measured %v %v", c.lo, it.MeasuredWeight, it.MeasuredWeightSource)
		}
	}
	if got := b.summary(b.user, l2).TotalWeight; got != 1400 {
		t.Errorf("L2 total after removing = %d, want 1400", got)
	}
	if got := b.catalogWeightInDB(gear); got != 700 {
		t.Errorf("catalog weight = %d at the end, want 700", got)
	}
}

// A measured weight on gear that has no catalog weight: the registration has
// the owner's weight and no catalog weight; removing it leaves no weight.
func TestMeasuredWeight_OnGearWithoutCatalogWeight(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Homemade stove", 0, false))
	lo := b.newLoadout(b.user, "L")
	item := b.addItem(b.user, lo, reg, 1)

	g := b.regGet(b.user, reg, http.StatusOK)
	if g.GearWeight != nil || g.UnitWeight != nil || g.WeightSource != "none" {
		t.Errorf("before: gear_weight %v unit %v source %q, want null, null, none", g.GearWeight, g.UnitWeight, g.WeightSource)
	}
	b.measure(b.user, reg, 42, "guess")
	g = b.regGet(b.user, reg, http.StatusOK)
	if g.GearWeight != nil {
		t.Errorf("gear_weight = %d, a measured weight is not a catalog weight", *g.GearWeight)
	}
	wantWeight(t, "registration", g.UnitWeight, g.WeightSource, 42, "guess")
	it := itemOf(t, b.full(b.user, lo), item)
	wantWeight(t, "item", it.UnitWeight, it.WeightSource, 42, "guess")
	if it.CatalogWeight != nil {
		t.Errorf("item catalog_weight = %d, want null", *it.CatalogWeight)
	}
	b.unmeasure(b.user, reg)
	it = itemOf(t, b.full(b.user, lo), item)
	if it.UnitWeight != nil || it.WeightSource != "none" {
		t.Errorf("after removing: unit %v source %q, want null, none", it.UnitWeight, it.WeightSource)
	}
}

// BR-026 AC-1: a value without a source, a source without a value, any other
// source, a value below 1 or a fraction is 400 and changes nothing; both can be
// changed or removed together only.
func TestMeasuredWeight_RefusedValuesChangeNothing(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Tent", 700, false))
	lo := b.newLoadout(b.user, "L")
	b.addItem(b.user, lo, reg, 1)
	b.measure(b.user, reg, 687, "weighed")
	before := b.stored()
	beforeReg, _ := json.Marshal(b.regGet(b.user, reg, http.StatusOK))

	for name, body := range map[string]string{
		"zero":                           `{"measured_weight": 0, "measured_weight_source": "weighed"}`,
		"negative":                       `{"measured_weight": -5, "measured_weight_source": "weighed"}`,
		"fraction":                       `{"measured_weight": 12.5, "measured_weight_source": "guess"}`,
		"whole number as fraction":       `{"measured_weight": 687.0, "measured_weight_source": "guess"}`,
		"exponent":                       `{"measured_weight": 1e3, "measured_weight_source": "guess"}`,
		"too large":                      `{"measured_weight": 99999999999999999999, "measured_weight_source": "guess"}`,
		"string":                         `{"measured_weight": "687", "measured_weight_source": "weighed"}`,
		"boolean":                        `{"measured_weight": true, "measured_weight_source": "weighed"}`,
		"value without source":           `{"measured_weight": 600}`,
		"source without value":           `{"measured_weight_source": "guess"}`,
		"value, null source":             `{"measured_weight": 600, "measured_weight_source": null}`,
		"null value, source":             `{"measured_weight": null, "measured_weight_source": "guess"}`,
		"null value only":                `{"measured_weight": null}`,
		"null source only":               `{"measured_weight_source": null}`,
		"other source":                   `{"measured_weight": 600, "measured_weight_source": "measured"}`,
		"catalog as a source":            `{"measured_weight": 600, "measured_weight_source": "catalog"}`,
		"source in other case":           `{"measured_weight": 600, "measured_weight_source": "Weighed"}`,
		"source is a number":             `{"measured_weight": 600, "measured_weight_source": 1}`,
		"empty source":                   `{"measured_weight": 600, "measured_weight_source": ""}`,
		"zero count with a valid weight": `{"measured_weight": 600, "measured_weight_source": "guess", "closet_count": 0}`,
	} {
		w := b.env.Do(http.MethodPost, ugPath(reg, "update"), b.user, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400; body: %s", name, w.Code, w.Body.String())
		}
	}
	if b.stored() != before {
		t.Error("a refused request changed stored data")
	}
	if got, _ := json.Marshal(b.regGet(b.user, reg, http.StatusOK)); string(got) != string(beforeReg) {
		t.Errorf("registration changed:\n%s\n%s", beforeReg, got)
	}
	b.measure(b.user, reg, 1, "guess") // the smallest allowed value
	if g := b.regGet(b.user, reg, http.StatusOK); g.UnitWeight == nil || *g.UnitWeight != 1 {
		t.Errorf("1 g not accepted: %v", g.UnitWeight)
	}
}

// BR-026 AC-2 and BR-018: only the owner sets, changes, removes or reads it;
// a foreign or unknown registration is the same 404, whatever the body says,
// and admins get no exception. Two owners of the same gear keep their own.
func TestMeasuredWeight_OwnerOnlyAndNoAdminException(t *testing.T) {
	b := newBoard(t)
	gear := b.gear("Tent", 700, false)
	reg := b.alicesReg(gear)
	bobsReg := b.register(b.bTok, b.bob, gear)
	b.measure(b.user, reg, 687, "weighed")
	before := b.stored()

	unknown := b.env.Do(http.MethodPost, ugPath(reg+1000, "update"), b.bTok, map[string]any{"measured_weight": 1, "measured_weight_source": "guess"}).Body.String()
	for name, body := range map[string]any{
		"set":             map[string]any{"measured_weight": 1, "measured_weight_source": "guess"},
		"remove":          `{"measured_weight": null, "measured_weight_source": null}`,
		"count":           map[string]any{"closet_count": 9},
		"invalid values":  `{"measured_weight": 0, "measured_weight_source": "x"}`, // 404 comes before 400
		"owner in a body": map[string]any{"usergear_user_id": b.bob, "measured_weight": 1, "measured_weight_source": "guess"},
	} {
		for who, tok := range map[string]string{"bob": b.bTok, "admin": b.admin} {
			w := b.env.Do(http.MethodPost, ugPath(reg, "update"), tok, body)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s as %s: status %d, want 404", name, who, w.Code)
			} else if w.Body.String() != unknown {
				t.Errorf("%s as %s: a foreign registration answers differently from an unknown one: %q vs %q", name, who, w.Body.String(), unknown)
			}
		}
	}
	b.regGet(b.bTok, reg, http.StatusNotFound)
	b.regGet(b.admin, reg, http.StatusNotFound)
	if b.stored() != before {
		t.Error("a refused request changed stored data")
	}

	// Bob's own registration of the same gear keeps its own weight.
	b.measure(b.bTok, bobsReg, 650, "guess")
	if g := b.regGet(b.user, reg, http.StatusOK); g.UnitWeight == nil || *g.UnitWeight != 687 || g.WeightSource != "weighed" {
		t.Errorf("alice's registration after bob's change: %v %q", g.UnitWeight, g.WeightSource)
	}
	if g := b.regGet(b.bTok, bobsReg, http.StatusOK); g.UnitWeight == nil || *g.UnitWeight != 650 || g.WeightSource != "guess" {
		t.Errorf("bob's registration: %v %q", g.UnitWeight, g.WeightSource)
	}
	if got := b.catalogWeightInDB(gear); got != 700 {
		t.Errorf("catalog weight = %d, want 700", got)
	}
}

// The measured weight is read where registrations are read: the list and the
// container contents, with the same fields.
func TestMeasuredWeight_ReportedInEveryRegistrationRead(t *testing.T) {
	b := newBoard(t)
	pack := b.alicesReg(b.gear("Pack", 1000, true))
	cup := b.alicesReg(b.gear("Cup", 30, false))
	b.closetLink(b.user, pack, cup)
	b.measure(b.user, cup, 25, "weighed")
	b.regUpdate(b.user, cup, map[string]any{"closet_count": 4}, http.StatusOK)

	check := func(what string, g models.UserGear) {
		t.Helper()
		if g.MeasuredWeight == nil || *g.MeasuredWeight != 25 || g.MeasuredWeightSource == nil || *g.MeasuredWeightSource != "weighed" || g.ClosetCount != 4 {
			t.Errorf("%s: measured %v %v count %d, want 25 weighed 4", what, g.MeasuredWeight, g.MeasuredWeightSource, g.ClosetCount)
		}
		wantWeight(t, what, g.UnitWeight, g.WeightSource, 25, "weighed")
		if g.GearWeight == nil || *g.GearWeight != 30 {
			t.Errorf("%s: gear_weight = %v, want the catalog 30", what, g.GearWeight)
		}
	}
	check("get", b.regGet(b.user, cup, http.StatusOK))

	listed, _ := page(t, b.env.Do(http.MethodGet, ugList(b.userID, "limit=100"), b.user, nil))
	found := false
	for _, it := range listed {
		if int64(it["usergear_registration_id"].(float64)) == cup {
			raw, _ := json.Marshal(it)
			var g models.UserGear
			if err := json.Unmarshal(raw, &g); err != nil {
				t.Fatal(err)
			}
			check("list", g)
			found = true
		}
	}
	if !found {
		t.Fatal("the registration is not in the list")
	}

	w := b.env.Do(http.MethodGet, ctList(pack, "limit=100"), b.user, nil)
	contents, _ := page(t, w)
	if len(contents) != 1 {
		t.Fatalf("container contents = %d, want 1", len(contents))
	}
	raw, _ := json.Marshal(contents[0])
	var g models.UserGear
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	check("container list", g)
}

// Insert takes the same fields: a measured weight with its source, and a count.
func TestRegistrationInsert_MeasuredWeightAndCount(t *testing.T) {
	b := newBoard(t)
	gear := b.gear("Tent", 700, false)
	insert := func(body any, want int) {
		t.Helper()
		expectStatus(t, b.env.Do(http.MethodPut, "/api/v1/usergear/insert", b.user, body), want)
	}

	insert(map[string]any{"usergear_gear_id": gear, "measured_weight": 687, "measured_weight_source": "weighed", "closet_count": 3}, http.StatusOK)
	g := b.regGet(b.user, b.newestReg(b.user, b.userID), http.StatusOK)
	if g.MeasuredWeight == nil || *g.MeasuredWeight != 687 || g.ClosetCount != 3 {
		t.Errorf("inserted = measured %v count %d, want 687 and 3", g.MeasuredWeight, g.ClosetCount)
	}
	wantWeight(t, "inserted", g.UnitWeight, g.WeightSource, 687, "weighed")

	// Without the fields, and with them null (the same as absent on insert).
	insert(map[string]any{"usergear_gear_id": gear}, http.StatusOK)
	g = b.regGet(b.user, b.newestReg(b.user, b.userID), http.StatusOK)
	if g.MeasuredWeight != nil || g.ClosetCount != 1 {
		t.Errorf("plain insert = measured %v count %d, want none and 1", g.MeasuredWeight, g.ClosetCount)
	}
	insert(fmt.Sprintf(`{"usergear_gear_id": %d, "measured_weight": null, "measured_weight_source": null, "closet_count": null}`, gear), http.StatusOK)
	g = b.regGet(b.user, b.newestReg(b.user, b.userID), http.StatusOK)
	if g.MeasuredWeight != nil || g.MeasuredWeightSource != nil || g.ClosetCount != 1 {
		t.Errorf("insert with nulls = measured %v %v count %d, want none and 1", g.MeasuredWeight, g.MeasuredWeightSource, g.ClosetCount)
	}

	// Refusals insert nothing.
	newest := b.newestReg(b.user, b.userID)
	for name, body := range map[string]any{
		"value without source": map[string]any{"usergear_gear_id": gear, "measured_weight": 5},
		"source without value": map[string]any{"usergear_gear_id": gear, "measured_weight_source": "guess"},
		"value, null source":   fmt.Sprintf(`{"usergear_gear_id": %d, "measured_weight": 5, "measured_weight_source": null}`, gear),
		"other source":         map[string]any{"usergear_gear_id": gear, "measured_weight": 5, "measured_weight_source": "measured"},
		"zero weight":          map[string]any{"usergear_gear_id": gear, "measured_weight": 0, "measured_weight_source": "guess"},
		"fraction":             fmt.Sprintf(`{"usergear_gear_id": %d, "measured_weight": 12.5, "measured_weight_source": "guess"}`, gear),
		"zero count":           map[string]any{"usergear_gear_id": gear, "closet_count": 0},
		"fractional count":     fmt.Sprintf(`{"usergear_gear_id": %d, "closet_count": 2.5}`, gear),
		"string count":         map[string]any{"usergear_gear_id": gear, "closet_count": "2"},
	} {
		insert(body, http.StatusBadRequest)
		_ = name
	}
	if got := b.newestReg(b.user, b.userID); got != newest {
		t.Errorf("a refused insert created registration %d", got)
	}
	// An unknown gear is 404 before an invalid field is 400 (ARS-001 section 1).
	insert(map[string]any{"usergear_gear_id": gear + 1000, "measured_weight": 5}, http.StatusNotFound)
	insert(map[string]any{"usergear_gear_id": gear + 1000, "closet_count": 2}, http.StatusNotFound)
}

// ---- BR-036 -------------------------------------------------------------------

// BR-036 verification semantics. Create a registration without a count: 1. Set
// 8, then 0 and 2.5: 0 and 2.5 are 400. Add two items of it with quantity 4 and
// 5 to one loadout: both accepted, and reads report count 8 with each item.
// Lower the count to 6: accepted. The loadout total uses quantity × unit weight,
// not the count. As another user, change the count: 404.
func TestClosetCount_DefaultSetRefuseAndNeverMultiplies(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Tent peg", 20, false))

	if g := b.regGet(b.user, reg, http.StatusOK); g.ClosetCount != 1 {
		t.Fatalf("a new registration has count %d, want 1", g.ClosetCount)
	}
	b.regUpdate(b.user, reg, map[string]any{"closet_count": 8}, http.StatusOK)
	if g := b.regGet(b.user, reg, http.StatusOK); g.ClosetCount != 8 {
		t.Errorf("count = %d, want 8", g.ClosetCount)
	}
	for _, body := range []string{`{"closet_count": 0}`, `{"closet_count": -3}`, `{"closet_count": 2.5}`, `{"closet_count": "4"}`, `{"closet_count": true}`, `{"closet_count": 1e1}`} {
		b.regUpdate(b.user, reg, body, http.StatusBadRequest)
	}
	// A null count is not a request to clear it (a count is never empty): unchanged.
	b.regUpdate(b.user, reg, `{"closet_count": null}`, http.StatusOK)
	if g := b.regGet(b.user, reg, http.StatusOK); g.ClosetCount != 8 {
		t.Errorf("count after refusals and a null = %d, want 8", g.ClosetCount)
	}

	lo := b.newLoadout(b.user, "Camp")
	i1, i2 := b.addItem(b.user, lo, reg, 4), b.addItem(b.user, lo, reg, 5) // 9 planned, 8 owned: accepted
	f := b.full(b.user, lo)
	for _, id := range []int64{i1, i2} {
		if it := itemOf(t, f, id); it.ClosetCount == nil || *it.ClosetCount != 8 {
			t.Errorf("item %d closet_count = %v, want 8", id, it.ClosetCount)
		}
	}
	if f.Loadout.TotalWeight != 9*20 {
		t.Errorf("total = %d, want %d: the count must not multiply a weight", f.Loadout.TotalWeight, 9*20)
	}

	// Lowering the count below the current use is accepted, and so is using more.
	b.regUpdate(b.user, reg, map[string]any{"closet_count": 6}, http.StatusOK)
	i3 := b.addItem(b.user, lo, reg, 10)
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 19*20 {
		t.Errorf("total = %d, want %d", got, 19*20)
	}
	b.call(http.MethodPost, itemPath(lo, i3, "update"), b.user, map[string]any{"quantity": 50}, http.StatusOK)
	// The count does not change the closet rules: one registration is at most one bag per loadout.
	bagReg := b.alicesReg(b.gear("Stuff sack", 50, true))
	b.regUpdate(b.user, bagReg, map[string]any{"closet_count": 3}, http.StatusOK)
	b.addBag(b.user, lo, bagReg)
	b.call(http.MethodPut, pPath(lo, "insert"), b.user, map[string]any{"place_kind": "bag", "usergear_registration_id": bagReg}, http.StatusConflict)

	// Another user cannot change or read it.
	b.regUpdate(b.bTok, reg, map[string]any{"closet_count": 99}, http.StatusNotFound)
	if g := b.regGet(b.user, reg, http.StatusOK); g.ClosetCount != 6 {
		t.Errorf("count after bob's attempt = %d, want 6", g.ClosetCount)
	}
}

// A request that changes only the count leaves the measured weight and the
// container limit alone, and the reverse (absent fields keep their value).
func TestRegistrationUpdate_AbsentFieldsKeepTheirValue(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Pack", 1000, true))
	b.regUpdate(b.user, reg, map[string]any{"max_container_weight": 9000, "closet_count": 2, "measured_weight": 980, "measured_weight_source": "weighed"}, http.StatusOK)

	b.regUpdate(b.user, reg, map[string]any{"closet_count": 3}, http.StatusOK)
	g := b.regGet(b.user, reg, http.StatusOK)
	if g.MeasuredWeight == nil || *g.MeasuredWeight != 980 || g.MaxContainerWeight == nil || *g.MaxContainerWeight != 9000 || g.ClosetCount != 3 {
		t.Errorf("after a count-only update: measured %v max %v count %d", g.MeasuredWeight, g.MaxContainerWeight, g.ClosetCount)
	}
	b.measure(b.user, reg, 970, "guess")
	g = b.regGet(b.user, reg, http.StatusOK)
	if g.ClosetCount != 3 || g.MaxContainerWeight == nil || *g.MaxContainerWeight != 9000 {
		t.Errorf("after a weight-only update: count %d max %v", g.ClosetCount, g.MaxContainerWeight)
	}
	// Clearing the container limit leaves the weight alone (it is the one clearable field of today).
	b.regUpdate(b.user, reg, `{"max_container_weight": null}`, http.StatusOK)
	g = b.regGet(b.user, reg, http.StatusOK)
	if g.MaxContainerWeight != nil || g.MeasuredWeight == nil || *g.MeasuredWeight != 970 {
		t.Errorf("after clearing the limit: max %v measured %v", g.MaxContainerWeight, g.MeasuredWeight)
	}
	// An empty body changes nothing, as before.
	b.regUpdate(b.user, reg, `{}`, http.StatusOK)
	if g2 := b.regGet(b.user, reg, http.StatusOK); g2.MeasuredWeight == nil || *g2.MeasuredWeight != 970 || g2.ClosetCount != 3 {
		t.Errorf("after an empty update: %+v", g2)
	}
}

// ---- BR-029 AC-3 with measured weights -----------------------------------------

// BR-029 AC-3's own example. L1 has Exos 58 (bag, measured 1 180 g) holding a
// stove (catalog 83 g, quantity 1) and gas (measured 360 g, quantity 2), a rain
// shell in Worn (395 g) and a spork in no place (catalog 11 g): the total is
// 1 180 + 83 + 720 + 11 = 1 994 g. Then reweighing the gas changes the total
// and nothing in the catalog, and an admin's stove edit changes it on the next
// read as well (the milestone B outcome).
func TestWeights_AC3ExampleWithMeasuredWeights(t *testing.T) {
	b := newBoard(t)
	gExos := b.gear("Exos 58", 1200, true) // the maker says 1 200, the owner weighed 1 180
	gStove := b.gear("Stove", 83, false)
	gGas := b.gear("Gas", 400, false) // the maker says 400, the owner weighed 360
	gShell := b.gear("Rain shell", 395, false)
	gSpork := b.gear("Spork", 11, false)
	exos, stove, gas, shell, spork := b.alicesReg(gExos), b.alicesReg(gStove), b.alicesReg(gGas), b.alicesReg(gShell), b.alicesReg(gSpork)
	b.measure(b.user, exos, 1180, "weighed")
	b.measure(b.user, gas, 360, "weighed")

	lo := b.newLoadout(b.user, "L1")
	bag := b.addBag(b.user, lo, exos)
	worn := wornOf(t, b.full(b.user, lo))
	iStove, iGas, iShell := b.addItem(b.user, lo, stove, 1), b.addItem(b.user, lo, gas, 2), b.addItem(b.user, lo, shell, 1)
	b.addItem(b.user, lo, spork, 1) // in no place
	b.move(b.user, lo, iStove, bag)
	b.move(b.user, lo, iGas, bag)
	b.move(b.user, lo, iShell, worn)

	f := b.full(b.user, lo)
	if f.Loadout.TotalWeight != 1994 || b.summary(b.user, lo).TotalWeight != 1994 {
		t.Fatalf("total = %d (list %d), want 1994", f.Loadout.TotalWeight, b.summary(b.user, lo).TotalWeight)
	}
	// Every input of the weight model is on the board: both weights and the source.
	it := itemOf(t, f, iGas)
	if it.CatalogWeight == nil || *it.CatalogWeight != 400 || it.MeasuredWeight == nil || *it.MeasuredWeight != 360 {
		t.Errorf("gas = catalog %v measured %v, want 400 and 360", it.CatalogWeight, it.MeasuredWeight)
	}
	wantWeight(t, "gas", it.UnitWeight, it.WeightSource, 360, "weighed")
	p := placeOf(f, bag)
	if p == nil || p.CatalogWeight == nil || *p.CatalogWeight != 1200 || p.MeasuredWeight == nil || *p.MeasuredWeight != 1180 ||
		p.MeasuredWeightSource == nil || *p.MeasuredWeightSource != "weighed" || p.UnitWeight == nil || *p.UnitWeight != 1180 {
		t.Errorf("bag = %+v", p)
	}
	if shell := itemOf(t, f, iShell); shell.MeasuredWeight != nil || shell.MeasuredWeightSource != nil {
		t.Errorf("shell has no measured weight: %v %v", shell.MeasuredWeight, shell.MeasuredWeightSource)
	}

	// Reweigh the gas: the loadout total changes, the catalog does not.
	b.measure(b.user, gas, 380, "weighed")
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+40 {
		t.Errorf("total after reweighing the gas = %d, want %d", got, 1994+40)
	}
	if got := b.summary(b.user, lo).TotalWeight; got != 1994+40 {
		t.Errorf("list total after reweighing = %d, want %d", got, 1994+40)
	}
	if got := b.catalogWeightInDB(gGas); got != 400 {
		t.Errorf("catalog weight of the gas = %d after reweighing, want 400", got)
	}
	// Reweigh the bag.
	b.measure(b.user, exos, 1100, "guess")
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+40-80 {
		t.Errorf("total after reweighing the bag = %d, want %d", got, 1994+40-80)
	}
	// An admin edits the stove's catalog weight: the next read shows it, and the
	// measured gas and bag are unaffected by catalog edits to other gear.
	b.setCatalogWeight(gStove, 100)
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+40-80+17 {
		t.Errorf("total after the catalog edit = %d, want %d", got, 1994+40-80+17)
	}
	// An admin edit of the gas's catalog weight does not reach the measured weight.
	b.setCatalogWeight(gGas, 450)
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+40-80+17 {
		t.Errorf("total after a catalog edit of measured gear = %d, want %d", got, 1994+40-80+17)
	}
	// Removing the gas's measured weight returns the (new) catalog weight.
	b.unmeasure(b.user, gas)
	if got := b.full(b.user, lo).Loadout.TotalWeight; got != 1994+40-80+17+2*(450-380) {
		t.Errorf("total after removing the measured weight = %d, want %d", got, 1994+40-80+17+2*(450-380))
	}
}

// ---- BR-030 with measured weight -------------------------------------------------

// BR-030 verification semantics. Registration R is used as an item in L1 (in a
// bag, ticked, measured 650 g guess) and as a bag in L2 holding two items and
// one nested bag. Delete R through the API. L1: the item is still there with its
// place, tick and quantity, reports no longer in the closet, and still counts
// 650 g with source guess; a later catalog change does not reach it. L2: R is no
// longer a bag, its items have no place, the nested bag is top level.
func TestDeleteRegistration_SnapshotKeepsMeasuredWeight(t *testing.T) {
	b := newBoard(t)
	gR := b.gear("Dry bag", 700, true)
	r := b.alicesReg(gR)
	carrier := b.alicesReg(b.gear("Pack", 1000, true))
	b.measure(b.user, r, 650, "guess")
	other := b.alicesReg(b.gear("Spork", 11, false))
	inner := b.alicesReg(b.gear("Pouch", 40, true))

	l1 := b.newLoadout(b.user, "L1")
	pack := b.addBag(b.user, l1, carrier)
	item := b.addItem(b.user, l1, r, 2)
	b.move(b.user, l1, item, pack)
	b.tick(b.user, l1, item, true, http.StatusOK)

	l2 := b.newLoadout(b.user, "L2")
	bagR := b.addBag(b.user, l2, r)
	nested := b.addBag(b.user, l2, inner)
	b.call(http.MethodPost, pPlacePath(l2, nested, "update"), b.user, map[string]any{"parent_place_id": bagR}, http.StatusOK)
	s1, s2 := b.addItem(b.user, l2, other, 1), b.addItem(b.user, l2, other, 1)
	b.move(b.user, l2, s1, bagR)
	b.move(b.user, l2, s2, bagR)

	beforeL1 := b.full(b.user, l1).Loadout.TotalWeight // 1000 + 2 x 650
	if beforeL1 != 2300 {
		t.Fatalf("L1 total before = %d, want 2300", beforeL1)
	}
	b.call(http.MethodDelete, ugPath(r, "delete"), b.user, nil, http.StatusOK)

	f1 := b.full(b.user, l1)
	it := itemOf(t, f1, item)
	if it.RegistrationExists || it.UsergearRegistrationID != nil || it.Quantity != 2 || !it.Ticked || it.PlaceID == nil || *it.PlaceID != pack {
		t.Errorf("L1 item after the delete = %+v", it)
	}
	wantWeight(t, "kept weight", it.UnitWeight, it.WeightSource, 650, "guess")
	// A deleted-registration item reports only its snapshot: no live catalog or measured values.
	if it.CatalogWeight != nil || it.MeasuredWeight != nil || it.MeasuredWeightSource != nil || it.ClosetCount != nil {
		t.Errorf("snapshot reports live values: catalog %v measured %v %v count %v", it.CatalogWeight, it.MeasuredWeight, it.MeasuredWeightSource, it.ClosetCount)
	}
	if f1.Loadout.TotalWeight != beforeL1 {
		t.Errorf("L1 total after the delete = %d, want the kept %d", f1.Loadout.TotalWeight, beforeL1)
	}
	b.setCatalogWeight(gR, 999)
	it = itemOf(t, b.full(b.user, l1), item)
	wantWeight(t, "kept weight after a catalog change", it.UnitWeight, it.WeightSource, 650, "guess")

	f2 := b.full(b.user, l2)
	if placeOf(f2, bagR) != nil {
		t.Error("R is still a bag of L2")
	}
	for _, id := range []int64{s1, s2} {
		if it := itemOf(t, f2, id); it.PlaceID != nil {
			t.Errorf("L2 item %d still has place %d", id, *it.PlaceID)
		}
	}
	if p := placeOf(f2, nested); p == nil || p.ParentPlaceID != nil {
		t.Errorf("the nested bag must be top level: %+v", p)
	}
	// Remove and move the snapshot like any item (BR-030 AC-2).
	b.move(b.user, l1, item, wornOf(t, f1))
	b.call(http.MethodDelete, itemPath(l1, item, "delete"), b.user, nil, http.StatusOK)
}

// A legacy item has no registration: it reports no measured weight and no
// count, and its weight follows the catalog (ADR-004 D-2).
func TestLegacyItemReportsNoRegistrationValues(t *testing.T) {
	b := newBoard(t)
	gear := b.gear("Not owned", 300, false)
	lo := b.newLoadout(b.user, "L")
	item := num(b.call(http.MethodPut, loPath(lo, "item/insert"), b.user, map[string]any{"gear_id": gear, "quantity": 1}, http.StatusCreated), "loadout_item_id")

	it := itemOf(t, b.full(b.user, lo), item)
	if it.RegistrationExists || it.MeasuredWeight != nil || it.MeasuredWeightSource != nil || it.ClosetCount != nil {
		t.Errorf("legacy item = %+v", it)
	}
	wantWeight(t, "legacy item", it.UnitWeight, it.WeightSource, 300, "catalog")
}
