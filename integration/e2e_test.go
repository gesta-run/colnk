package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
	localservice "github.com/gesta-run/colnk/pkg/provider"
	"github.com/gesta-run/colnk/pkg/transport"
)

type testBridge struct {
	remote *remote.Remote
}

func startBridge(t *testing.T, root string, policy protocol.NetworkPolicy) *testBridge {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverConnection := make(chan *transport.Conn, 1)
	serverError := make(chan error, 1)
	go func() {
		raw, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverError <- acceptErr
			return
		}
		serverPolicy := protocol.DefaultNetworkPolicy()
		serverPolicy.MaxTCPConnections = policy.MaxTCPConnections
		if serverPolicy.MaxTCPConnections == 0 {
			serverPolicy.MaxTCPConnections = 256
		}
		connection, acceptErr := transport.Accept(ctx, raw, "sk-test", serverPolicy)
		if acceptErr != nil {
			_ = raw.Close()
			serverError <- acceptErr
			return
		}
		serverConnection <- connection
	}()
	clientPolicy := policy
	clientPolicy.MaxTCPConnections = 0
	clientConnection, ack, err := transport.Dial(ctx, transport.Config{
		Endpoint: listener.Addr().String(), Credential: "sk-test", Policy: clientPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service, err := localservice.NewService(root, ack.Policy, logger, false)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = service.Serve(ctx, clientConnection) }()
	var agentConnection *transport.Conn
	select {
	case agentConnection = <-serverConnection:
	case err := <-serverError:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	t.Cleanup(func() {
		cancel()
		_ = clientConnection.Close()
		_ = agentConnection.Close()
		_ = listener.Close()
	})
	return &testBridge{remote: remote.NewRemote(agentConnection)}
}

func TestUnauthorizedConnectionIsRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		raw, acceptErr := listener.Accept()
		if acceptErr == nil {
			defer raw.Close()
			_, _ = transport.Accept(ctx, raw, "sk-correct", protocol.NetworkPolicy{})
		}
	}()
	if _, _, err := transport.Dial(ctx, transport.Config{Endpoint: listener.Addr().String(), Credential: "sk-wrong"}); err == nil || !transport.IsPermanent(err) {
		t.Fatalf("wrong API key was not permanently rejected: %v", err)
	}
}

func TestFileRoundTrip(t *testing.T) {
	bridge := startBridge(t, t.TempDir(), protocol.NetworkPolicy{})
	if _, _, err := bridge.remote.File(context.Background(), protocol.Request{Operation: "create", Path: "hello.txt", Mode: 0o600}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := bridge.remote.File(context.Background(), protocol.Request{Operation: "write", Path: "hello.txt"}, []byte("hello through bridge")); err != nil {
		t.Fatal(err)
	}
	_, data, err := bridge.remote.File(context.Background(), protocol.Request{Operation: "read", Path: "hello.txt", Size: 1024}, nil)
	if err != nil || string(data) != "hello through bridge" {
		t.Fatalf("unexpected read %q: %v", data, err)
	}
	entries, err := bridge.remote.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "hello.txt" || entries[0].Attr.Size != uint64(len("hello through bridge")) {
		t.Fatalf("unexpected directory entries: %#v", entries)
	}
}

func TestDirectoryEntriesStreamAcrossPages(t *testing.T) {
	root := t.TempDir()
	for index := range 300 {
		if err := os.WriteFile(fmt.Sprintf("%s/file-%03d", root, index), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bridge := startBridge(t, root, protocol.NetworkPolicy{})
	entries, err := bridge.remote.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 300 {
		t.Fatalf("received %d directory entries; want 300", len(entries))
	}
}

func TestLargeFileUsesBoundedChunks(t *testing.T) {
	bridge := startBridge(t, t.TempDir(), protocol.NetworkPolicy{})
	const chunkSize = 1 << 20
	const chunkCount = 12
	chunk := bytes.Repeat([]byte("colnk-large-file-block"), chunkSize/len("colnk-large-file-block")+1)[:chunkSize]
	if _, _, err := bridge.remote.File(context.Background(), protocol.Request{Operation: "create", Path: "large.bin", Mode: 0o600}, nil); err != nil {
		t.Fatal(err)
	}
	writtenHash := sha256.New()
	for index := range chunkCount {
		if _, _, err := bridge.remote.File(context.Background(), protocol.Request{Operation: "write", Path: "large.bin", Offset: int64(index * chunkSize)}, chunk); err != nil {
			t.Fatal(err)
		}
		_, _ = writtenHash.Write(chunk)
	}
	readHash := sha256.New()
	for index := range chunkCount {
		_, data, err := bridge.remote.File(context.Background(), protocol.Request{Operation: "read", Path: "large.bin", Offset: int64(index * chunkSize), Size: chunkSize}, nil)
		if err != nil || len(data) != chunkSize {
			t.Fatalf("large file read failed at chunk %d: %d bytes, %v", index, len(data), err)
		}
		_, _ = readHash.Write(data)
	}
	if !bytes.Equal(writtenHash.Sum(nil), readHash.Sum(nil)) {
		t.Fatal("large file checksum mismatch")
	}
}

func TestTCPRoundTrip(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go echoConnections(listener)
	bridge := startBridge(t, t.TempDir(), protocol.NetworkPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}})
	stream, err := bridge.remote.OpenTCP(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	payload := []byte("tcp-through-local")
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, response); err != nil || !bytes.Equal(payload, response) {
		t.Fatalf("unexpected echo %q: %v", response, err)
	}
}

func TestTCPPolicyAndConnectionLimit(t *testing.T) {
	denied := startBridge(t, t.TempDir(), protocol.NetworkPolicy{AllowedCIDRs: []string{"100.64.0.1/32"}})
	if _, err := denied.remote.OpenTCP(context.Background(), "127.0.0.1:12345"); err == nil {
		t.Fatal("expected local policy to deny target")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go echoConnections(listener)
	limited := startBridge(t, t.TempDir(), protocol.NetworkPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}, MaxTCPConnections: 1})
	first, err := limited.remote.OpenTCP(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := limited.remote.OpenTCP(context.Background(), listener.Addr().String()); err == nil {
		t.Fatal("expected concurrent TCP connection limit to be enforced")
	}
}

func echoConnections(listener net.Listener) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer connection.Close()
			_, _ = io.Copy(connection, connection)
		}()
	}
}
