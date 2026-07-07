package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// A non-atomic SaveSites (plain truncate-then-write) racing a reader or a second
// writer can leave sites.yaml half-written, which is how a live daemon dropped
// the whole registry during a restart. Hammer SaveSites and LoadSites
// concurrently: every read must parse a complete file and no temp file may linger.
func TestSaveSitesConcurrentNeverCorrupts(t *testing.T) {
	setDataDir(t)

	mk := func(n int) *SiteRegistry {
		reg := &SiteRegistry{}
		for i := 0; i < n; i++ {
			reg.Sites = append(reg.Sites, Site{
				Name:    fmt.Sprintf("s%d", i),
				Domains: []string{fmt.Sprintf("s%d.test", i)},
				Path:    fmt.Sprintf("/tmp/s%d", i),
			})
		}
		return reg
	}

	if err := SaveSites(mk(10)); err != nil {
		t.Fatalf("seed SaveSites: %v", err)
	}

	var writers sync.WaitGroup
	for w := 0; w < 8; w++ {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			for i := 0; i < 60; i++ {
				if err := SaveSites(mk(1 + (i+w)%25)); err != nil {
					t.Errorf("SaveSites: %v", err)
					return
				}
			}
		}(w)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := LoadSites(); err != nil {
					t.Errorf("LoadSites during concurrent writes: %v", err)
					return
				}
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()

	if _, err := LoadSites(); err != nil {
		t.Fatalf("final LoadSites: %v", err)
	}
	entries, err := os.ReadDir(DataDir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("atomic write left a temp file behind: %s", e.Name())
		}
	}
}
