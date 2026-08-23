package remote

import (
	"context"
	"syscall"

	"github.com/gesta-run/colnk/pkg/protocol"
)

func (r *Remote) File(ctx context.Context, request protocol.Request, data []byte) (protocol.Response, []byte, error) {
	request.Kind = protocol.KindFile
	stream, err := r.conn.OpenStreamSync(ctx)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	defer stream.Close()
	stop := bindStreamContext(ctx, stream)
	defer stop()
	if err := protocol.WriteRequest(stream, request, data); err != nil {
		return protocol.Response{}, nil, err
	}
	response, payload, err := protocol.ReadResponse(stream)
	if err != nil {
		return protocol.Response{}, nil, err
	}
	if response.ErrorCode != 0 {
		return response, nil, &RemoteError{Code: syscall.Errno(response.ErrorCode), Message: response.Error}
	}
	return response, payload, nil
}

func (r *Remote) ReadDir(ctx context.Context, path string) ([]protocol.DirEntry, error) {
	stream, err := r.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	stop := bindStreamContext(ctx, stream)
	defer stop()
	if err := protocol.WriteRequest(stream, protocol.Request{Kind: protocol.KindFile, Operation: "readdir", Path: path}, nil); err != nil {
		return nil, err
	}
	var entries []protocol.DirEntry
	for {
		response, payload, err := protocol.ReadResponse(stream)
		if err != nil {
			return nil, err
		}
		if response.ErrorCode != 0 {
			return nil, &RemoteError{Code: syscall.Errno(response.ErrorCode), Message: response.Error}
		}
		if len(payload) > 0 {
			page, err := protocol.DecodeDirEntries(payload)
			if err != nil {
				return nil, err
			}
			entries = append(entries, page...)
		}
		if !response.More {
			return entries, nil
		}
	}
}
