package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/gesta-run/colnk/pkg/protocol"
)

type NetworkPolicy struct {
	networks []*net.IPNet
	ports    map[uint16]struct{}
	suffixes []string
}

func ParseNetworkPolicy(policy protocol.NetworkPolicy) (*NetworkPolicy, error) {
	parsed := &NetworkPolicy{ports: make(map[uint16]struct{})}
	for _, value := range policy.AllowedCIDRs {
		ip, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("parse allowed cidr %q: %w", value, err)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("only IPv4 CIDRs are supported: %q", value)
		}
		parsed.networks = append(parsed.networks, network)
	}
	for _, port := range policy.AllowedPorts {
		parsed.ports[port] = struct{}{}
	}
	for _, suffix := range policy.DNSSuffixes {
		parsed.suffixes = append(parsed.suffixes, strings.TrimSuffix(strings.ToLower(suffix), "."))
	}
	return parsed, nil
}

func (p *NetworkPolicy) AllowTCP(target string) bool {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return false
	}
	if len(p.ports) > 0 {
		if _, ok := p.ports[uint16(port)]; !ok {
			return false
		}
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return p.AllowIP(ip)
}

func (p *NetworkPolicy) AllowIP(ip net.IP) bool {
	if ip == nil || ip.To4() == nil {
		return false
	}
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *NetworkPolicy) AllowDNS(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	for _, suffix := range p.suffixes {
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}
