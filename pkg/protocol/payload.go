package protocol

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
)

const (
	payloadEncodingFlate   = "flate"
	minCompressiblePayload = 4 << 10
)

func encodePayload(data []byte) ([]byte, string, int, error) {
	if len(data) < minCompressiblePayload {
		return data, "", 0, nil
	}
	var encoded bytes.Buffer
	writer, err := flate.NewWriter(&encoded, flate.BestSpeed)
	if err != nil {
		return nil, "", 0, fmt.Errorf("create payload compressor: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return nil, "", 0, fmt.Errorf("compress payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", 0, fmt.Errorf("finish payload compression: %w", err)
	}
	if encoded.Len() >= len(data)-len(data)/16 {
		return data, "", 0, nil
	}
	return encoded.Bytes(), payloadEncodingFlate, len(data), nil
}

func decodePayload(data []byte, encoding string, rawLength, maximum int) ([]byte, error) {
	if encoding == "" {
		if rawLength != 0 {
			return nil, errors.New("raw payload length requires an encoding")
		}
		return data, nil
	}
	if encoding != payloadEncodingFlate {
		return nil, fmt.Errorf("unsupported payload encoding %q", encoding)
	}
	if rawLength <= 0 || rawLength > maximum {
		return nil, fmt.Errorf("invalid raw payload length %d", rawLength)
	}
	reader := flate.NewReader(bytes.NewReader(data))
	defer reader.Close()
	var decoded bytes.Buffer
	decoded.Grow(rawLength)
	if _, err := io.Copy(&decoded, io.LimitReader(reader, int64(rawLength)+1)); err != nil {
		return nil, fmt.Errorf("decompress payload: %w", err)
	}
	if decoded.Len() != rawLength {
		return nil, fmt.Errorf("decompressed payload length is %d, expected %d", decoded.Len(), rawLength)
	}
	return decoded.Bytes(), nil
}

func writePayload(w io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	_, err := w.Write(data)
	return err
}

func readPayload(r io.Reader, length int, maximum int) ([]byte, error) {
	if length < 0 || length > maximum {
		return nil, fmt.Errorf("invalid payload length %d", length)
	}
	if length == 0 {
		return nil, nil
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}
