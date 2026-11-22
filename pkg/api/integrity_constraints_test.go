package endpoints

import (
	"net/http"
	"testing"
)

// Constraints added by migration 000005 (COR-05), seen through the handlers.
// A duplicate container link is 409; the database itself refuses self-links
// and non-positive quantities.

func TestContainerInsert_DuplicateLinkIs409(t *testing.T) {
	f := newIntFixture(t)
	body := map[string]any{"user_container_id": f.regBox, "user_gear_registration_id": f.regThing}
	expectStatus(t, f.env.Do(http.MethodPut, "/api/v1/container/insert", f.aTok, body), http.StatusOK)
	w := f.env.Do(http.MethodPut, "/api/v1/container/insert", f.aTok, body)
	expectStatus(t, w, http.StatusConflict)
	if n := f.env.Count("user_container_registration", ""); n != 1 {
		t.Fatalf("%d links, want 1", n)
	}
	// The reverse direction would put each inside the other: a cycle (BR-020 AC-4).
	expectStatus(t, f.env.Do(http.MethodPut, "/api/v1/container/insert", f.aTok,
		map[string]any{"user_container_id": f.regThing, "user_gear_registration_id": f.regBox}), http.StatusConflict)
}

// The database itself refuses self-links, whatever the handler does.
func TestDatabaseRefusesSelfLinkAndNonPositiveQuantity(t *testing.T) {
	f := newIntFixture(t)
	if _, err := f.env.DB.Exec(`INSERT INTO user_container_registration (userContainerId, userGearRegistrationId) VALUES (?, ?)`, f.regBox, f.regBox); err == nil {
		t.Fatal("database accepted a self-link")
	}
	if _, err := f.env.DB.Exec(`UPDATE loadout_items SET quantity = 0 WHERE loadoutItemId = ?`, f.loadoutItem); err == nil {
		t.Fatal("database accepted quantity 0")
	}
}
