package endpoints

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/Sea-Shell/gogear-api/pkg/models"
)

// Exact on-your-back total (BR-029 AC-3, ARS-001 "total_weight"). Every
// quantity and weight the API accepts is kept, so the total can pass 2^63-1.
// These tests read the total as the raw token of the response body, never
// through float64 or int64: a float or a clamp would show as a different
// digit string, and a quoted number as a quote.

// totalOf is a total that fits int64, for the tests of ordinary weights.
func totalOf(w models.WeightTotal) int64 {
	n, err := strconv.ParseInt(w.String(), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("total %s does not fit int64: %v", w, err))
	}
	return n
}

const (
	maxInt64 = math.MaxInt64 // 9223372036854775807
	// Expected totals, written out so a wrong sum cannot hide behind the code
	// that computes it.
	twoMax            = "18446744073709551614"                   // 2 x MaxInt64
	twoMaxPlus83      = "18446744073709551697"                   // 2 x MaxInt64 + 83
	maxTimesMax       = "85070591730234615847396907784232501249" // MaxInt64 x MaxInt64
	maxTimesMaxPlus83 = "85070591730234615847396907784232501332" // MaxInt64 x MaxInt64 + 83
)

var bareInteger = regexp.MustCompile(`^[0-9]+$`)

// rawTotal is the token of "total_weight" in a JSON object, exactly as written.
// It fails the test when the value is a string, null, a fraction or an exponent.
func rawTotal(t *testing.T, where string, obj json.RawMessage) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(obj, &m); err != nil {
		t.Fatalf("%s: not an object: %v: %s", where, err, obj)
	}
	tok, ok := m["total_weight"]
	if !ok {
		t.Fatalf("%s: no total_weight in %s", where, obj)
	}
	if !bareInteger.Match(tok) {
		t.Errorf("%s: total_weight token %s is not a bare unsigned integer", where, tok)
	}
	return string(tok)
}

// rawTotals reads the total of loadout lo through every route that reports it:
// the list, get, full, and (when slug is not "") the public loadout.
func (b *board) rawTotals(lo int64, slug string) map[string]string {
	b.env.T.Helper()
	t := b.env.T
	got := map[string]string{}

	w := b.env.Do(http.MethodGet, "/api/v1/loadout/list", b.user, nil)
	expectStatus(t, w, http.StatusOK)
	var all []json.RawMessage
	decodeJSON(t, w, &all)
	for _, l := range all {
		var id struct {
			LoadoutID int64 `json:"loadout_id"`
		}
		if err := json.Unmarshal(l, &id); err != nil {
			t.Fatal(err)
		}
		if id.LoadoutID == lo {
			got["list"] = rawTotal(t, "list", l)
		}
	}

	w = b.env.Do(http.MethodGet, loPath(lo, "get"), b.user, nil)
	expectStatus(t, w, http.StatusOK)
	got["get"] = rawTotal(t, "get", w.Body.Bytes())

	w = b.env.Do(http.MethodGet, loPath(lo, "full"), b.user, nil)
	expectStatus(t, w, http.StatusOK)
	var full map[string]json.RawMessage
	decodeJSON(t, w, &full)
	got["full"] = rawTotal(t, "full", full["loadout"])

	if slug != "" {
		w = b.env.Do(http.MethodGet, "/api/v1/public/loadout/"+slug, "", nil)
		expectStatus(t, w, http.StatusOK)
		got["public"] = rawTotal(t, "public", w.Body.Bytes())
	}
	return got
}

func wantTotals(t *testing.T, got map[string]string, want string) {
	t.Helper()
	for _, route := range []string{"list", "get", "full", "public"} {
		if g, ok := got[route]; !ok || g != want {
			t.Errorf("total_weight on %s = %q, want %q", route, g, want)
		}
	}
}

// measureMax gives a registration the largest measured weight BR-026 accepts.
func (b *board) measureMax(reg int64) {
	b.env.T.Helper()
	b.regUpdate(b.user, reg, map[string]any{"measured_weight": int64(maxInt64), "measured_weight_source": "weighed"}, http.StatusOK)
}

// publicLoadout creates a public loadout with a slug.
func (b *board) publicLoadout(name, slug string) int64 {
	b.env.T.Helper()
	return num(b.call(http.MethodPut, "/api/v1/loadout/insert", b.user,
		map[string]any{"loadout_name": name, "loadout_is_public": true, "loadout_slug": slug}, http.StatusCreated), "loadout_id")
}

// A product that does not fit int64: 2 x 9223372036854775807. SQLite turns
// that product into a REAL and float64 would write ...616.
func TestExactTotal_ProductBeyondInt64(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Tent", 700, false))
	b.measureMax(reg)
	lo := b.publicLoadout("Big", "big")
	b.addItem(b.user, lo, reg, 2)

	wantTotals(t, b.rawTotals(lo, "big"), twoMax)
}

// Every product fits int64, only the sum does not: an item and a bag of
// 9223372036854775807 g and an 83 g item. SQLite's SUM raises an error here.
func TestExactTotal_SumBeyondInt64WithNoProductOverflow(t *testing.T) {
	b := newBoard(t)
	item := b.alicesReg(b.gear("Heavy thing", 5, false))
	bagReg := b.alicesReg(b.gear("Heavy bag", 5, true))
	small := b.alicesReg(b.gear("Stove", 83, false))
	b.measureMax(item)
	b.measureMax(bagReg)
	lo := b.publicLoadout("Sum", "sum")
	b.addBag(b.user, lo, bagReg)
	b.addItem(b.user, lo, item, 1)
	b.addItem(b.user, lo, small, 1)

	wantTotals(t, b.rawTotals(lo, "sum"), twoMaxPlus83)
}

// Both extremes at once: quantity and weight of 9223372036854775807, the
// largest product the API can store, and a bag, and a small item on top.
func TestExactTotal_MaximumProduct(t *testing.T) {
	b := newBoard(t)
	reg := b.alicesReg(b.gear("Anvil", 5, false))
	b.measureMax(reg)
	lo := b.publicLoadout("Max", "max")
	b.addItem(b.user, lo, reg, maxInt64)
	wantTotals(t, b.rawTotals(lo, "max"), maxTimesMax)

	b.addItem(b.user, lo, b.alicesReg(b.gear("Stove", 83, false)), 1)
	wantTotals(t, b.rawTotals(lo, "max"), maxTimesMaxPlus83)
}

// Worn is not on the back, however large; a bag counts once; moving the item
// out of Worn counts it at once, with the same exact total on every route.
func TestExactTotal_WornLeftOutAndBagCountedOnce(t *testing.T) {
	b := newBoard(t)
	heavy := b.alicesReg(b.gear("Heavy thing", 5, false))
	bagReg := b.alicesReg(b.gear("Heavy bag", 5, true))
	small := b.alicesReg(b.gear("Stove", 83, false))
	b.measureMax(heavy)
	b.measureMax(bagReg)
	lo := b.publicLoadout("Worn", "worn")
	bag := b.addBag(b.user, lo, bagReg)
	worn := wornOf(t, b.full(b.user, lo))
	iHeavy := b.addItem(b.user, lo, heavy, maxInt64)
	b.addItem(b.user, lo, small, 1)
	b.move(b.user, lo, iHeavy, worn)

	// Only the bag (once) and the 83 g item are on the back.
	wantTotals(t, b.rawTotals(lo, "worn"), "9223372036854775890")

	// Into the bag, the item counts: MaxInt64 x MaxInt64 + the bag + 83.
	b.move(b.user, lo, iHeavy, bag)
	wantTotals(t, b.rawTotals(lo, "worn"), "85070591730234615856620279821087277139")

	// And back to Worn: the total returns.
	b.move(b.user, lo, iHeavy, worn)
	wantTotals(t, b.rawTotals(lo, "worn"), "9223372036854775890")
}

// Ordinary totals are the same bare integers as before: the 1 994 g example of
// BR-029 AC-3, an empty loadout (0), and a loadout of gear without a weight.
func TestExactTotal_OrdinaryTotalsAreBareIntegers(t *testing.T) {
	b := newBoard(t)
	exos := b.alicesReg(b.gear("Exos 58", 1180, true))
	stove := b.alicesReg(b.gear("Stove", 83, false))
	gas := b.alicesReg(b.gear("Gas", 360, false))
	shell := b.alicesReg(b.gear("Rain shell", 395, false))
	spork := b.alicesReg(b.gear("Spork", 11, false))
	unweighed := b.alicesReg(b.gear("Mystery", 0, false))

	empty := b.publicLoadout("Empty", "empty")
	wantTotals(t, b.rawTotals(empty, "empty"), "0")

	lo := b.publicLoadout("L1", "l1")
	bag := b.addBag(b.user, lo, exos)
	worn := wornOf(t, b.full(b.user, lo))
	iStove, iGas, iShell := b.addItem(b.user, lo, stove, 1), b.addItem(b.user, lo, gas, 2), b.addItem(b.user, lo, shell, 1)
	b.addItem(b.user, lo, spork, 1)
	b.addItem(b.user, lo, unweighed, 3)
	b.move(b.user, lo, iStove, bag)
	b.move(b.user, lo, iGas, bag)
	b.move(b.user, lo, iShell, worn)
	wantTotals(t, b.rawTotals(lo, "l1"), "1994")

	// The response to the insert reports the new loadout's total too.
	w := b.env.Do(http.MethodPut, "/api/v1/loadout/insert", b.user, map[string]any{"loadout_name": "fresh"})
	expectStatus(t, w, http.StatusCreated)
	if got := rawTotal(t, "insert", w.Body.Bytes()); got != "0" {
		t.Errorf("total_weight of a new loadout = %s, want 0", got)
	}
}

// A legacy item (no registration, gear read live) counts like any other.
func TestExactTotal_LegacyItemCountsExactly(t *testing.T) {
	b := newBoard(t)
	g := b.gear("Old", 2147483647, false) // the largest catalog weight
	lo := b.publicLoadout("Legacy", "legacy")
	b.call(http.MethodPut, loPath(lo, "item/insert"), b.user, map[string]any{"gear_id": g, "quantity": int64(maxInt64)}, http.StatusCreated)

	// 2147483647 x 9223372036854775807
	wantTotals(t, b.rawTotals(lo, "legacy"), "19807040619342712359383728129")
}

// The decoder side: the totals of the two term arrays one statement returns
// (items, then bags). The terms are integer literals; anything else is an
// error, never a rounded value.
func TestSumOnBackTerms(t *testing.T) {
	for _, c := range []struct {
		name  string
		terms []string
		want  string
	}{
		{"none", []string{`[]`, `[]`}, "0"},
		{"one item", []string{`[[3,100]]`, `[]`}, "300"},
		{"product past int64", []string{`[[2,9223372036854775807]]`, `[]`}, twoMax},
		{"sum past int64", []string{`[[1,9223372036854775807],[1,83]]`, `[[1,9223372036854775807]]`}, twoMaxPlus83},
		{"largest product", []string{`[[9223372036854775807,9223372036854775807]]`, `[]`}, maxTimesMax},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := sumOnBackTerms(c.terms...)
			if err != nil || got.String() != c.want {
				t.Errorf("sum(%v) = %s, %v; want %s", c.terms, got, err, c.want)
			}
		})
	}
	for _, bad := range []string{`[[1,1.5]]`, `[[1,2e3]]`, `[[1,1.0]]`, `[["2",5]]`, `[[null,5]]`, `[[1]]`, `[[1,2,3]]`, `{}`, ``, `[[1,5]`} {
		t.Run("rejects "+bad, func(t *testing.T) {
			if got, err := sumOnBackTerms(`[[1,1]]`, bad); err == nil {
				t.Errorf("sum(%s) = %s, want an error", bad, got)
			}
		})
	}
}
