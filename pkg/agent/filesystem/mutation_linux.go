//go:build linux

package filesystem

import (
	"context"
	"path"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/gesta-run/colnk/pkg/protocol"
)

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
