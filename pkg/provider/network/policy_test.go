package network

import (
	"testing"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestNetworkPolicy(t *testing.T) {
	policy, err := ParseNetworkPolicy(protocol.NetworkPolicy{
		AllowedCIDRs: []string{"100.64.0.1/32", "10.0.0.0/8"},
		AllowedPorts: []uint16{443}, DNSSuffixes: []string{"corp.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !policy.AllowTCP("100.64.0.1:443") || !policy.AllowTCP("10.2.3.4:443") {
		t.Fatal("allowed targets rejected")
	}
	if policy.AllowTCP("10.2.3.4:80") || policy.AllowTCP("192.168.1.1:443") {
		t.Fatal("denied target accepted")
	}
	if !policy.AllowDNS("api.corp.test") || policy.AllowDNS("api.example.com") {
		t.Fatal("dns suffix policy mismatch")
	}
}
