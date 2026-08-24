package filesystem

import (
	"context"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

type metadataCache struct {
	mu       sync.Mutex
	entries  map[string]metadataEntry
	capacity int
	ttl      time.Duration
	sequence uint64
}

type metadataEntry struct {
	path     string
	attr     *protocol.FileAttr
	dir      []protocol.DirEntry
	dirAttrs map[string]protocol.FileAttr
	dirLinks map[string]string
	link     *string
	missing  bool
	expires  time.Time
	sequence uint64
}

func newMetadataCache(capacity int, ttl time.Duration) *metadataCache {
	return &metadataCache{entries: make(map[string]metadataEntry), capacity: capacity, ttl: ttl}
}

func (cache *metadataCache) getAttr(path string) (*protocol.FileAttr, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.getLocked("attr:" + path)
	if ok && entry.attr != nil {
		value := *entry.attr
		return &value, true
	}
	parent, name := parentAndName(path)
	entry, ok = cache.getLocked("dir:" + parent)
	if !ok || entry.dirAttrs == nil {
		return nil, false
	}
	value, ok := entry.dirAttrs[name]
	if !ok {
		return nil, false
	}
	return &value, true
}

func (cache *metadataCache) putAttr(path string, attr *protocol.FileAttr) {
	if attr == nil {
		return
	}
	cache.mu.Lock()
	delete(cache.entries, "missing:"+path)
	cache.mu.Unlock()
	value := *attr
	cache.put("attr:"+path, metadataEntry{path: path, attr: &value})
}

func (cache *metadataCache) isMissing(path string) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if entry, ok := cache.getLocked("missing:" + path); ok && entry.missing {
		return true
	}
	if path == "" {
		return false
	}
	parent, name := parentAndName(path)
	entry, ok := cache.getLocked("dir:" + parent)
	if !ok || entry.dirAttrs == nil {
		return false
	}
	_, exists := entry.dirAttrs[name]
	return !exists
}

func (cache *metadataCache) putMissing(path string) {
	cache.put("missing:"+path, metadataEntry{path: path, missing: true})
}

func (cache *metadataCache) getLink(path string) (string, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.getLocked("link:" + path)
	if ok && entry.link != nil {
		return *entry.link, true
	}
	parent, name := parentAndName(path)
	entry, ok = cache.getLocked("dir:" + parent)
	if !ok || entry.dirLinks == nil {
		return "", false
	}
	target, ok := entry.dirLinks[name]
	return target, ok
}

func (cache *metadataCache) putLink(path string, target string) {
	value := target
	cache.put("link:"+path, metadataEntry{path: path, link: &value})
}

func (cache *metadataCache) getDir(path string) ([]protocol.DirEntry, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.getLocked("dir:" + path)
	if !ok || entry.dir == nil {
		return nil, false
	}
	return append([]protocol.DirEntry(nil), entry.dir...), true
}

func (cache *metadataCache) putDir(path string, entries []protocol.DirEntry) {
	attributes := make(map[string]protocol.FileAttr, len(entries))
	links := make(map[string]string)
	for _, entry := range entries {
		attributes[entry.Name] = entry.Attr
		if entry.Attr.Mode&uint32(os.ModeSymlink) != 0 && entry.LinkTarget != "" {
			links[entry.Name] = entry.LinkTarget
		}
	}
	cache.put("dir:"+path, metadataEntry{
		path: path, dir: append([]protocol.DirEntry(nil), entries...), dirAttrs: attributes, dirLinks: links,
	})
}

func parentAndName(value string) (string, string) {
	parent, name := path.Split(value)
	return strings.TrimSuffix(parent, "/"), name
}

func (cache *metadataCache) invalidate(path string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for key, entry := range cache.entries {
		if entry.path == path || pathPrefix(entry.path, path) {
			delete(cache.entries, key)
		}
	}
}

func (cache *metadataCache) invalidateExact(path string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for key, entry := range cache.entries {
		if entry.path == path {
			delete(cache.entries, key)
		}
	}
}

func (cache *metadataCache) getLocked(key string) (metadataEntry, bool) {
	entry, ok := cache.entries[key]
	if !ok {
		return metadataEntry{}, false
	}
	if time.Now().After(entry.expires) {
		delete(cache.entries, key)
		return metadataEntry{}, false
	}
	cache.sequence++
	entry.sequence = cache.sequence
	cache.entries[key] = entry
	return entry, true
}

func (cache *metadataCache) put(key string, entry metadataEntry) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.sequence++
	entry.sequence = cache.sequence
	entry.expires = time.Now().Add(cache.ttl)
	cache.entries[key] = entry
	for len(cache.entries) > cache.capacity {
		cache.evictOldestLocked()
	}
}

func (cache *metadataCache) evictOldestLocked() {
	var oldestKey string
	var oldestSequence uint64
	for key, entry := range cache.entries {
		if oldestKey == "" || entry.sequence < oldestSequence {
			oldestKey = key
			oldestSequence = entry.sequence
		}
	}
	delete(cache.entries, oldestKey)
}

func pathPrefix(candidate, parent string) bool {
	if parent == "" {
		return candidate != ""
	}
	return len(candidate) > len(parent) && candidate[:len(parent)] == parent && candidate[len(parent)] == '/'
}

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
