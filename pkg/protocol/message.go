package protocol

import (
	"fmt"
	"io"
)

func WriteRequest(w io.Writer, request Request, data []byte) error {
	if len(data) > MaxPayloadBytes {
		return fmt.Errorf("request payload exceeds %d bytes", MaxPayloadBytes)
	}
	payload, encoding, rawLength, err := encodePayload(data)
	if err != nil {
		return err
	}
	request.DataLength = len(payload)
	request.DataEncoding = encoding
	request.RawDataLength = rawLength
	if err := WriteJSON(w, request); err != nil {
		return err
	}
	return writePayload(w, payload)
}

func ReadRequest(r io.Reader) (Request, []byte, error) {
	request, err := ReadRequestHeader(r)
	if err != nil {
		return Request{}, nil, err
	}
	data, err := ReadRequestPayload(r, request)
	return request, data, err
}

func ReadRequestHeader(r io.Reader) (Request, error) {
	var request Request
	if err := ReadJSON(r, &request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func ReadRequestPayload(r io.Reader, request Request) ([]byte, error) {
	data, err := readPayload(r, request.DataLength, MaxPayloadBytes)
	if err != nil {
		return nil, err
	}
	return decodePayload(data, request.DataEncoding, request.RawDataLength, MaxPayloadBytes)
}

func WriteResponse(w io.Writer, response Response, data []byte) error {
	if len(data) > MaxResponsePayloadBytes {
		return fmt.Errorf("response payload exceeds %d bytes", MaxResponsePayloadBytes)
	}
	payload, encoding, rawLength, err := encodePayload(data)
	if err != nil {
		return err
	}
	response.DataLength = len(payload)
	response.DataEncoding = encoding
	response.RawDataLength = rawLength
	if err := WriteJSON(w, response); err != nil {
		return err
	}
	return writePayload(w, payload)
}

func ReadResponse(r io.Reader) (Response, []byte, error) {
	var response Response
	if err := ReadJSON(r, &response); err != nil {
		return Response{}, nil, err
	}
	data, err := readPayload(r, response.DataLength, MaxResponsePayloadBytes)
	if err != nil {
		return Response{}, nil, err
	}
	data, err = decodePayload(data, response.DataEncoding, response.RawDataLength, MaxResponsePayloadBytes)
	return response, data, err
}
