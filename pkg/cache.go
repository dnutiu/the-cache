package pkg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// cacheListener is a struct used to subscribe to the cache for request deduplication
type cacheListener struct {
	isCompleted chan int
	isError     chan error
}

// cacheEntry represents an internal entry of the cache.
type cacheEntry struct {
	// isCurrentlyBeingProcessed is a flag indicating if the cache entry is currently being processed.
	isCurrentlyBeingProcessed bool
	// cacheListeners holds the listeners interested to learn about this cache.
	cacheListeners []cacheListener
	// expiryAt is the timestamp of when this cache entry should expire.
	expiryAt int64
	// dataMutex protects access to the data field.
	dataMutex sync.Mutex
	// data is the cached data.
	data []byte
}

// Cache offers a simple API for caching stuff.
type Cache struct {
	// defaultTTL represents the default given TTL.
	defaultTTL time.Duration
	// internalCache holds the internal cache data
	internalCache sync.Map
	// httpClient holds an instance of the HTTP client
	httpClient *http.Client
	// hitsCounter tracks how many cache hits occurred for this cache.
	hitsCounter int
	// missesCounter tracks how many cache misses occurred for this cache.
	missesCounter int
}

// NewCache builds a new Cache.
// defaultTTL is the default time to live for each entry in the cache.
func NewCache(defaultTTL time.Duration) *Cache {
	return &Cache{
		defaultTTL: defaultTTL,
		httpClient: &http.Client{},
	}
}

// Fetch fetches the data from the given url.
// An optional ttlOverride can be given in order to override the ttl for the cache entry.
func (c *Cache) Fetch(ctx context.Context, url string, ttlOverride ...time.Duration) ([]byte, error) {
	ttlToSet := c.defaultTTL
	if len(ttlOverride) > 0 {
		ttlToSet = ttlOverride[0]
	}

	theInternalCacheEntry := cacheEntry{
		cacheListeners:            make([]cacheListener, 0),
		expiryAt:                  time.Now().Add(ttlToSet).UnixNano(),
		isCurrentlyBeingProcessed: true,
		data:                      nil,
	}

	// Load the cached value or store a placeholder to dedupe requests.
	mapEntry, ok := c.internalCache.LoadOrStore(url, &theInternalCacheEntry)
	if ok {
		// We have gotten something from the internal cache, need to validate it.
		cachedEntry, ok := mapEntry.(*cacheEntry)

		// Cache entry is not expired
		if ok && time.Now().UnixNano() < cachedEntry.expiryAt {

			// Attempt to grab its data
			cachedEntry.dataMutex.Lock()
			cacheEntryData := cachedEntry.data
			cachedEntry.dataMutex.Unlock()

			// If it doesn't have data means it is inflight.
			if cacheEntryData == nil {

				// Register this fetch as a listener
				listener := cacheListener{
					isCompleted: make(chan int),
					isError:     make(chan error),
				}

				cachedEntry.dataMutex.Lock()
				if !cachedEntry.isCurrentlyBeingProcessed {
					if cachedEntry.data == nil {
						cachedEntry.dataMutex.Unlock()

						// Clean up listener resources, no longer registering
						close(listener.isError)
						close(listener.isCompleted)

						return nil, errors.New("failed to grab entry from cache")
					} else {
						cachedEntry.dataMutex.Unlock()

						// Clean up listener resources, no longer registering
						close(listener.isError)
						close(listener.isCompleted)

						return cachedEntry.data, nil
					}
				}
				cachedEntry.cacheListeners = append(cachedEntry.cacheListeners, listener)
				cachedEntry.dataMutex.Unlock()

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case val := <-listener.isError:
					return nil, val
				case <-listener.isCompleted:
					return cachedEntry.data, nil

				}
			}

			// Has data, return it
			c.hitsCounter += 1
			return cachedEntry.data, nil
		}

		// If cache entry is expired, request dedup for ongoing stuff might fail here, should not happen often
		if ok && time.Now().UnixNano() >= cachedEntry.expiryAt {
			// Maybe I Could attempt to set data to nil and update timestamp of internal cache, then proceed to refetch here
			c.internalCache.Delete(url)
			return c.Fetch(ctx, url, ttlToSet)
		}
	}

	// We need to fetch data here because we don't have it in the cache.
	data, err := c.fetchData(ctx, url)
	if err != nil {
		// We had an error, first delete the cache key then announce the others.
		c.internalCache.Delete(url)

		// Announce listeners of errors if possible
		theInternalCacheEntry.dataMutex.Lock()
		theInternalCacheEntry.isCurrentlyBeingProcessed = false
		for _, listener := range theInternalCacheEntry.cacheListeners {
			select {
			case listener.isError <- err:
				slog.Debug("sent close message to listener")
			default:
				slog.Debug("could not sent close message to listener")
			}
			close(listener.isError)
			close(listener.isCompleted)
		}
		theInternalCacheEntry.dataMutex.Unlock()
		return nil, err
	}

	// Set the data
	theInternalCacheEntry.dataMutex.Lock()
	theInternalCacheEntry.data = data
	theInternalCacheEntry.dataMutex.Unlock()

	// Announce listeners that data is available
	theInternalCacheEntry.dataMutex.Lock()
	theInternalCacheEntry.isCurrentlyBeingProcessed = false
	for _, listener := range theInternalCacheEntry.cacheListeners {
		select {
		case listener.isCompleted <- 1:
			slog.Debug("sent completed message to listener")
		default:
			slog.Debug("could not sent completed message to listener")
		}
		close(listener.isCompleted)
		close(listener.isError)
	}
	theInternalCacheEntry.dataMutex.Unlock()
	c.missesCounter += 1
	return data, nil
}

// fetchData fetches the bytes of the given url, if it fails it will return an error.
func (c *Cache) fetchData(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// We assume that any status code which is not 200 is an error
	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("unexpected status code %d", resp.StatusCode))
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to close response body for %s", url))
		}
	}(resp.Body)
	bytes, err := ReadAll(ctx, resp.Body)
	return bytes, err
}

// Stats returns the statistics of the cache.
func (c *Cache) Stats() (hits int, misses int, entries int) {
	// Note: we use range here to check if keys have been expired to report the correct entries counter.
	// This may not be ideal, could use a background goroutine.
	c.internalCache.Range(func(key, value interface{}) bool {
		entry, ok := value.(*cacheEntry)
		if ok {
			if time.Now().Unix() < entry.expiryAt {
				entries += 1
			}
		}
		return true
	})
	return c.hitsCounter, c.missesCounter, entries
}
