package pkg

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_Fetch(t *testing.T) {
	// test fetchData
}

func TestCache_Stats(t *testing.T) {
	leCache := NewCache(10 * time.Minute)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			_, err := leCache.Fetch(context.Background(), "https://www.computerwoche.de/article/4049065/lernplattformen-fur-unternehmen-ein-kaufratgeber.html")
			if err == nil {
				fmt.Printf("Got the data: %d\n", i)
			} else {
				fmt.Printf("Got an error: %d\n", i)
			}
			wg.Done()
		}()
		wg.Wait()
	}
	hits, misses, entries := leCache.Stats()
	assert.Equal(t, hits, 99)
	assert.Equal(t, misses, 1)
	assert.Equal(t, entries, 1)
}

func TestCache_Concurrency(t *testing.T) {
	// Simulate concurrent fetching
}

func TestCacheTTL(t *testing.T) {
	// test ttl expiration, re fetchData
}
