package filesystem

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
