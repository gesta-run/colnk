package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	want := Request{Kind: KindFile, Operation: "write", Path: "tmp/test", Offset: 7}
	payload := []byte("test-payload")
	if err := WriteRequest(&buffer, want, payload); err != nil {
		t.Fatal(err)
	}
	got, gotPayload, err := ReadRequest(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != want.Kind || got.Operation != want.Operation || got.Path != want.Path || got.Offset != want.Offset {
		t.Fatalf("request mismatch: %#v", got)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("payload mismatch: %q", gotPayload)
	}
}

func TestCompressiblePayloadRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte("source-code-line\n"), MaxPayloadBytes/17)
	var wire bytes.Buffer
	if err := WriteRequest(&wire, Request{Kind: KindFile, Operation: "write"}, payload); err != nil {
		t.Fatal(err)
	}
	encodedSize := wire.Len()
	if encodedSize >= len(payload)/4 {
		t.Fatalf("compressible payload remained too large: %d", encodedSize)
	}
	request, decoded, err := ReadRequest(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if request.DataEncoding != payloadEncodingFlate || request.RawDataLength != len(payload) {
		t.Fatalf("compression metadata mismatch: %#v", request)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatal("compressed payload changed during round trip")
	}
}

func TestRejectsInvalidCompressedPayload(t *testing.T) {
	var wire bytes.Buffer
	request := Request{
		Kind: KindFile, DataLength: 4, DataEncoding: payloadEncodingFlate, RawDataLength: MaxPayloadBytes + 1,
	}
	if err := WriteJSON(&wire, request); err != nil {
		t.Fatal(err)
	}
	wire.WriteString("fake")
	if _, _, err := ReadRequest(&wire); err == nil {
		t.Fatal("accepted an oversized compressed payload")
	}
}

func TestRejectsMalformedFrameLengths(t *testing.T) {
	for _, length := range []uint32{0, MaxHeaderBytes + 1} {
		var buffer bytes.Buffer
		if err := binary.Write(&buffer, binary.BigEndian, length); err != nil {
			t.Fatal(err)
		}
		if err := ReadJSON(&buffer, &Request{}); err == nil {
			t.Fatalf("accepted invalid header length %d", length)
		}
	}
}

func TestRejectsInvalidDeclaredPayloadLength(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteJSON(&buffer, Request{Kind: KindFile, DataLength: -1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRequest(&buffer); err == nil {
		t.Fatal("accepted negative payload length")
	}
}

func TestPayloadLimit(t *testing.T) {
	data := []byte(strings.Repeat("x", MaxPayloadBytes+1))
	if err := WriteRequest(&bytes.Buffer{}, Request{}, data); err == nil {
		t.Fatal("expected oversized payload to fail")
	}
	if err := WriteResponse(&bytes.Buffer{}, Response{}, data); err != nil {
		t.Fatalf("expected response payload larger than a file chunk to succeed: %v", err)
	}
	data = []byte(strings.Repeat("x", MaxResponsePayloadBytes+1))
	if err := WriteResponse(&bytes.Buffer{}, Response{}, data); err == nil {
		t.Fatal("expected oversized response payload to fail")
	}
}

func TestDirectoryEntriesRoundTrip(t *testing.T) {
	want := []DirEntry{{Name: "hello.txt", Attr: FileAttr{Mode: 0o100640, Size: 42, ModTimeNS: 7}, LinkTarget: "target.txt"}}
	data, err := EncodeDirEntries(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDirEntries(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("directory entries mismatch: %#v", got)
	}
}

func TestDirectoryEntriesSplitIntoBoundedPages(t *testing.T) {
	entries := make([]DirEntry, 40)
	for index := range entries {
		entries[index] = DirEntry{Name: fmt.Sprintf("entry-%d", index), LinkTarget: strings.Repeat("x", 32<<10)}
	}
	pages, err := EncodeDirEntryPages(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 2 {
		t.Fatal("large directory response was not split")
	}
	decoded := 0
	for _, page := range pages {
		if len(page) > DirectoryPagePayloadBytes {
			t.Fatalf("directory page exceeded limit: %d", len(page))
		}
		values, err := DecodeDirEntries(page)
		if err != nil {
			t.Fatal(err)
		}
		decoded += len(values)
	}
	if decoded != len(entries) {
		t.Fatalf("decoded %d entries; want %d", decoded, len(entries))
	}
}

func TestValidateHandshake(t *testing.T) {
	valid := Handshake{MajorVersion: MajorVersion, MinorVersion: MinorVersion, APIKey: "sk-test"}
	if err := ValidateHandshake(valid); err != nil {
		t.Fatalf("valid handshake rejected: %v", err)
	}
	valid.MinorVersion++
	if err := ValidateHandshake(valid); err != nil {
		t.Fatalf("newer minor version rejected: %v", err)
	}
	valid.MajorVersion++
	if err := ValidateHandshake(valid); err == nil {
		t.Fatal("incompatible major version accepted")
	}
}
