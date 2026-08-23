//go:build linux

package filesystem

import (
	"sync"
	"time"

	"github.com/gesta-run/colnk/pkg/agent/remote"
	"golang.org/x/sync/semaphore"
)

const (
	maxPendingWriteBlocks = 8
	maxTotalWriteBlocks   = 128
	fuseUnmountTimeout    = 2 * time.Second
)

type remoteFS struct {
	remote      *remote.Remote
	mu          sync.RWMutex
	nodes       map[string]*remoteNode
	cache       *metadataCache
	dataCache   *readBlockCache
	writeBudget *semaphore.Weighted
	mountpoint  string
}

type remoteNode struct {
	filesystem  *remoteFS
	path        string
	writeMu     sync.Mutex
	writeBlocks map[int64]*pendingWriteBlock
	writeMtime  *time.Time
}
