package transport

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func TestPermanentErrorClassification(t *testing.T) {
	permanent := &PermanentError{Err: errors.New("invalid configuration")}
	if !IsPermanent(permanent) || IsPermanent(errors.New("temporary network error")) {
		t.Fatal("connection error classification is incorrect")
	}
}

func TestEndpointValidation(t *testing.T) {
	if _, err := endpointAddress("http://agent.example.test:7443"); err == nil {
		t.Fatal("URL endpoint was accepted")
	}
	if address, err := endpointAddress("agent.example.test:7443"); err != nil || address != "agent.example.test:7443" {
		t.Fatalf("unexpected endpoint result %q %v", address, err)
	}
}

func TestTCPHandshake(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	requestedPolicy := protocol.NetworkPolicy{AllowedCIDRs: []string{"192.168.1.0/24"}, AllowedPorts: []uint16{443}}
	serverPolicy := protocol.DefaultNetworkPolicy()
	serverPolicy.MaxTCPConnections = 8
	accepted := make(chan *Conn, 1)
	go func() {
		connection, err := Accept(context.Background(), server, "sk-test", serverPolicy)
		if err == nil {
			accepted <- connection
		}
	}()
	_ = client.SetDeadline(time.Now().Add(handshakeTimeout))
	if err := protocol.WriteJSON(client, protocol.Handshake{
		MajorVersion: protocol.MajorVersion, MinorVersion: protocol.MinorVersion,
		APIKey: "sk-test", Policy: requestedPolicy,
	}); err != nil {
		t.Fatal(err)
	}
	var ack protocol.HandshakeAck
	if err := protocol.ReadJSON(client, &ack); err != nil || !ack.Accepted || ack.Policy.MaxTCPConnections != 8 ||
		len(ack.Policy.AllowedCIDRs) != 1 || ack.Policy.AllowedCIDRs[0] != "192.168.1.0/24" ||
		len(ack.Policy.AllowedPorts) != 1 || ack.Policy.AllowedPorts[0] != 443 {
		t.Fatalf("unexpected handshake response %#v: %v", ack, err)
	}
	_ = client.SetDeadline(time.Time{})
	clientConn, err := newConn(client, false)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()
	serverConn := <-accepted
	defer serverConn.Close()
}

func TestAdmissionRunsOnlyAfterAuthentication(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	admissionCalled := false
	result := make(chan error, 1)
	go func() {
		_, _, err := AcceptWhen(context.Background(), server, "sk-correct", protocol.NetworkPolicy{}, func() bool {
			admissionCalled = true
			return true
		})
		result <- err
	}()
	if err := protocol.WriteJSON(client, protocol.Handshake{MajorVersion: protocol.MajorVersion, APIKey: "sk-wrong"}); err != nil {
		t.Fatal(err)
	}
	var ack protocol.HandshakeAck
	if err := protocol.ReadJSON(client, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Accepted || ack.Error != "unauthorized" {
		t.Fatalf("unexpected handshake response %#v", ack)
	}
	if err := <-result; !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unexpected accept error %v", err)
	}
	if admissionCalled {
		t.Fatal("unauthenticated connection reached admission control")
	}
}

func TestActiveSessionIsRejectedDuringAdmission(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	result := make(chan error, 1)
	go func() {
		_, admitted, err := AcceptWhen(context.Background(), server, "sk-test", protocol.NetworkPolicy{}, func() bool { return false })
		if admitted {
			result <- errors.New("rejected connection was admitted")
			return
		}
		result <- err
	}()
	if err := protocol.WriteJSON(client, protocol.Handshake{MajorVersion: protocol.MajorVersion, APIKey: "sk-test"}); err != nil {
		t.Fatal(err)
	}
	var ack protocol.HandshakeAck
	if err := protocol.ReadJSON(client, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Accepted || ack.ErrorCode != protocol.HandshakeErrorSessionActive || ack.Error != ErrSessionActive.Error() {
		t.Fatalf("unexpected handshake response %#v", ack)
	}
	if err := <-result; !errors.Is(err, ErrSessionActive) {
		t.Fatalf("unexpected accept error %v", err)
	}
}

func TestActiveSessionRejectionIsRetryable(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		raw, acceptErr := listener.Accept()
		if acceptErr == nil {
			defer raw.Close()
			_, _, _ = AcceptWhen(context.Background(), raw, "sk-test", protocol.NetworkPolicy{}, func() bool { return false })
		}
	}()
	_, _, err = Dial(context.Background(), Config{Endpoint: listener.Addr().String(), Credential: "sk-test"})
	if err == nil || IsPermanent(err) {
		t.Fatalf("active session rejection should be retryable: %v", err)
	}
}
