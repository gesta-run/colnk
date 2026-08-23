package filesystem

import (
	"bytes"
	"testing"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestPendingWriteBlockMergesOutOfOrderAndOverlappingWrites(t *testing.T) {
	block := pendingWriteBlock{data: make([]byte, protocol.MaxPayloadBytes)}
	third := protocol.MaxPayloadBytes / 3
	block.write(third, bytes.Repeat([]byte("b"), third))
	block.write(0, bytes.Repeat([]byte("a"), third))
	block.write(2*third, bytes.Repeat([]byte("c"), protocol.MaxPayloadBytes-2*third))
	block.write(third-1, []byte("xy"))
	if len(block.dirty) != 1 || block.dirty[0] != (writeRange{start: 0, end: protocol.MaxPayloadBytes}) {
		t.Fatalf("complete block retained dirty ranges: %#v", block.dirty)
	}
	if block.data[third-1] != 'x' || block.data[third] != 'y' {
		t.Fatal("later overlapping write did not win")
	}
}

func TestPendingWriteBlockRetainsGaps(t *testing.T) {
	block := pendingWriteBlock{data: make([]byte, protocol.MaxPayloadBytes)}
	block.write(0, []byte("left"))
	block.write(16, []byte("right"))
	if len(block.dirty) != 2 {
		t.Fatalf("gap was incorrectly merged: %#v", block.dirty)
	}
}
