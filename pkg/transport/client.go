package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
)

type Config struct {
	Endpoint   string
	Credential string
	Policy     protocol.NetworkPolicy
}

type PermanentError struct {
	Err error
}

const handshakeTimeout = 10 * time.Second

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func IsPermanent(err error) bool {
	var permanent *PermanentError
	return errors.As(err, &permanent)
}

func Dial(ctx context.Context, config Config) (*Conn, protocol.HandshakeAck, error) {
	address, err := endpointAddress(config.Endpoint)
	if err != nil {
		return nil, protocol.HandshakeAck{}, &PermanentError{Err: err}
	}
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, protocol.HandshakeAck{}, fmt.Errorf("dial server: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = raw.Close()
		}
	}()
	_ = raw.SetDeadline(time.Now().Add(handshakeTimeout))
	if err := protocol.WriteJSON(raw, protocol.Handshake{
		MajorVersion: protocol.MajorVersion,
		MinorVersion: protocol.MinorVersion,
		APIKey:       config.Credential,
		Policy:       config.Policy,
	}); err != nil {
		return nil, protocol.HandshakeAck{}, err
	}
	var ack protocol.HandshakeAck
	if err := protocol.ReadJSON(raw, &ack); err != nil {
		return nil, protocol.HandshakeAck{}, err
	}
	if !ack.Accepted {
		rejection := fmt.Errorf("server rejected connection: %s", ack.Error)
		if ack.ErrorCode == protocol.HandshakeErrorSessionActive {
			return nil, ack, rejection
		}
		return nil, ack, &PermanentError{Err: rejection}
	}
	if ack.MajorVersion != protocol.MajorVersion {
		return nil, ack, &PermanentError{Err: errors.New("server returned an incompatible protocol version")}
	}
	if ack.MinorVersion < 1 && !sameNetworkShare(config.Policy, ack.Policy) {
		return nil, ack, &PermanentError{Err: errors.New("server does not support the requested client network policy")}
	}
	_ = raw.SetDeadline(time.Time{})
	connection, err := newConn(raw, false)
	if err != nil {
		return nil, ack, err
	}
	succeeded = true
	return connection, ack, nil
}

func sameNetworkShare(requested, effective protocol.NetworkPolicy) bool {
	return slices.Equal(requested.AllowedCIDRs, effective.AllowedCIDRs) &&
		slices.Equal(requested.AllowedPorts, effective.AllowedPorts) &&
		slices.Equal(requested.DNSSuffixes, effective.DNSSuffixes)
}

func endpointAddress(endpoint string) (string, error) {
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return "", errors.New("endpoint must be host:port")
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" || port == "" {
		return "", errors.New("endpoint must be host:port")
	}
	return endpoint, nil
}
