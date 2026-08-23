package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

func WriteJSON(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if len(data) > MaxHeaderBytes {
		return fmt.Errorf("message header exceeds %d bytes", MaxHeaderBytes)
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if _, err := w.Write(size[:]); err != nil {
		return fmt.Errorf("write message size: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

func ReadJSON(r io.Reader, value any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return fmt.Errorf("read message size: %w", err)
	}
	length := binary.BigEndian.Uint32(size[:])
	if length == 0 || length > MaxHeaderBytes {
		return fmt.Errorf("invalid message header length %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	return nil
}
