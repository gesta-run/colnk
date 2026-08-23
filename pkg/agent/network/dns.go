package network

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gesta-run/colnk/pkg/agent/remote"
	"github.com/gesta-run/colnk/pkg/protocol"
	"github.com/miekg/dns"
)

type DNSConfig struct {
	ListenAddress string
	Upstream      string
}

type dnsHandler struct {
	ctx      context.Context
	remote   *remote.Remote
	policy   protocol.NetworkPolicy
	networks []*net.IPNet
	upstream string
}

func SetupDNS(ctx context.Context, remote *remote.Remote, policy protocol.NetworkPolicy, config DNSConfig) (func() error, error) {
	if config.ListenAddress == "" {
		config.ListenAddress = "127.0.0.1:53"
	}
	if config.Upstream == "" {
		config.Upstream = "1.1.1.1:53"
	}
	networks, err := parseNetworks(policy.AllowedCIDRs)
	if err != nil {
		return nil, err
	}
	handler := &dnsHandler{ctx: ctx, remote: remote, policy: policy, networks: networks, upstream: config.Upstream}
	packetConnection, err := net.ListenPacket("udp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen udp dns: %w", err)
	}
	tcpListener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		_ = packetConnection.Close()
		return nil, fmt.Errorf("listen tcp dns: %w", err)
	}
	udpServer := &dns.Server{PacketConn: packetConnection, Handler: handler}
	tcpServer := &dns.Server{Listener: tcpListener, Handler: handler}
	go func() { _ = udpServer.ActivateAndServe() }()
	go func() { _ = tcpServer.ActivateAndServe() }()
	var cleanupOnce sync.Once
	var cleanupError error
	cleanup := func() error {
		cleanupOnce.Do(func() {
			_ = tcpServer.Shutdown()
			cleanupError = udpServer.Shutdown()
		})
		return cleanupError
	}
	go func() {
		<-ctx.Done()
		_ = cleanup()
	}()
	return cleanup, nil
}

func (h *dnsHandler) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	if len(request.Question) != 1 || !h.isLocalName(request.Question[0].Name) {
		h.forward(writer, request)
		return
	}
	question := request.Question[0]
	lookupContext, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	addresses, err := h.remote.LookupDNS(lookupContext, question.Name)
	response := new(dns.Msg)
	response.SetReply(request)
	if err != nil {
		response.Rcode = dns.RcodeNameError
		_ = writer.WriteMsg(response)
		return
	}
	for _, value := range addresses {
		ip := net.ParseIP(value)
		if !h.allowedIP(ip) {
			continue
		}
		switch {
		case question.Qtype == dns.TypeA && ip.To4() != nil:
			response.Answer = append(response.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: ip.To4()})
		case question.Qtype == dns.TypeAAAA && ip.To4() == nil:
			response.Answer = append(response.Answer, &dns.AAAA{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 30}, AAAA: ip})
		}
	}
	_ = writer.WriteMsg(response)
}

func (h *dnsHandler) allowedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, network := range h.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseNetworks(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		ip, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("parse allowed DNS network %q: %w", value, err)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("only IPv4 DNS networks are supported: %q", value)
		}
		result = append(result, network)
	}
	return result, nil
}

func (h *dnsHandler) isLocalName(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	if name == "host.colnk" {
		return true
	}
	for _, suffix := range h.policy.DNSSuffixes {
		suffix = strings.TrimSuffix(strings.ToLower(suffix), ".")
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}

func (h *dnsHandler) forward(writer dns.ResponseWriter, request *dns.Msg) {
	response, _, err := new(dns.Client).Exchange(request, h.upstream)
	if err != nil {
		failure := new(dns.Msg)
		failure.SetRcode(request, dns.RcodeServerFailure)
		_ = writer.WriteMsg(failure)
		return
	}
	_ = writer.WriteMsg(response)
}
