package protocol

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// WriteFrame serialises payload as msgpack and writes a length-prefixed frame
// to w. Thread-safe if the caller ensures exclusive access to w.
//
//	[4B BE uint32 frameLen][1B frameType][msgpack(payload)]
func WriteFrame(w io.Writer, frameType FrameType, payload interface{}) error {
	encoded, err := msgpack.Marshal(payload)
	if err != nil {
		return fmt.Errorf("quicframe/protocol: marshal: %w", err)
	}

	// frameLen = 1 (type byte) + len(encoded)
	frameLen := uint32(1 + len(encoded))
	if frameLen > MaxFrameSize {
		return fmt.Errorf("quicframe/protocol: payload too large (%d bytes)", frameLen)
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, frameLen)

	if _, err = w.Write(header); err != nil {
		return fmt.Errorf("quicframe/protocol: write header: %w", err)
	}
	if _, err = w.Write([]byte{byte(frameType)}); err != nil {
		return fmt.Errorf("quicframe/protocol: write type: %w", err)
	}
	if _, err = w.Write(encoded); err != nil {
		return fmt.Errorf("quicframe/protocol: write payload: %w", err)
	}
	return nil
}

// ReadFrame reads one frame from r, returning its type and raw msgpack payload.
// Callers decode the payload with DecodeXxx helpers.
func ReadFrame(r io.Reader) (FrameType, []byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, fmt.Errorf("quicframe/protocol: read header: %w", err)
	}

	frameLen := binary.BigEndian.Uint32(header)
	if frameLen == 0 || frameLen > MaxFrameSize {
		return 0, nil, fmt.Errorf("quicframe/protocol: invalid frame length %d", frameLen)
	}

	buf := make([]byte, frameLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, fmt.Errorf("quicframe/protocol: read body: %w", err)
	}

	return FrameType(buf[0]), buf[1:], nil
}

// DecodeRequest decodes a Request from a raw msgpack payload.
func DecodeRequest(data []byte) (*Request, error) {
	var req Request
	if err := msgpack.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("quicframe/protocol: decode request: %w", err)
	}
	return &req, nil
}

// DecodeResponse decodes a Response from a raw msgpack payload.
func DecodeResponse(data []byte) (*Response, error) {
	var resp Response
	if err := msgpack.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("quicframe/protocol: decode response: %w", err)
	}
	return &resp, nil
}

// DecodeStreamChunk decodes a StreamChunk from a raw msgpack payload.
func DecodeStreamChunk(data []byte) (*StreamChunk, error) {
	var chunk StreamChunk
	if err := msgpack.Unmarshal(data, &chunk); err != nil {
		return nil, fmt.Errorf("quicframe/protocol: decode stream chunk: %w", err)
	}
	return &chunk, nil
}

// DecodeError decodes an ErrorFrame from a raw msgpack payload.
func DecodeError(data []byte) (*ErrorFrame, error) {
	var ef ErrorFrame
	if err := msgpack.Unmarshal(data, &ef); err != nil {
		return nil, fmt.Errorf("quicframe/protocol: decode error: %w", err)
	}
	return &ef, nil
}

// WriteError is a convenience wrapper for sending an error frame.
func WriteError(w io.Writer, requestID string, code int, message string, data []byte) error {
	return WriteFrame(w, FrameTypeError, &ErrorFrame{
		ID:      requestID,
		Code:    code,
		Message: message,
		Data:    data,
	})
}
