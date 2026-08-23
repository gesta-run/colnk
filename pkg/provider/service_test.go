package provider

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestServiceReservesIndependentRequestCapacity(t *testing.T) {
	const tcpLimit = 7
	service, err := NewService(t.TempDir(), protocol.NetworkPolicy{MaxTCPConnections: tcpLimit}, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cap(service.streamSlots), tcpLimit+concurrentFileRequests+concurrentDNSRequests; got != want {
		t.Fatalf("unexpected stream capacity %d; want %d", got, want)
	}
	if got := cap(service.fileSlots); got != concurrentFileRequests {
		t.Fatalf("unexpected file request capacity %d", got)
	}
}
