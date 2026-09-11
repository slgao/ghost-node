package provisioner

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBurnedMatchesExactAndNeighbours(t *testing.T) {
	store, err := LoadBurnedStore("")
	if err != nil {
		t.Fatal(err)
	}
	store.Add("203.0.113.7")

	if ok, _ := store.Burned("203.0.113.7", time.Hour); !ok {
		t.Error("the exact address should be reported as burned")
	}
	// Censors commonly null-route a whole /24, so a neighbour is a poor draw.
	if ok, why := store.Burned("203.0.113.99", time.Hour); !ok {
		t.Error("an address in the same /24 should be reported as burned")
	} else if why == "" {
		t.Error("expected an explanation for the neighbour match")
	}
	if ok, _ := store.Burned("198.51.100.7", time.Hour); ok {
		t.Error("an unrelated address should not be reported as burned")
	}
}

func TestBurnedRespectsWindow(t *testing.T) {
	store, err := LoadBurnedStore("")
	if err != nil {
		t.Fatal(err)
	}
	store.Add("203.0.113.7")

	if ok, _ := store.Burned("203.0.113.7", 0); ok {
		t.Error("a zero window should disable the check")
	}
	store.entries["203.0.113.7"] = time.Now().Add(-48 * time.Hour)
	if ok, _ := store.Burned("203.0.113.7", time.Hour); ok {
		t.Error("an entry older than the window should not match")
	}
}

func TestBurnedStoreRoundTripsAndPrunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "burned.json")

	store, err := LoadBurnedStore(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Add("203.0.113.7")
	store.entries["198.51.100.1"] = time.Now().Add(-90 * 24 * time.Hour)

	if err := store.Save(30 * 24 * time.Hour); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := LoadBurnedStore(path)
	if err != nil {
		t.Fatalf("LoadBurnedStore: %v", err)
	}
	if ok, _ := reloaded.Burned("203.0.113.7", time.Hour); !ok {
		t.Error("the recent entry did not survive the round trip")
	}
	if _, found := reloaded.entries["198.51.100.1"]; found {
		t.Error("the stale entry should have been pruned on save")
	}
}

func TestLoadBurnedStoreMissingFileIsNotAnError(t *testing.T) {
	store, err := LoadBurnedStore(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("a missing store should load empty, got: %v", err)
	}
	if len(store.List()) != 0 {
		t.Error("expected an empty store")
	}
}
