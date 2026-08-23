//go:build linux

package network

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"

	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
)

const (
	firewallChain       = "COLNK"
	interfaceOwnerAlias = "colnk-owned"
)

type NetworkConfig struct {
	InterfaceName string
	ProxyPort     int
}

type networkRuntime struct {
	config           NetworkConfig
	policy           protocol.NetworkPolicy
	remote           *remote.Remote
	logger           *slog.Logger
	listener         net.Listener
	tcpSlots         chan struct{}
	routes           []string
	interfaceCreated bool
	once             sync.Once
}

func SetupNetwork(ctx context.Context, remote *remote.Remote, policy protocol.NetworkPolicy, config NetworkConfig, logger *slog.Logger) (func() error, error) {
	if config.InterfaceName == "" {
		config.InterfaceName = "local0"
	}
	if config.ProxyPort == 0 {
		config.ProxyPort = 15001
	}
	connectionLimit := policy.MaxTCPConnections
	if connectionLimit <= 0 {
		connectionLimit = 256
	}
	connectionLimit = min(connectionLimit, 1024)
	runtime := &networkRuntime{
		config: config, policy: policy, remote: remote, logger: logger,
		tcpSlots: make(chan struct{}, connectionLimit),
	}
	if err := runtime.start(ctx); err != nil {
		_ = runtime.stop()
		return nil, err
	}
	return runtime.stop, nil
}

func (r *networkRuntime) start(ctx context.Context) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(r.config.ProxyPort))
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return fmt.Errorf("listen transparent proxy: %w", err)
	}
	r.listener = listener
	if err := r.reconcileStaleState(); err != nil {
		return err
	}
	if err := runCommand("ip", "link", "add", "dev", r.config.InterfaceName, "type", "dummy"); err != nil {
		return err
	}
	r.interfaceCreated = true
	commands := [][]string{
		{"ip", "link", "set", "dev", r.config.InterfaceName, "alias", interfaceOwnerAlias},
		{"ip", "addr", "add", "100.64.0.2/32", "dev", r.config.InterfaceName},
		{"ip", "link", "set", "dev", r.config.InterfaceName, "up"},
	}
	for _, command := range commands {
		if err := runCommand(command...); err != nil {
			return err
		}
	}
	if err := r.prepareFirewall(); err != nil {
		return err
	}
	for _, cidr := range r.policy.AllowedCIDRs {
		if err := r.addRoute(cidr); err != nil {
			return err
		}
	}
	go r.acceptLoop(ctx)
	go func() {
		<-ctx.Done()
		_ = r.stop()
	}()
	return nil
}

func (r *networkRuntime) stop() error {
	var firstError error
	r.once.Do(func() {
		if r.listener != nil {
			_ = r.listener.Close()
		}
		for index := len(r.routes) - 1; index >= 0; index-- {
			if err := runCommand("ip", "route", "del", r.routes[index], "dev", r.config.InterfaceName); err != nil && firstError == nil {
				firstError = err
			}
		}
		if err := r.cleanupFirewall(); err != nil && firstError == nil {
			firstError = err
		}
		if r.interfaceCreated {
			if err := runCommand("ip", "link", "del", r.config.InterfaceName); err != nil && firstError == nil {
				firstError = err
			}
		}
	})
	return firstError
}
