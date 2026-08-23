package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
	"github.com/gesta-run/colnk/pkg/transport"
)

func (r *Remote) OpenTCP(ctx context.Context, target string) (transport.Stream, error) {
	stream, err := r.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	stop := bindStreamContext(ctx, stream)
	request := protocol.Request{Kind: protocol.KindTCP, Target: target}
	if err := protocol.WriteRequest(stream, request, nil); err != nil {
		stop()
		_ = stream.Close()
		return nil, err
	}
	response, _, err := protocol.ReadResponse(stream)
	if err != nil {
		stop()
		_ = stream.Close()
		return nil, err
	}
	if response.ErrorCode != 0 {
		stop()
		_ = stream.Close()
		return nil, &RemoteError{Code: syscall.Errno(response.ErrorCode), Message: response.Error}
	}
	_ = stream.SetDeadline(time.Time{})
	return &contextStream{Stream: stream, stop: stop}, nil
}

func (r *Remote) LookupDNS(ctx context.Context, name string) ([]string, error) {
	stream, err := r.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	stop := bindStreamContext(ctx, stream)
	defer stop()
	request := protocol.Request{Kind: protocol.KindDNS, Target: name}
	if err := protocol.WriteRequest(stream, request, nil); err != nil {
		return nil, err
	}
	response, payload, err := protocol.ReadResponse(stream)
	if err != nil {
		return nil, err
	}
	if response.ErrorCode != 0 {
		return nil, &RemoteError{Code: syscall.Errno(response.ErrorCode), Message: response.Error}
	}
	var addresses []string
	if err := json.Unmarshal(payload, &addresses); err != nil {
		return nil, fmt.Errorf("decode dns response: %w", err)
	}
	return addresses, nil
}
