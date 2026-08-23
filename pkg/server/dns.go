package server

import (
	"context"
	"net"

	"github.com/gesta-run/colnk/pkg/agent/network"
	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
)

func startDNS(ctx context.Context, remote *remote.Remote, policy protocol.NetworkPolicy, config Config) (func() error, func(), error) {
	stopDNS, err := network.SetupDNS(ctx, remote, policy, network.DNSConfig{
		ListenAddress: config.DNSAddress, Upstream: config.UpstreamDNS,
	})
	if err != nil {
		return nil, nil, err
	}
	restore := func() {}
	if config.ConfigureResolver {
		resolverHost, _, splitErr := net.SplitHostPort(config.DNSAddress)
		if splitErr != nil {
			_ = stopDNS()
			return nil, nil, splitErr
		}
		restoreResolver, resolverErr := network.ConfigureResolver(resolverHost)
		if resolverErr != nil {
			_ = stopDNS()
			return nil, nil, resolverErr
		}
		restore = func() { _ = restoreResolver() }
	}
	return stopDNS, restore, nil
}
