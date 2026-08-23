package provider

import (
	"context"
	"syscall"
	"time"

	"github.com/gesta-run/colnk/pkg/protocol"
	providerfs "github.com/gesta-run/colnk/pkg/provider/filesystem"
	"github.com/gesta-run/colnk/pkg/transport"
)

func (s *Service) handleFile(ctx context.Context, stream transport.Stream, request protocol.Request) {
	select {
	case s.fileSlots <- struct{}{}:
		defer func() { <-s.fileSlots }()
	default:
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EAGAIN), Error: "file request limit reached"}, nil)
		_ = stream.Close()
		return
	}
	weight := fileRequestWeight(request)
	if weight <= 0 || weight > maxInFlightFileBytes {
		_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: int(syscall.EINVAL), Error: "invalid file payload size"}, nil)
		_ = stream.Close()
		return
	}
	if err := s.payloadBudget.Acquire(ctx, weight); err != nil {
		_ = stream.Close()
		return
	}
	defer s.payloadBudget.Release(weight)
	data, err := protocol.ReadRequestPayload(stream, request)
	_ = stream.SetReadDeadline(time.Time{})
	if err != nil {
		s.logger.Warn("read file payload", "error", err)
		_ = stream.Close()
		return
	}
	if request.Operation == "readdir" {
		s.handleReadDir(stream, request, len(data))
		return
	}
	response, payload := s.files.Handle(request, data)
	s.logFileAudit(request, response.ErrorCode, len(data), len(payload))
	_ = protocol.WriteResponse(stream, response, payload)
	_ = stream.Close()
}

func (s *Service) handleReadDir(stream transport.Stream, request protocol.Request, bytesIn int) {
	bytesOut := 0
	writeFailed := false
	err := s.files.StreamDir(request.Path, func(payload []byte, more bool) error {
		bytesOut += len(payload)
		if err := protocol.WriteResponse(stream, protocol.Response{More: more}, payload); err != nil {
			writeFailed = true
			return err
		}
		return nil
	})
	resultCode := 0
	if err != nil {
		resultCode = int(providerfs.Errno(err))
		if !writeFailed {
			_ = protocol.WriteResponse(stream, protocol.Response{ErrorCode: resultCode, Error: err.Error()}, nil)
		}
	}
	s.logFileAudit(request, resultCode, bytesIn, bytesOut)
	_ = stream.Close()
}

func requestHasPayload(request protocol.Request) bool {
	return request.DataLength != 0 || request.RawDataLength != 0 || request.DataEncoding != ""
}
