//go:build linux

package filesystem

import (
	"context"
	"os"
	"path"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/gesta-run/colnk/pkg/protocol"
)

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
