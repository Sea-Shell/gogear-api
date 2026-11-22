package endpoints

import (
	"net/http"
	"testing"
)

// An absent weight is stored as NULL on gear create (ADR-003 D-7).

func (f *catalogFixture) storedWeight(gear int64) (isNull bool, weight int64) {
	f.env.T.Helper()
	var w *int64
	if err := f.env.DB.QueryRow(`SELECT gearWeight FROM gear WHERE gearId = ?`, gear).Scan(&w); err != nil {
		f.env.T.Fatal(err)
	}
	if w == nil {
		return true, 0
	}
	return false, *w
}

func TestInsertGear_AbsentWeightIsStoredAsNull(t *testing.T) {
	f := newCatalogFixture(t)
	base := func() map[string]any {
		return map[string]any{"gear_top_category_id": f.top, "gear_category_id": f.cat, "gear_manufacture_id": f.man, "gear_name": "no weight"}
	}

	for name, body := range map[string]map[string]any{
		"absent": base(),
		"null":   func() map[string]any { b := base(); b["gear_weight"] = nil; return b }(),
	} {
		got := gearBody(t, f, http.MethodPut, "/api/v1/gear/insert", f.adminTok, body)
		id := int64(got["gear_id"].(float64))
		if isNull, w := f.storedWeight(id); !isNull {
			t.Errorf("%s weight: stored %d, want NULL", name, w)
		}
		if v, present := got["gear_weight"]; !present || v != nil {
			t.Errorf("%s weight: response gear_weight = %v, want null", name, v)
		}
	}

	b := base()
	b["gear_weight"] = 250
	got := gearBody(t, f, http.MethodPut, "/api/v1/gear/insert", f.adminTok, b)
	id := int64(got["gear_id"].(float64))
	if isNull, w := f.storedWeight(id); isNull || w != 250 || got["gear_weight"] != float64(250) {
		t.Errorf("given weight: stored (null=%v, %d), response %v, want 250", isNull, w, got["gear_weight"])
	}
}
