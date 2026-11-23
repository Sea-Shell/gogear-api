package endpoints

import (
	"net/http"
	"strings"
	"testing"
)

// Item writes map a database trigger or unique refusal like place writes: a
// 409 with a fixed message, the trigger logged as a missed handler check
// (ARS-001 section 1). They never become a 500 or a slug message. The
// refusals are forced with a trigger or index added by the test, since the
// handlers check every rule before writing.

func TestItemWrites_TriggerRefusalIs409AndLogged(t *testing.T) {
	for _, c := range []struct {
		name, event string
		method      string
		path        func(f *placeFixture, item int64) string
		body        any
	}{
		{"insert", "INSERT", http.MethodPut, func(f *placeFixture, _ int64) string { return loPath(f.lo, "item/insert") },
			nil}, // body set below
		{"update", "UPDATE", http.MethodPost, func(f *placeFixture, i int64) string { return itemPath(f.lo, i, "update") },
			map[string]any{"quantity": 2}},
		{"tick", "UPDATE", http.MethodPost, func(f *placeFixture, i int64) string { return itemPath(f.lo, i, "tick") },
			map[string]any{"ticked": true}},
		{"delete", "DELETE", http.MethodDelete, func(f *placeFixture, i int64) string { return itemPath(f.lo, i, "delete") }, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newPlaceFixture(t)
			item := f.env.seedRegItem(f.lo, f.tent, 0, 1)
			body := c.body
			if c.name == "insert" {
				body = map[string]any{"usergear_registration_id": f.pad}
			}
			if _, err := f.env.DB.Exec(`CREATE TRIGGER fail_item BEFORE ` + c.event + ` ON loadout_items BEGIN SELECT RAISE(ABORT, 'injected trigger refusal'); END`); err != nil {
				t.Fatal(err)
			}
			before := f.stored()
			code, resp := f.do(c.method, c.path(f, item), body)
			if code != http.StatusConflict {
				t.Fatalf("status %d, want 409: %s", code, resp)
			}
			if !strings.Contains(resp, "conflicts with the loadout's current state") || strings.Contains(resp, "slug") || strings.Contains(resp, "injected") {
				t.Errorf("body = %s", resp)
			}
			if f.stored() != before {
				t.Error("the refused write changed stored data")
			}
			logged := false
			for _, e := range f.env.Logs.All() {
				if strings.Contains(e.Message, "item write refused by a database trigger") && strings.Contains(e.Message, "injected trigger refusal") {
					logged = true
				}
			}
			if !logged {
				t.Errorf("the trigger refusal was not logged: %v", f.env.Logs.All())
			}
		})
	}
}

func TestItemWrites_UniqueRefusalIs409WithoutSlugMessage(t *testing.T) {
	f := newPlaceFixture(t)
	// The schema allows an item once per registration per loadout today; the
	// test makes it unique to force the refusal.
	if _, err := f.env.DB.Exec(`CREATE UNIQUE INDEX tmp_item_unique ON loadout_items(loadoutId, registrationId)`); err != nil {
		t.Fatal(err)
	}
	if code, resp := f.do(http.MethodPut, loPath(f.lo, "item/insert"), map[string]any{"usergear_registration_id": f.pad}); code != http.StatusCreated {
		t.Fatalf("first insert: %d %s", code, resp)
	}
	before := f.stored()
	code, resp := f.do(http.MethodPut, loPath(f.lo, "item/insert"), map[string]any{"usergear_registration_id": f.pad})
	if code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", code, resp)
	}
	if !strings.Contains(resp, "the item already exists in this loadout") || strings.Contains(resp, "slug") {
		t.Errorf("body = %s", resp)
	}
	if f.stored() != before {
		t.Error("the refused insert changed stored data")
	}
}
