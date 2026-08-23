//go:build linux

package filesystem

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"bazil.org/fuse"
	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
)

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
