package pkg

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_Fetch(t *testing.T) {
	t.Run("test fetch caches entry", func(t *testing.T) {
		// Given
		serverCallCounter := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			serverCallCounter += 1
			_, _ = w.Write([]byte(fmt.Sprintf("Hello World")))
		}))
		defer server.Close()

		leCache := NewCache(10 * time.Minute)

		// Then
		for range 100 {
			data, err := leCache.Fetch(context.Background(), server.URL)
			assert.NoError(t, err)
			assert.Equal(t, "Hello World", string(data))
			assert.Equal(t, 1, serverCallCounter)
		}
	})
	t.Run("test fetch error not being cached", func(t *testing.T) {
		// Setup
		serverCallCounter := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if serverCallCounter == 0 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			serverCallCounter += 1
			_, _ = w.Write([]byte(fmt.Sprintf("Hello World")))
		}))
		defer server.Close()

		leCache := NewCache(10 * time.Minute)

		// Test error is not cached
		data, err := leCache.Fetch(context.Background(), server.URL)
		assert.Error(t, err)
		assert.Equal(t, "", string(data))
		assert.Equal(t, 1, serverCallCounter)

		// Test no error
		data, err = leCache.Fetch(context.Background(), server.URL)
		assert.NoError(t, err)
		assert.Equal(t, "Hello World", string(data))
		assert.Equal(t, 2, serverCallCounter)
	})
}

func TestCache_Stats(t *testing.T) {
	// Setup
	serverCallCounter := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		serverCallCounter += 1
		_, _ = w.Write([]byte(fmt.Sprintf("Hello world")))
	}))
	defer server.Close()

	leCache := NewCache(10 * time.Minute)

	// Test
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			_, err := leCache.Fetch(context.Background(), server.URL)
			if err != nil {
				t.Error(fmt.Sprintf("Unknown error occured: %v", err))
			}
			wg.Done()
		}()
		wg.Wait()
	}
	hits, misses, entries := leCache.Stats()

	// Assert
	assert.Equal(t, 99, hits)
	assert.Equal(t, 1, misses)
	assert.Equal(t, 1, entries)
	assert.Equal(t, 1, serverCallCounter)
}

func TestCache_Concurrency(t *testing.T) {
	// Simulate concurrent fetching
}

func TestCacheTTL(t *testing.T) {
	serverCallCounter := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		serverCallCounter += 1
		_, _ = w.Write([]byte(fmt.Sprintf("Hello world %d", serverCallCounter)))
	}))
	defer server.Close()
	leCache := NewCache(100 * time.Millisecond)

	// Test first request, it should fetch the same URL as ttl is not expired
	serverUrl := server.URL
	data, err := leCache.Fetch(context.Background(), serverUrl)
	assert.NoError(t, err)
	assert.Equal(t, "Hello world 1", string(data))

	data, err = leCache.Fetch(context.Background(), serverUrl)
	assert.NoError(t, err)
	assert.Equal(t, "Hello world 1", string(data))
	assert.Equal(t, 1, serverCallCounter)

	time.Sleep(100 * time.Millisecond)

	// Test second fetch
	data, err = leCache.Fetch(context.Background(), serverUrl, 250*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, "Hello world 2", string(data))
	assert.Equal(t, 2, serverCallCounter)

	for range 100 {
		data, err = leCache.Fetch(context.Background(), serverUrl, 250*time.Millisecond)
		assert.NoError(t, err)
		assert.Equal(t, "Hello world 2", string(data))
		assert.Equal(t, 2, serverCallCounter)
	}
	// Sleep to pass TTL override time
	time.Sleep(100 * time.Millisecond)

	data, err = leCache.Fetch(context.Background(), serverUrl, 250*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, "Hello world 2", string(data))
	assert.Equal(t, 2, serverCallCounter)

	// Final result test
	time.Sleep(150 * time.Millisecond)
	data, err = leCache.Fetch(context.Background(), serverUrl, 250*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, "Hello world 3", string(data))
	assert.Equal(t, 3, serverCallCounter)

}
