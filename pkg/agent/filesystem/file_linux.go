//go:build linux

package filesystem

import (
	"context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/gesta-run/colnk/pkg/protocol"
)

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
