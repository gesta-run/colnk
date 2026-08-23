package network

import (
	"net"
	"testing"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestSplitDNSFiltersReboundAddress(t *testing.T) {
	networks, err := parseNetworks([]string{"10.20.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	handler := &dnsHandler{networks: networks}
	if !handler.allowedIP(net.ParseIP("10.20.1.5")) {
		t.Fatal("allowed split DNS address was rejected")
	}
	if handler.allowedIP(net.ParseIP("203.0.113.10")) {
		t.Fatal("DNS rebinding address escaped the configured CIDR")
	}
}

func TestSplitDNSSuffix(t *testing.T) {
	handler := &dnsHandler{policy: protocol.NetworkPolicy{DNSSuffixes: []string{"corp.example"}}}
	if !handler.isLocalName("api.corp.example.") || handler.isLocalName("corp.example.invalid.") {
		t.Fatal("split DNS suffix matching is incorrect")
	}
}
