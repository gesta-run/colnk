package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/gesta-run/colnk/pkg/transport"
)

type sessionState struct {
	mu     sync.Mutex
	active bool
}

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return fmt.Errorf("listen for provider client: %w", err)
	}
	runContext, cancel := context.WithCancel(ctx)
	var sessions sync.WaitGroup
	stopListener := context.AfterFunc(runContext, func() { _ = listener.Close() })
	defer func() {
		cancel()
		_ = listener.Close()
		sessions.Wait()
		stopListener()
	}()
	logger.Info("CoLnk server listening", "address", config.Listen, "mountpoint", config.Mountpoint)
	var state sessionState
	handshakeSlots := make(chan struct{}, 32)
	for {
		raw, err := listener.Accept()
		if err != nil {
			if runContext.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept provider client: %w", err)
		}
		select {
		case handshakeSlots <- struct{}{}:
			sessions.Add(1)
			go func(raw net.Conn) {
				defer sessions.Done()
				defer func() { <-handshakeSlots }()
				stopConnection := context.AfterFunc(runContext, func() { _ = raw.Close() })
				defer stopConnection()
				if err := serveSession(runContext, raw, config, &state, logger); err != nil && runContext.Err() == nil && !errors.Is(err, transport.ErrSessionActive) && !errors.Is(err, transport.ErrUnauthorized) {
					logger.Warn("provider session ended", "error", err)
				}
			}(raw)
		default:
			_ = raw.Close()
		}
	}
}
