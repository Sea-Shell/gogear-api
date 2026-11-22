package endpoints

import (
	"fmt"
	"net/http"
	"testing"
)

// Gear reads with missing values: dimensions and status may be NULL in the
// catalog.

func gearBody(t *testing.T, f *catalogFixture, method, path, tok string, body any) map[string]any {
	t.Helper()
	w := f.env.Do(method, path, tok, body)
	expectStatus(t, w, http.StatusOK)
	var m map[string]any
	decodeJSON(t, w, &m)
	return m
}

// GET /gear/:id/get returned 400 for gear whose height, length, width or
// status is NULL, because the row could not be scanned.
func TestGetGear_NullDimensionsAndStatusAreReturned(t *testing.T) {
	f := newCatalogFixture(t)
	g := f.env.seedGearX(false, true, f.cat, f.top)
	got := gearBody(t, f, http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", g), f.userTok, nil)
	if int64(got["gear_id"].(float64)) != g || got["gear_name"] != "nulls" || got["gear_weight"] != float64(100) {
		t.Errorf("gear = %v", got)
	}
	for _, k := range []string{"gear_height", "gear_length", "gear_width", "gear_status"} {
		v, present := got[k]
		if !present || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, present)
		}
	}

	full := f.env.seedGearX(false, false, f.cat, f.top)
	got = gearBody(t, f, http.MethodGet, fmt.Sprintf("/api/v1/gear/%d/get", full), f.userTok, nil)
	if got["gear_height"] != float64(1) || got["gear_length"] != float64(2) || got["gear_width"] != float64(3) || got["gear_status"] != true {
		t.Errorf("gear with values = %v", got)
	}
}
