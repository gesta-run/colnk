package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/gesta-run/colnk/pkg/agent/filesystem"
	"github.com/gesta-run/colnk/pkg/agent/network"
	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/transport"
)

func serveSession(ctx context.Context, raw net.Conn, config Config, state *sessionState, logger *slog.Logger) error {
	defer raw.Close()
	connection, admitted, err := transport.AcceptWhen(ctx, raw, config.APIKey, config.Policy, func() bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.active {
			return false
		}
		state.active = true
		return true
	})
	releaseSession := func() {}
	if admitted {
		var releaseOnce sync.Once
		releaseSession = func() {
			releaseOnce.Do(func() {
				state.mu.Lock()
				state.active = false
				state.mu.Unlock()
			})
		}
		defer releaseSession()
	}
	if err != nil {
		return err
	}
	defer connection.Close()
	policy := connection.NetworkPolicy()
	sessionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-connection.Context().Done():
			cancel()
		case <-sessionContext.Done():
		}
	}()
	remote := remote.NewRemote(connection)
	unmount, err := filesystem.Mount(sessionContext, remote, config.Mountpoint, config.AllowOther, config.MetadataCacheTTL)
	if err != nil {
		return fmt.Errorf("mount local filesystem: %w", err)
	}
	stopNetwork, err := network.SetupNetwork(sessionContext, remote, policy, network.NetworkConfig{
		InterfaceName: config.InterfaceName, ProxyPort: config.ProxyPort,
	}, logger)
	if err != nil {
		releaseSession()
		_ = unmount()
		return fmt.Errorf("configure local network: %w", err)
	}
	stopDNS, restoreResolver, err := startDNS(sessionContext, remote, policy, config)
	if err != nil {
		releaseSession()
		_ = stopNetwork()
		_ = unmount()
		return fmt.Errorf("configure DNS: %w", err)
	}
	logger.Info("Mac bridge ready", "mountpoint", config.Mountpoint, "interface", config.InterfaceName)
	<-sessionContext.Done()
	releaseSession()
	_ = connection.Close()
	restoreResolver()
	_ = stopDNS()
	_ = stopNetwork()
	_ = unmount()
	return connection.Context().Err()
}
