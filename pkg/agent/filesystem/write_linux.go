//go:build linux

package filesystem

import (
	"context"
	"sort"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func (n *remoteNode) bufferWrite(ctx context.Context, offset int64, data []byte) error {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	for len(data) > 0 {
		blockOffset := offset / int64(protocol.MaxPayloadBytes) * int64(protocol.MaxPayloadBytes)
		inside := int(offset - blockOffset)
		count := min(len(data), protocol.MaxPayloadBytes-inside)
		block := n.writeBlocks[blockOffset]
		if block == nil {
			if len(n.writeBlocks) >= maxPendingWriteBlocks {
				if _, err := n.flushWritesLocked(ctx, false); err != nil {
					return err
				}
			}
			acquired := n.filesystem.writeBudget.TryAcquire(1)
			if !acquired {
				if len(n.writeBlocks) > 0 {
					if _, err := n.flushWritesLocked(ctx, false); err != nil {
						return err
					}
				}
				acquired = n.filesystem.writeBudget.TryAcquire(1)
			}
			if !acquired {
				response, _, err := n.filesystem.remote.File(ctx, protocol.Request{
					Operation: "write", Path: n.remotePath(), Offset: offset,
				}, data[:count])
				if err != nil {
					return err
				}
				n.filesystem.updateChangedAttr(n.remotePath(), response.Attr)
				offset += int64(count)
				data = data[count:]
				continue
			}
			if n.writeBlocks == nil {
				n.writeBlocks = make(map[int64]*pendingWriteBlock)
			}
			block = &pendingWriteBlock{data: make([]byte, protocol.MaxPayloadBytes)}
			n.writeBlocks[blockOffset] = block
		}
		block.write(inside, data[:count])
		offset += int64(count)
		data = data[count:]
	}
	return nil
}

func (n *remoteNode) flushPending(ctx context.Context) error {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	_, err := n.flushWritesLocked(ctx, false)
	return err
}

func (n *remoteNode) flushPendingSync(ctx context.Context) (bool, error) {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	return n.flushWritesLocked(ctx, true)
}

func (n *remoteNode) flushWritesLocked(ctx context.Context, syncLast bool) (bool, error) {
	offsets := make([]int64, 0, len(n.writeBlocks))
	operationCount := 0
	for offset := range n.writeBlocks {
		offsets = append(offsets, offset)
		operationCount += len(n.writeBlocks[offset].dirty)
	}
	sort.Slice(offsets, func(left, right int) bool { return offsets[left] < offsets[right] })
	var latest *protocol.FileAttr
	remaining := operationCount
	for _, offset := range offsets {
		block := n.writeBlocks[offset]
		for _, dirty := range block.dirty {
			remaining--
			request := protocol.Request{Operation: "write", Path: n.remotePath(), Offset: offset + int64(dirty.start)}
			if remaining == 0 {
				request.Sync = syncLast
				if n.writeMtime != nil {
					request.SetModTime = true
					request.ModTimeNS = n.writeMtime.UnixNano()
				}
			}
			response, _, err := n.filesystem.remote.File(ctx, request, block.data[dirty.start:dirty.end])
			if err != nil {
				return false, err
			}
			if response.Attr != nil {
				if latest == nil {
					value := *response.Attr
					latest = &value
				} else {
					latest.Size = max(latest.Size, response.Attr.Size)
					latest.ModTimeNS = max(latest.ModTimeNS, response.Attr.ModTimeNS)
				}
			}
		}
		delete(n.writeBlocks, offset)
		n.filesystem.writeBudget.Release(1)
	}
	remotePath := n.remotePath()
	if len(offsets) > 0 {
		n.writeBlocks = nil
		n.writeMtime = nil
		n.filesystem.updateChangedAttr(remotePath, latest)
	}
	if n.writeMtime != nil {
		response, _, err := n.filesystem.remote.File(ctx, protocol.Request{
			Operation: "chtimes", Path: remotePath, ModTimeNS: n.writeMtime.UnixNano(),
		}, nil)
		if err != nil {
			return false, err
		}
		n.writeMtime = nil
		n.filesystem.updateChangedAttr(remotePath, response.Attr)
	}
	return syncLast && operationCount > 0, nil
}

func (n *remoteNode) deferMtime(value time.Time) bool {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	if len(n.writeBlocks) == 0 {
		return false
	}
	copy := value
	n.writeMtime = &copy
	return true
}
