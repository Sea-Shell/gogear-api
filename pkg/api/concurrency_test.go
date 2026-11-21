package endpoints

import (
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
