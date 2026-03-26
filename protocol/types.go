// Package protocol defines the binary wire format for QuicFrame.
//
// Wire format per frame:
//
//	┌──────────────────┐
//	│  FrameLen  (4B)  │  ← big-endian uint32: byte count of Type + Payload
//	├──────────────────┤
//	│  FrameType (1B)  │  ← discriminator byte
//	├──────────────────┤
//	│  Payload (msgpack│  ← msgpack-encoded struct
//	└──────────────────┘
//
// All strings are UTF-8. All integers are msgpack-native (variable-length).
package protocol

// FrameType discriminates the payload structure.
type FrameType uint8

const (
	FrameTypeRequest    FrameType = 0x01 // client → server: initiate a call
	FrameTypeResponse   FrameType = 0x02 // server → client: non-streaming reply
	FrameTypeStreamData FrameType = 0x03 // server → client: streaming chunk
	FrameTypeStreamEnd  FrameType = 0x04 // server → client: final chunk
	FrameTypeError      FrameType = 0x05 // either direction: protocol/app error
	FrameTypePing       FrameType = 0x06 // keep-alive probe
	FrameTypePong       FrameType = 0x07 // keep-alive reply
)

// Status codes mirror HTTP semantics for developer familiarity.
const (
	StatusOK                  = 200
	StatusCreated             = 201
	StatusAccepted            = 202
	StatusNoContent           = 204
	StatusBadRequest          = 400
	StatusUnauthorized        = 401
	StatusForbidden           = 403
	StatusNotFound            = 404
	StatusMethodNotAllowed    = 405
	StatusConflict            = 409
	StatusUnprocessableEntity = 422
	StatusTooManyRequests     = 429
	StatusInternalServerError = 500
	StatusNotImplemented      = 501
	StatusServiceUnavailable  = 503
)

// Request is the msgpack payload carried by FrameTypeRequest.
// Sent by clients (native SDK or browser) to initiate a call.
type Request struct {
	ID      string            `msgpack:"id"`               // client-generated UUID
	Method  string            `msgpack:"method"`           // GET, POST, PUT, DELETE, PATCH, …
	Path    string            `msgpack:"path"`             // absolute path, e.g. /users/42
	Headers map[string]string `msgpack:"headers,omitempty"` // arbitrary key-value metadata
	Body    []byte            `msgpack:"body,omitempty"`   // msgpack-encoded request body
	Stream  bool              `msgpack:"stream,omitempty"` // true: client expects a streamed response
}

// Response is the msgpack payload carried by FrameTypeResponse.
// Sent by the server for non-streaming (full) replies.
type Response struct {
	ID      string            `msgpack:"id"`               // echoes Request.ID
	Status  int               `msgpack:"status"`           // numeric status code
	Headers map[string]string `msgpack:"headers,omitempty"`
	Body    []byte            `msgpack:"body,omitempty"`   // msgpack-encoded response body
	Stream  bool              `msgpack:"stream,omitempty"` // true: FrameTypeStreamData frames follow
}

// StreamChunk is the msgpack payload for FrameTypeStreamData / FrameTypeStreamEnd.
// The server emits a sequence of StreamChunks for a streaming response.
// The final chunk has Final=true and is sent as FrameTypeStreamEnd.
type StreamChunk struct {
	ID    string `msgpack:"id"`    // echoes Request.ID
	Seq   uint64 `msgpack:"seq"`   // monotonically increasing, 0-based
	Data  []byte `msgpack:"data"`  // chunk payload (empty on final chunk)
	Final bool   `msgpack:"final"` // true on the last chunk
}

// ErrorFrame is the msgpack payload carried by FrameTypeError.
type ErrorFrame struct {
	ID      string `msgpack:"id"`      // echoes Request.ID; empty for connection-level errors
	Code    int    `msgpack:"code"`    // status code
	Message string `msgpack:"message"` // human-readable error
}

// PingFrame is carried by FrameTypePing / FrameTypePong.
type PingFrame struct {
	Timestamp int64 `msgpack:"ts"` // unix nanoseconds
}

// MaxFrameSize caps individual frame sizes to prevent memory exhaustion.
const MaxFrameSize = 64 * 1024 * 1024 // 64 MiB
