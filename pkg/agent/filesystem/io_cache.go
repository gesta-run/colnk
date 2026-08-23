package filesystem

import (
	"context"
	"sync"
	"time"
)

type readBlockKey struct {
	path   string
	offset int64
}

type readBlockEntry struct {
	data     []byte
	expires  time.Time
	sequence uint64
}

type readBlockFlight struct {
	done chan struct{}
	data []byte
	err  error
}

type readBlockCache struct {
	mu          sync.Mutex
	entries     map[readBlockKey]readBlockEntry
	flights     map[readBlockKey]*readBlockFlight
	generations map[string]uint64
	capacity    int
	ttl         time.Duration
	sequence    uint64
}

func newReadBlockCache(capacity int, ttl time.Duration) *readBlockCache {
	return &readBlockCache{
		entries: make(map[readBlockKey]readBlockEntry), flights: make(map[readBlockKey]*readBlockFlight),
		generations: make(map[string]uint64), capacity: capacity, ttl: ttl,
	}
}

func (cache *readBlockCache) load(ctx context.Context, path string, offset int64, loader func() ([]byte, error)) ([]byte, error) {
	key := readBlockKey{path: path, offset: offset}
	cache.mu.Lock()
	if entry, ok := cache.entries[key]; ok {
		if time.Now().Before(entry.expires) {
			cache.sequence++
			entry.sequence = cache.sequence
			cache.entries[key] = entry
			cache.mu.Unlock()
			return entry.data, nil
		}
		delete(cache.entries, key)
	}
	if flight := cache.flights[key]; flight != nil {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return flight.data, flight.err
		}
	}
	flight := &readBlockFlight{done: make(chan struct{})}
	cache.flights[key] = flight
	generation := cache.generations[path]
	cache.mu.Unlock()

	data, err := loader()
	cache.mu.Lock()
	flight.data, flight.err = data, err
	delete(cache.flights, key)
	if err == nil && cache.generations[path] == generation {
		cache.sequence++
		cache.entries[key] = readBlockEntry{
			data: data, expires: time.Now().Add(cache.ttl), sequence: cache.sequence,
		}
		for len(cache.entries) > cache.capacity {
			cache.evictOldestLocked()
		}
	}
	close(flight.done)
	cache.mu.Unlock()
	return data, err
}

func (cache *readBlockCache) invalidate(path string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.generations[path]++
	invalidatedFlights := make(map[string]struct{})
	for key := range cache.flights {
		if key.path == path || pathPrefix(key.path, path) {
			invalidatedFlights[key.path] = struct{}{}
		}
	}
	for flightPath := range invalidatedFlights {
		if flightPath != path {
			cache.generations[flightPath]++
		}
	}
	for key := range cache.entries {
		if key.path == path || pathPrefix(key.path, path) {
			delete(cache.entries, key)
		}
	}
}

func (cache *readBlockCache) evictOldestLocked() {
	var oldestKey readBlockKey
	var oldestSequence uint64
	found := false
	for key, entry := range cache.entries {
		if !found || entry.sequence < oldestSequence {
			oldestKey, oldestSequence, found = key, entry.sequence, true
		}
	}
	if found {
		delete(cache.entries, oldestKey)
	}
}
