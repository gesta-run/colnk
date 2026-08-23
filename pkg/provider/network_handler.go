package provider

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
	"github.com/gesta-run/colnk/pkg/transport"
)

func (s *Service) handleTCP(ctx context.Context, stream transport.Stream, target string) {
	if !s.policy.AllowTCP(target) {
		s.logNetworkAudit("tcp", target, "denied", "policy")
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EACCES), Error: "target denied by local policy"}, nil)
		_ = stream.Close()
		return
	}
	select {
	case s.tcpSlots <- struct{}{}:
		defer func() { <-s.tcpSlots }()
	default:
		s.logNetworkAudit("tcp", target, "denied", "connection_limit")
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EAGAIN), Error: "TCP connection limit reached"}, nil)
		_ = stream.Close()
		return
	}
	dialTarget := target
	host, port, _ := net.SplitHostPort(target)
	if host == "100.64.0.1" {
		dialTarget = net.JoinHostPort("127.0.0.1", port)
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", dialTarget)
	if err != nil {
		s.logNetworkAudit("tcp", target, "failed", "dial")
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.ECONNREFUSED), Error: "local connection failed"}, nil)
		_ = stream.Close()
		return
	}
	defer connection.Close()
	if err := protocol.WriteResponse(stream, protocol.Response{}, nil); err != nil {
		return
	}
	toTarget, fromTarget := proxyBidirectional(stream, connection)
	attributes := []any{"result", "closed", "bytes_sent", toTarget, "bytes_received", fromTarget}
	if s.auditResources {
		attributes = append(attributes, "target", target)
	}
	s.logger.Info("tcp audit", attributes...)
}

func (s *Service) handleDNSLimited(ctx context.Context, stream transport.Stream, name string) {
	select {
	case s.dnsSlots <- struct{}{}:
		defer func() { <-s.dnsSlots }()
		s.handleDNS(ctx, stream, name)
	default:
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EAGAIN), Error: "DNS request limit reached"}, nil)
		_ = stream.Close()
	}
}

func (s *Service) handleDNS(ctx context.Context, stream transport.Stream, name string) {
	if !s.policy.AllowDNS(name) && !strings.EqualFold(strings.TrimSuffix(name, "."), "host.colnk") {
		s.logNetworkAudit("dns", name, "denied", "policy")
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EACCES), Error: "dns name denied by local policy"}, nil)
		_ = stream.Close()
		return
	}
	var addresses []string
	if strings.EqualFold(strings.TrimSuffix(name, "."), "host.colnk") {
		addresses = []string{"100.64.0.1"}
	} else {
		lookupContext, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		values, err := net.DefaultResolver.LookupIPAddr(lookupContext, name)
		if err != nil {
			s.logNetworkAudit("dns", name, "failed", "lookup")
			_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.ENOENT), Error: "dns lookup failed"}, nil)
			_ = stream.Close()
			return
		}
		for _, value := range values {
			if s.policy.AllowIP(value.IP) {
				addresses = append(addresses, value.IP.String())
			}
		}
	}
	payload, _ := json.Marshal(addresses)
	attributes := []any{"result", "allowed", "address_count", len(addresses)}
	if s.auditResources {
		attributes = append(attributes, "name", name)
	}
	s.logger.Info("dns audit", attributes...)
	_ = protocol.WriteResponse(stream, protocol.Response{}, payload)
	_ = stream.Close()
}

func proxyBidirectional(left io.ReadWriteCloser, right io.ReadWriteCloser) (int64, int64) {
	type copyResult struct {
		toTarget bool
		count    int64
	}
	done := make(chan copyResult, 2)
	copySide := func(dst io.Writer, src io.Reader, toTarget bool) {
		count, _ := io.Copy(dst, src)
		done <- copyResult{toTarget: toTarget, count: count}
	}
	go copySide(left, right, false)
	go copySide(right, left, true)
	first := <-done
	_ = left.Close()
	_ = right.Close()
	second := <-done
	var toTarget, fromTarget int64
	for _, result := range []copyResult{first, second} {
		if result.toTarget {
			toTarget = result.count
		} else {
			fromTarget = result.count
		}
	}
	return toTarget, fromTarget
}
