package transport

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

var ErrUnauthorized = errors.New("unauthorized")
var ErrSessionActive = errors.New("session already active")

func Accept(ctx context.Context, raw net.Conn, credential string, policy protocol.NetworkPolicy) (*Conn, error) {
	connection, _, err := AcceptWhen(ctx, raw, credential, policy, nil)
	return connection, err
}

func AcceptWhen(ctx context.Context, raw net.Conn, credential string, policy protocol.NetworkPolicy, admit func() bool) (*Conn, bool, error) {
	_ = raw.SetDeadline(time.Now().Add(handshakeTimeout))
	var handshake protocol.Handshake
	if err := protocol.ReadJSON(raw, &handshake); err != nil {
		return nil, false, err
	}
	if err := protocol.ValidateHandshake(handshake); err != nil || !sameCredential(handshake.APIKey, credential) {
		_ = protocol.WriteJSON(raw, protocol.HandshakeAck{
			Accepted: false, ErrorCode: protocol.HandshakeErrorUnauthorized, Error: "unauthorized",
		})
		return nil, false, ErrUnauthorized
	}
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}
	admitted := false
	if admit != nil {
		admitted = admit()
		if !admitted {
			_ = protocol.WriteJSON(raw, protocol.HandshakeAck{
				Accepted: false, ErrorCode: protocol.HandshakeErrorSessionActive, Error: ErrSessionActive.Error(),
			})
			return nil, false, ErrSessionActive
		}
	}
	effectivePolicy := policy
	if handshake.MinorVersion >= 1 {
		effectivePolicy = handshake.Policy
		effectivePolicy.MaxTCPConnections = policy.MaxTCPConnections
	}
	if err := protocol.ValidateNetworkPolicy(effectivePolicy); err != nil {
		_ = protocol.WriteJSON(raw, protocol.HandshakeAck{
			Accepted: false, ErrorCode: protocol.HandshakeErrorUnauthorized, Error: "invalid network policy",
		})
		return nil, admitted, err
	}
	ack := protocol.HandshakeAck{
		Accepted: true, MajorVersion: protocol.MajorVersion,
		MinorVersion: min(protocol.MinorVersion, handshake.MinorVersion), Policy: effectivePolicy,
	}
	if err := protocol.WriteJSON(raw, ack); err != nil {
		return nil, admitted, err
	}
	_ = raw.SetDeadline(time.Time{})
	connection, err := newConn(raw, true)
	if connection != nil {
		connection.policy = effectivePolicy
	}
	return connection, admitted, err
}

func sameCredential(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
