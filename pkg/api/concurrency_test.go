package endpoints

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// Regression: concurrent writers on the file-backed database must all
// succeed. With deferred transactions SQLite answered SQLITE_BUSY at once and
// handlers returned 500 "database is locked".
func TestConcurrentWritesAllSucceed(t *testing.T) {
	env := newAuthzEnv(t)
	alice := env.SeedUser("alice", "alice@example.com", false)
	bob := env.SeedUser("bob", "bob@example.com", false)
	toks := map[int64]string{alice: env.Token(alice, false), bob: env.Token(bob, false)}
	gear := env.seedGearW(100)
	loadouts := map[int64]int64{alice: env.seedLO(alice, false, "a-lo"), bob: env.seedLO(bob, false, "b-lo")}
	items := map[int64]int64{alice: env.seedItem(loadouts[alice], gear, 1), bob: env.seedItem(loadouts[bob], gear, 1)}

	const perUser = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for _, u := range []int64{alice, bob} {
		for i := 0; i < perUser; i++ {
			wg.Add(1)
			go func(u int64, i int) {
				defer wg.Done()
				var code int
				if i%2 == 0 {
					code = env.Do(http.MethodPost, loPath(loadouts[u], "update"), toks[u],
						map[string]any{"loadout_description": "d"}).Code
				} else {
					code = env.Do(http.MethodPost, itemPath(loadouts[u], items[u], "update"), toks[u],
						map[string]any{"quantity": i + 1}).Code
				}
				mu.Lock()
				codes[code]++
				mu.Unlock()
			}(u, i)
		}
	}
	wg.Wait()
	if codes[http.StatusOK] != 2*perUser {
		t.Fatalf("want all %d writes to succeed, got status counts %v", 2*perUser, codes)
	}
}

// A burst of mixed writes and reads from three users (one an admin) on the
// file-backed database. Every request must succeed: no "database is locked",
// whichever tables the requests touch. Under the rollback journal readers and
// writers block each other and this fails; WAL lets them proceed together.
func TestConcurrentMixedWorkloadAllSucceeds(t *testing.T) {
	env := newAuthzEnv(t)
	alice := env.SeedUser("alice", "alice@example.com", false)
	bob := env.SeedUser("bob", "bob@example.com", false)
	root := env.SeedUser("root", "root@example.com", true)
	users := []int64{alice, bob, root}
	gear := env.seedGearW(100)
	catalogGear := env.seedGearX(false, false, 1, 1)
	rootAdminTok := env.Token(root, true)

	type tenant struct {
		tok      string
		loadout  int64
		item     int64
		contents []int64 // pre-seeded registration pairs: containers[i] gets contents[i]
		boxes    []int64
	}
	const total, kinds = 240, 8
	perUser := total / len(users)
	tenants := map[int64]*tenant{}
	for _, u := range users {
		tn := &tenant{tok: env.Token(u, u == root)}
		tn.loadout = env.seedLO(u, false, fmt.Sprintf("lo-%d", u))
		tn.item = env.seedItem(tn.loadout, gear, 1)
		for i := 0; i < perUser/kinds+2; i++ {
			tn.boxes = append(tn.boxes, env.SeedRegistration(u, env.capGear()))
			tn.contents = append(tn.contents, env.SeedRegistration(u, gear))
		}
		tenants[u] = tn
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := map[string]int{}
	record := func(what string, got, want int) {
		if got != want {
			mu.Lock()
			failures[fmt.Sprintf("%s: %d (want %d)", what, got, want)]++
			mu.Unlock()
		}
	}
	for _, u := range users {
		for i := 0; i < perUser; i++ {
			wg.Add(1)
			go func(u int64, i int) {
				defer wg.Done()
				tn := tenants[u]
				switch i % kinds {
				case 0:
					record("loadout update", env.Do(http.MethodPost, loPath(tn.loadout, "update"), tn.tok,
						map[string]any{"loadout_description": "d"}).Code, http.StatusOK)
				case 1:
					record("item update", env.Do(http.MethodPost, itemPath(tn.loadout, tn.item, "update"), tn.tok,
						map[string]any{"quantity": i + 1}).Code, http.StatusOK)
				case 2:
					record("registration insert", env.Do(http.MethodPut, "/api/v1/usergear/insert", tn.tok,
						map[string]any{"usergear_gear_id": gear}).Code, http.StatusOK)
				case 3:
					n := i / kinds
					record("container link insert", env.Do(http.MethodPut, "/api/v1/container/insert", tn.tok,
						map[string]any{"user_container_id": tn.boxes[n], "user_gear_registration_id": tn.contents[n]}).Code, http.StatusOK)
				case 4:
					record("catalog update", env.Do(http.MethodPost, fmt.Sprintf("/api/v1/gear/%d/update", catalogGear), rootAdminTok,
						map[string]any{"gear_name": fmt.Sprintf("n%d", i)}).Code, http.StatusOK)
				case 5:
					record("user update", env.Do(http.MethodPost, userPath(u, "update"), tn.tok,
						map[string]any{"user_name": fmt.Sprintf("name %d", i)}).Code, http.StatusOK)
				case 6:
					record("loadout list", env.Do(http.MethodGet, "/api/v1/loadout/list", tn.tok, nil).Code, http.StatusOK)
					record("user list", env.Do(http.MethodGet, "/api/v1/users/list", tn.tok, nil).Code, http.StatusOK)
					record("registration list", env.Do(http.MethodGet, fmt.Sprintf("/api/v1/usergear/%d/list", u), tn.tok, nil).Code, http.StatusOK)
				case 7:
					record("loadout insert", env.Do(http.MethodPut, "/api/v1/loadout/insert", tn.tok,
						map[string]any{"loadout_name": fmt.Sprintf("l%d", i)}).Code, http.StatusCreated)
					record("item insert", env.Do(http.MethodPut, loPath(tn.loadout, "item/insert"), tn.tok,
						map[string]any{"gear_id": gear, "quantity": 1}).Code, http.StatusCreated)
				}
			}(u, i)
		}
	}
	wg.Wait()
	if len(failures) != 0 {
		t.Fatalf("requests failed under concurrency: %v", failures)
	}
}
