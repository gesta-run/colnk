//go:build linux

package filesystem

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
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

func (f *remoteFS) Root() (fs.Node, error) {
	return f.node(""), nil
}

func (f *remoteFS) invalidateChange(remotePath string) {
	f.cache.invalidate(remotePath)
	f.dataCache.invalidate(remotePath)
	parent := path.Dir(remotePath)
	if parent == "." {
		parent = ""
	}
	f.cache.invalidateExact(parent)
}

func (f *remoteFS) updateChangedAttr(remotePath string, attr *protocol.FileAttr) {
	f.invalidateChange(remotePath)
	f.cache.putAttr(remotePath, attr)
}

func (f *remoteFS) node(remotePath string) *remoteNode {
	f.mu.Lock()
	defer f.mu.Unlock()
	if node := f.nodes[remotePath]; node != nil {
		return node
	}
	node := &remoteNode{filesystem: f, path: remotePath}
	f.nodes[remotePath] = node
	return node
}

func (n *remoteNode) remotePath() string {
	n.filesystem.mu.RLock()
	defer n.filesystem.mu.RUnlock()
	return n.path
}

func (n *remoteNode) Forget() {
	n.filesystem.mu.Lock()
	defer n.filesystem.mu.Unlock()
	if n.filesystem.nodes[n.path] == n {
		delete(n.filesystem.nodes, n.path)
	}
}

func (f *remoteFS) renameNodes(oldPath, newPath string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for currentPath, node := range f.nodes {
		if currentPath != oldPath && !strings.HasPrefix(currentPath, oldPath+"/") {
			continue
		}
		updatedPath := newPath + strings.TrimPrefix(currentPath, oldPath)
		delete(f.nodes, currentPath)
		node.path = updatedPath
		f.nodes[updatedPath] = node
	}
}

func (f *remoteFS) removeNodes(remotePath string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for currentPath := range f.nodes {
		if currentPath == remotePath || strings.HasPrefix(currentPath, remotePath+"/") {
			delete(f.nodes, currentPath)
		}
	}
}

func (n *remoteNode) Attr(ctx context.Context, attr *fuse.Attr) error {
	remotePath := n.remotePath()
	if cached, ok := n.filesystem.cache.getAttr(remotePath); ok {
		fillAttr(attr, n.pendingAttr(cached), n.filesystem.cache.ttl)
		return nil
	}
	if n.filesystem.cache.isMissing(remotePath) {
		return syscall.ENOENT
	}
	response, _, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "stat", Path: remotePath}, nil)
	if err != nil {
		if isRemoteErrno(err, syscall.ENOENT) {
			n.filesystem.cache.putMissing(remotePath)
		}
		return fuseError(err)
	}
	n.filesystem.cache.putAttr(remotePath, response.Attr)
	fillAttr(attr, n.pendingAttr(response.Attr), n.filesystem.cache.ttl)
	return nil
}

func (n *remoteNode) Setattr(ctx context.Context, request *fuse.SetattrRequest, response *fuse.SetattrResponse) error {
	if request.Valid.Size() || request.Valid.Mode() {
		if err := n.flushPending(ctx); err != nil {
			return fuseError(err)
		}
	}
	var latest *protocol.FileAttr
	if request.Valid.Size() {
		result, _, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "truncate", Path: n.remotePath(), Offset: int64(request.Size)}, nil)
		if err != nil {
			return fuseError(err)
		}
		latest = result.Attr
	}
	if request.Valid.Mode() {
		result, _, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "chmod", Path: n.remotePath(), Mode: uint32(request.Mode.Perm())}, nil)
		if err != nil {
			return fuseError(err)
		}
		latest = result.Attr
	}
	if request.Valid.Mtime() {
		if n.deferMtime(request.Mtime) {
			latest = nil
		} else {
			result, _, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "chtimes", Path: n.remotePath(), ModTimeNS: request.Mtime.UnixNano()}, nil)
			if err != nil {
				return fuseError(err)
			}
			latest = result.Attr
		}
	}
	if latest != nil {
		n.filesystem.updateChangedAttr(n.remotePath(), latest)
		fillAttr(&response.Attr, latest, n.filesystem.cache.ttl)
		return nil
	}
	return n.Attr(ctx, &response.Attr)
}

func (n *remoteNode) pendingAttr(source *protocol.FileAttr) *protocol.FileAttr {
	if source == nil {
		return nil
	}
	value := *source
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	for blockOffset, block := range n.writeBlocks {
		for _, dirty := range block.dirty {
			end := uint64(blockOffset + int64(dirty.end))
			if end > value.Size {
				value.Size = end
			}
		}
	}
	if n.writeMtime != nil {
		value.ModTimeNS = n.writeMtime.UnixNano()
	}
	return &value
}

func fillAttr(target *fuse.Attr, source *protocol.FileAttr, ttl time.Duration) {
	if source == nil {
		return
	}
	target.Inode = source.Inode
	target.Size = source.Size
	target.Mode = os.FileMode(source.Mode)
	target.Mtime = time.Unix(0, source.ModTimeNS)
	target.Ctime = target.Mtime
	target.Uid = source.UID
	target.Gid = source.GID
	target.Valid = ttl
}

func direntType(mode os.FileMode) fuse.DirentType {
	switch {
	case mode.IsDir():
		return fuse.DT_Dir
	case mode&os.ModeSymlink != 0:
		return fuse.DT_Link
	default:
		return fuse.DT_File
	}
}

func fuseError(err error) error {
	if err == nil {
		return nil
	}
	var remoteError *remote.RemoteError
	if errors.As(err, &remoteError) {
		return remoteError.Code
	}
	if errors.Is(err, context.Canceled) {
		return syscall.EINTR
	}
	return syscall.EIO
}

func isRemoteErrno(err error, code syscall.Errno) bool {
	var remoteError *remote.RemoteError
	return errors.As(err, &remoteError) && remoteError.Code == code
}

func (n *remoteNode) Lookup(ctx context.Context, name string) (fs.Node, error) {
	child := n.filesystem.node(path.Join(n.remotePath(), name))
	if err := child.Attr(ctx, &fuse.Attr{}); err != nil {
		return nil, err
	}
	return child, nil
}

func (n *remoteNode) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	remotePath := n.remotePath()
	if cached, ok := n.filesystem.cache.getDir(remotePath); ok {
		return fuseDirEntries(cached), nil
	}
	entries, err := n.filesystem.remote.ReadDir(ctx, remotePath)
	if err != nil {
		return nil, fuseError(err)
	}
	n.filesystem.cache.putDir(remotePath, entries)
	return fuseDirEntries(entries), nil
}

func fuseDirEntries(values []protocol.DirEntry) []fuse.Dirent {
	entries := make([]fuse.Dirent, 0, len(values))
	for _, entry := range values {
		entries = append(entries, fuse.Dirent{Name: entry.Name, Type: direntType(os.FileMode(entry.Attr.Mode))})
	}
	return entries
}

func (n *remoteNode) Open(_ context.Context, _ *fuse.OpenRequest, _ *fuse.OpenResponse) (fs.Handle, error) {
	return n, nil
}

func (n *remoteNode) Read(ctx context.Context, request *fuse.ReadRequest, response *fuse.ReadResponse) error {
	if err := n.flushPending(ctx); err != nil {
		return fuseError(err)
	}
	remotePath := n.remotePath()
	response.Data = make([]byte, 0, request.Size)
	for offset, remaining := request.Offset, request.Size; remaining > 0; {
		blockOffset := offset / int64(protocol.MaxPayloadBytes) * int64(protocol.MaxPayloadBytes)
		data, err := n.filesystem.dataCache.load(ctx, remotePath, blockOffset, func() ([]byte, error) {
			_, block, loadErr := n.filesystem.remote.File(ctx, protocol.Request{
				Operation: "read", Path: remotePath, Offset: blockOffset, Size: protocol.MaxPayloadBytes,
			}, nil)
			return block, loadErr
		})
		if err != nil {
			return fuseError(err)
		}
		inside := int(offset - blockOffset)
		if inside >= len(data) {
			break
		}
		count := min(remaining, len(data)-inside)
		response.Data = append(response.Data, data[inside:inside+count]...)
		offset += int64(count)
		remaining -= count
		if len(data) < protocol.MaxPayloadBytes {
			break
		}
	}
	return nil
}

func (n *remoteNode) Write(ctx context.Context, request *fuse.WriteRequest, response *fuse.WriteResponse) error {
	n.filesystem.invalidateChange(n.remotePath())
	if err := n.bufferWrite(ctx, request.Offset, request.Data); err != nil {
		return fuseError(err)
	}
	response.Size = len(request.Data)
	return nil
}

func (n *remoteNode) Fsync(ctx context.Context, _ *fuse.FsyncRequest) error {
	synced, err := n.flushPendingSync(ctx)
	if err != nil {
		return fuseError(err)
	}
	if synced {
		return nil
	}
	_, _, err = n.filesystem.remote.File(ctx, protocol.Request{Operation: "fsync", Path: n.remotePath()}, nil)
	return fuseError(err)
}

func (n *remoteNode) Flush(ctx context.Context, _ *fuse.FlushRequest) error {
	return fuseError(n.flushPending(ctx))
}

func (n *remoteNode) Release(ctx context.Context, _ *fuse.ReleaseRequest) error {
	return fuseError(n.flushPending(ctx))
}

func (n *remoteNode) Mkdir(ctx context.Context, request *fuse.MkdirRequest) (fs.Node, error) {
	childPath := path.Join(n.remotePath(), request.Name)
	response, _, err := n.filesystem.remote.File(ctx, protocol.Request{
		Operation: "mkdir", Path: childPath, Mode: uint32(request.Mode.Perm()),
	}, nil)
	if err != nil {
		return nil, fuseError(err)
	}
	n.filesystem.updateChangedAttr(childPath, response.Attr)
	return n.filesystem.node(childPath), nil
}

func (n *remoteNode) Create(ctx context.Context, request *fuse.CreateRequest, _ *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	child := n.filesystem.node(path.Join(n.remotePath(), request.Name))
	response, _, err := n.filesystem.remote.File(ctx, protocol.Request{
		Operation: "create", Path: child.remotePath(), Mode: uint32(request.Mode.Perm()),
	}, nil)
	if err != nil {
		return nil, nil, fuseError(err)
	}
	n.filesystem.updateChangedAttr(child.remotePath(), response.Attr)
	return child, child, nil
}

func (n *remoteNode) Remove(ctx context.Context, request *fuse.RemoveRequest) error {
	childPath := path.Join(n.remotePath(), request.Name)
	if err := n.filesystem.node(childPath).flushPending(ctx); err != nil {
		return fuseError(err)
	}
	_, _, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "remove", Path: childPath}, nil)
	if err == nil {
		n.filesystem.removeNodes(childPath)
		n.filesystem.invalidateChange(childPath)
	}
	return fuseError(err)
}

func (n *remoteNode) Rename(ctx context.Context, request *fuse.RenameRequest, newDirectory fs.Node) error {
	target, ok := newDirectory.(*remoteNode)
	if !ok {
		return syscall.EIO
	}
	oldPath := path.Join(n.remotePath(), request.OldName)
	newPath := path.Join(target.remotePath(), request.NewName)
	if err := n.filesystem.node(oldPath).flushPending(ctx); err != nil {
		return fuseError(err)
	}
	if err := n.filesystem.node(newPath).flushPending(ctx); err != nil {
		return fuseError(err)
	}
	response, _, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "rename", Path: oldPath, NewPath: newPath}, nil)
	if err == nil {
		n.filesystem.renameNodes(oldPath, newPath)
		n.filesystem.invalidateChange(oldPath)
		n.filesystem.updateChangedAttr(newPath, response.Attr)
	}
	return fuseError(err)
}

func (n *remoteNode) Symlink(ctx context.Context, request *fuse.SymlinkRequest) (fs.Node, error) {
	child := n.filesystem.node(path.Join(n.remotePath(), request.NewName))
	target := localSymlinkTarget(n.filesystem.mountpoint, request.Target)
	response, _, err := n.filesystem.remote.File(ctx, protocol.Request{
		Operation: "symlink", Path: child.remotePath(), Target: target,
	}, nil)
	if err != nil {
		return nil, fuseError(err)
	}
	n.filesystem.updateChangedAttr(child.remotePath(), response.Attr)
	return child, nil
}

func (n *remoteNode) Readlink(ctx context.Context, _ *fuse.ReadlinkRequest) (string, error) {
	if target, ok := n.filesystem.cache.getLink(n.remotePath()); ok {
		return mountSymlinkTarget(n.filesystem.mountpoint, target), nil
	}
	_, data, err := n.filesystem.remote.File(ctx, protocol.Request{Operation: "readlink", Path: n.remotePath()}, nil)
	if err != nil {
		return "", fuseError(err)
	}
	target := string(data)
	n.filesystem.cache.putLink(n.remotePath(), target)
	return mountSymlinkTarget(n.filesystem.mountpoint, target), nil
}
