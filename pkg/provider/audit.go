package provider

import "github.com/gesta-run/colnk/pkg/protocol"

func (s *Service) logFileAudit(request protocol.Request, resultCode, bytesIn, bytesOut int) {
	attributes := []any{"operation", request.Operation, "result_code", resultCode, "bytes_in", bytesIn, "bytes_out", bytesOut}
	if s.auditResources {
		attributes = append(attributes, "path", request.Path)
	}
	s.logger.Info("file audit", attributes...)
}

func fileRequestWeight(request protocol.Request) int64 {
	incoming := max(request.DataLength, request.RawDataLength)
	if incoming < 0 || incoming > protocol.MaxPayloadBytes {
		return -1
	}
	return int64(incoming + protocol.MaxPayloadBytes)
}

func (s *Service) logNetworkAudit(kind, resource, result, reason string) {
	attributes := []any{"result", result, "reason", reason}
	if s.auditResources {
		attributes = append(attributes, "resource", resource)
	}
	s.logger.Info(kind+" audit", attributes...)
}
