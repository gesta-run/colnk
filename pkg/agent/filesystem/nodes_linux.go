//go:build linux

package filesystem

import (
	"path"
	"strings"

	"bazil.org/fuse/fs"
	"github.com/gesta-run/colnk/pkg/protocol"
)

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
