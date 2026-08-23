//go:build !linux

package network

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
)

type NetworkConfig struct {
	InterfaceName string
	ProxyPort     int
}

func SetupNetwork(_ context.Context, _ *remote.Remote, _ protocol.NetworkPolicy, _ NetworkConfig, _ *slog.Logger) (func() error, error) {
	return nil, errors.New("agent network setup is supported only on Linux")
}
