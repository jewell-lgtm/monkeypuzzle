package inbox

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

func TestState_LoadMissingIsEmpty(t *testing.T) {
	st, err := Load(filepath.Join(t.TempDir(), "inbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != stateVersion || len(st.Order) != 0 || st.Order == nil {
		t.Fatalf("want empty state, got %+v", st)
	}
}

func TestState_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	until := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	fetched := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	in := State{
		Order:   []string{"mp/a", "api/b"},
		Notes:   map[string]string{"api/b": "waiting on review"},
		Snoozed: map[string]time.Time{"api/b": until},
		Cache:   map[string]CacheEntry{"api/b": {PR: &PR{Number: 12, URL: "u", State: "open", Draft: true}, FetchedAt: fetched}},
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != stateVersion || len(out.Order) != 2 || out.Order[1] != "api/b" {
		t.Errorf("order: %+v", out)
	}
	if out.Notes["api/b"] != "waiting on review" || !out.Snoozed["api/b"].Equal(until) {
		t.Errorf("annotations: %+v", out)
	}
	c := out.Cache["api/b"]
	if c.PR == nil || c.PR.Number != 12 || !c.PR.Draft || !c.FetchedAt.Equal(fetched) {
		t.Errorf("cache: %+v", c)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file left behind")
	}
}

func TestState_LoadRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("want parse error")
	}
}

// TestUpdate_Locks proves concurrent read-modify-write cycles through Update
// serialize on the lock file: every goroutine's append survives.
func TestUpdate_Locks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(paths.EnvConfigDir, dir)
	fs := adapters.NewOSFS("")

	const n = 25
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- Update(fs, func(st *State) (bool, error) {
				st.Order = append(st.Order, "p/"+strconv.Itoa(i))
				return true, nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	st, err := Load(filepath.Join(dir, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Order) != n {
		t.Fatalf("lost updates: want %d keys, got %d", n, len(st.Order))
	}
}

func TestUpdate_UnchangedDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(paths.EnvConfigDir, dir)
	err := Update(adapters.NewOSFS(""), func(st *State) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFileName)); !os.IsNotExist(err) {
		t.Fatal("state file written despite no change")
	}
}
