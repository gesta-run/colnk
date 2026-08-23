package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"syscall"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
	providerfs "github.com/gesta-run/colnk/pkg/provider/filesystem"
	providernet "github.com/gesta-run/colnk/pkg/provider/network"
	"github.com/gesta-run/colnk/pkg/transport"
	"golang.org/x/sync/semaphore"
)

type Service struct {
	files          *providerfs.FileService
	policy         *providernet.NetworkPolicy
	logger         *slog.Logger
	tcpSlots       chan struct{}
	fileSlots      chan struct{}
	dnsSlots       chan struct{}
	streamSlots    chan struct{}
	payloadBudget  *semaphore.Weighted
	auditResources bool
}

const (
	concurrentFileRequests = 32
	concurrentDNSRequests  = 8
	maxInFlightFileBytes   = 64 << 20
)

func NewService(root string, networkPolicy protocol.NetworkPolicy, logger *slog.Logger, auditResources bool) (*Service, error) {
	files, err := providerfs.NewFileService(root)
	if err != nil {
		return nil, fmt.Errorf("create file service: %w", err)
	}
	policy, err := providernet.ParseNetworkPolicy(networkPolicy)
	if err != nil {
		return nil, err
	}
	connectionLimit := networkPolicy.MaxTCPConnections
	if connectionLimit <= 0 {
		connectionLimit = 256
	}
	connectionLimit = min(connectionLimit, 1024)
	return &Service{
		files: files, policy: policy, logger: logger, tcpSlots: make(chan struct{}, connectionLimit),
		fileSlots: make(chan struct{}, concurrentFileRequests), dnsSlots: make(chan struct{}, concurrentDNSRequests),
		streamSlots:   make(chan struct{}, connectionLimit+concurrentFileRequests+concurrentDNSRequests),
		payloadBudget: semaphore.NewWeighted(maxInFlightFileBytes), auditResources: auditResources,
	}, nil
}

func (s *Service) Serve(ctx context.Context, conn *transport.Conn) error {
	sessionContext, cancel := context.WithCancel(ctx)
	stopConnection := context.AfterFunc(conn.Context(), cancel)
	var requests sync.WaitGroup
	defer func() {
		cancel()
		stopConnection()
		requests.Wait()
		_ = s.files.Close()
	}()
	for {
		stream, err := conn.AcceptStream(sessionContext)
		if err != nil {
			return err
		}
		select {
		case s.streamSlots <- struct{}{}:
			requests.Add(1)
			go func() {
				defer requests.Done()
				defer func() { <-s.streamSlots }()
				s.handleStream(sessionContext, stream)
			}()
		default:
			_ = stream.Close()
		}
	}
}

func (s *Service) handleStream(ctx context.Context, stream transport.Stream) {
	stopStream := context.AfterFunc(ctx, func() { _ = stream.Close() })
	defer stopStream()
	_ = stream.SetReadDeadline(time.Now().Add(60 * time.Second))
	request, err := protocol.ReadRequestHeader(stream)
	if err != nil {
		s.logger.Warn("read bridge request", "error", err)
		_ = stream.Close()
		return
	}
	if request.Kind != protocol.KindFile && requestHasPayload(request) {
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EINVAL), Error: "unexpected request payload"}, nil)
		_ = stream.Close()
		return
	}
	switch request.Kind {
	case protocol.KindFile:
		s.handleFile(ctx, stream, request)
	case protocol.KindTCP:
		_ = stream.SetReadDeadline(time.Time{})
		s.handleTCP(ctx, stream, request.Target)
	case protocol.KindDNS:
		_ = stream.SetReadDeadline(time.Time{})
		s.handleDNSLimited(ctx, stream, request.Target)
	default:
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.ENOSYS), Error: "unsupported request kind"}, nil)
		_ = stream.Close()
	}
}
