package protocol

import (
	"encoding/json"
	"fmt"
)

func EncodeDirEntries(entries []DirEntry) ([]byte, error) {
	data, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal directory entries: %w", err)
	}
	if len(data) > MaxResponsePayloadBytes {
		return nil, fmt.Errorf("directory entries exceed %d bytes", MaxResponsePayloadBytes)
	}
	return data, nil
}

func EncodeDirEntryPages(entries []DirEntry) ([][]byte, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	data, err := EncodeDirEntries(entries)
	if err == nil && len(data) <= DirectoryPagePayloadBytes {
		return [][]byte{data}, nil
	}
	if len(entries) == 1 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("directory entry exceeds %d bytes", DirectoryPagePayloadBytes)
	}
	middle := len(entries) / 2
	left, err := EncodeDirEntryPages(entries[:middle])
	if err != nil {
		return nil, err
	}
	right, err := EncodeDirEntryPages(entries[middle:])
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

func DecodeDirEntries(data []byte) ([]DirEntry, error) {
	var entries []DirEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode directory entries: %w", err)
	}
	return entries, nil
}
