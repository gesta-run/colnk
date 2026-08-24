package filesystem

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestMetadataCacheHitInvalidationAndExpiry(t *testing.T) {
	cache := newMetadataCache(2, 10*time.Millisecond)
	cache.putAttr("a/file", &protocol.FileAttr{Size: 7})
	if attr, ok := cache.getAttr("a/file"); !ok || attr.Size != 7 {
		t.Fatal("metadata cache missed a hot attribute")
	}
	cache.invalidate("a")
	if _, ok := cache.getAttr("a/file"); ok {
		t.Fatal("metadata cache retained an invalidated child")
	}
	cache.putDir("directory", []protocol.DirEntry{{Name: "file", Attr: protocol.FileAttr{Size: 9}}})
	if attr, ok := cache.getAttr("directory/file"); !ok || attr.Size != 9 {
		t.Fatal("metadata cache missed attributes supplied by the parent directory")
	}
	time.Sleep(15 * time.Millisecond)
	if _, ok := cache.getDir("directory"); ok {
		t.Fatal("metadata cache retained an expired directory")
	}
	if _, ok := cache.getAttr("directory/file"); ok {
		t.Fatal("metadata cache retained expired directory attributes")
	}
}

func TestMetadataCacheStoresDirectoryAsOneEntry(t *testing.T) {
	cache := newMetadataCache(2, time.Minute)
	cache.putDir("directory", []protocol.DirEntry{
		{Name: "one", Attr: protocol.FileAttr{Size: 1}},
		{Name: "two", Attr: protocol.FileAttr{Size: 2}},
		{Name: "three", Attr: protocol.FileAttr{Mode: uint32(os.ModeSymlink), Size: 3}, LinkTarget: "one"},
	})
	if len(cache.entries) != 1 {
		t.Fatalf("directory consumed %d cache entries", len(cache.entries))
	}
	if attr, ok := cache.getAttr("directory/three"); !ok || attr.Size != 3 {
		t.Fatal("directory attribute index missed an entry")
	}
	if target, ok := cache.getLink("directory/three"); !ok || target != "one" {
		t.Fatal("directory symlink index missed an entry")
	}
}

func TestMetadataCacheExactInvalidationPreservesSiblings(t *testing.T) {
	cache := newMetadataCache(4, time.Minute)
	cache.putDir("", []protocol.DirEntry{{Name: "directory", Attr: protocol.FileAttr{Size: 1}}})
	cache.putDir("directory", []protocol.DirEntry{{Name: "file", Attr: protocol.FileAttr{Size: 2}}})
	cache.invalidateExact("")
	if _, ok := cache.getDir(""); ok {
		t.Fatal("exact invalidation retained the target")
	}
	if _, ok := cache.getDir("directory"); !ok {
		t.Fatal("exact invalidation removed a child directory")
	}
}

func TestMetadataCacheCapacity(t *testing.T) {
	cache := newMetadataCache(2, time.Minute)
	for _, name := range []string{"one", "two"} {
		cache.putAttr(name, &protocol.FileAttr{})
	}
	_, _ = cache.getAttr("one")
	cache.putAttr("three", &protocol.FileAttr{})
	if len(cache.entries) != 2 {
		t.Fatalf("cache exceeded capacity: %d", len(cache.entries))
	}
	if _, ok := cache.getAttr("two"); ok {
		t.Fatal("least recently used cache entry was not evicted")
	}
}

func TestMetadataCacheNegativeEntries(t *testing.T) {
	cache := newMetadataCache(4, time.Minute)
	cache.putMissing("directory/missing")
	if !cache.isMissing("directory/missing") {
		t.Fatal("negative cache missed an explicit entry")
	}
	cache.putAttr("directory/missing", &protocol.FileAttr{Size: 1})
	if cache.isMissing("directory/missing") {
		t.Fatal("positive attribute did not replace a negative entry")
	}
	cache.putDir("directory", []protocol.DirEntry{{Name: "present"}})
	if !cache.isMissing("directory/absent") {
		t.Fatal("directory cache did not prove a child was absent")
	}
	if cache.isMissing("unknown/absent") {
		t.Fatal("unknown parent produced a false negative entry")
	}
}

func TestReadBlockCacheLoadsOnceForConcurrentMisses(t *testing.T) {
	cache := newReadBlockCache(2, time.Minute)
	var loads atomic.Int32
	loader := func() ([]byte, error) {
		loads.Add(1)
		time.Sleep(10 * time.Millisecond)
		return []byte("block"), nil
	}
	var wait sync.WaitGroup
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			data, err := cache.load(context.Background(), "file", 0, loader)
			if err != nil || string(data) != "block" {
				t.Errorf("unexpected load result %q: %v", data, err)
			}
		}()
	}
	wait.Wait()
	if loads.Load() != 1 {
		t.Fatalf("cache performed %d loads", loads.Load())
	}
}

func TestReadBlockCacheInvalidationAndCapacity(t *testing.T) {
	cache := newReadBlockCache(2, time.Minute)
	loads := 0
	load := func(value string) func() ([]byte, error) {
		return func() ([]byte, error) { loads++; return []byte(value), nil }
	}
	_, _ = cache.load(context.Background(), "one", 0, load("one"))
	_, _ = cache.load(context.Background(), "two", 0, load("two"))
	_, _ = cache.load(context.Background(), "three", 0, load("three"))
	if len(cache.entries) != 2 {
		t.Fatalf("cache exceeded capacity: %d", len(cache.entries))
	}
	cache.invalidate("two")
	_, _ = cache.load(context.Background(), "two", 0, load("two-new"))
	if loads != 4 {
		t.Fatalf("invalidation did not reload data: %d", loads)
	}
}

func TestReadBlockCacheInvalidatesInFlightChildLoad(t *testing.T) {
	cache := newReadBlockCache(2, time.Minute)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.load(context.Background(), "dir/file", 0, func() ([]byte, error) {
			close(started)
			<-release
			return []byte("old"), nil
		})
	}()
	<-started
	cache.invalidate("dir")
	close(release)
	<-done
	loads := 0
	data, err := cache.load(context.Background(), "dir/file", 0, func() ([]byte, error) {
		loads++
		return []byte("new"), nil
	})
	if err != nil || string(data) != "new" || loads != 1 {
		t.Fatalf("stale in-flight data was cached: %q, loads=%d, err=%v", data, loads, err)
	}
}
