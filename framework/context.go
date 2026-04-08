package quicframe

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/msmecodex/quicframe/protocol"
	"github.com/vmihailenco/msgpack/v5"
)

// Context carries all state for a single request/response cycle.
// It is not safe for concurrent use; middleware and handlers run serially
// on the same goroutine for each stream.
type Context struct {
	// Request is the decoded incoming frame.
	Request *protocol.Request

	// conn provides access to connection-level metadata (remote addr, etc.)
	conn remoteAddrProvider

	// stream is the underlying QUIC bidirectional stream.
	// Shared with the StreamWriter; close is handled by the transport layer.
	stream io.ReadWriter

	// params holds route path parameters (e.g. /users/:id → "id" → "42").
	params map[string]string

	// locals is a per-request key-value store for middleware communication.
	locals sync.Map

	// sent guards against double-sends.
	sent atomic.Bool

	// ctx is the underlying request context.
	ctx context.Context
}

// remoteAddrProvider is satisfied by quic.Connection and webtransport.Session.
type remoteAddrProvider interface {
	RemoteAddr() net.Addr
	PeerCertificates() []*x509.Certificate
}

func newContext(ctx context.Context, req *protocol.Request, conn remoteAddrProvider, stream io.ReadWriter) *Context {
	return &Context{
		ctx:     ctx,
		Request: req,
		conn:    conn,
		stream:  stream,
	}
}

// ─── Request accessors ───────────────────────────────────────────────────────

// Param returns the value of the named route parameter (e.g. "id" for /users/:id).
func (c *Context) Param(key string) string { return c.params[key] }

// Header returns the value of the named request header (case-sensitive).
func (c *Context) Header(key string) string {
	if c.Request.Headers == nil {
		return ""
	}
	return c.Request.Headers[key]
}

// Method returns the request method string (GET, POST, …).
func (c *Context) Method() string { return c.Request.Method }

// Path returns the raw request path.
func (c *Context) Path() string { return c.Request.Path }

// RequestID returns the client-supplied request UUID.
func (c *Context) RequestID() string { return c.Request.ID }

// Body returns the raw msgpack-encoded request body bytes.
func (c *Context) Body() []byte { return c.Request.Body }

// Bind decodes the request body (msgpack) into v.
func (c *Context) Bind(v interface{}) error {
	if len(c.Request.Body) == 0 {
		return nil
	}
	if err := msgpack.Unmarshal(c.Request.Body, v); err != nil {
		return fmt.Errorf("quicframe: bind: %w", err)
	}
	return nil
}

// BindAndValidate decodes the request body and validates it using `validate` tags.
func (c *Context) BindAndValidate(v interface{}) error {
	if err := c.Bind(v); err != nil {
		return err
	}
	return Validate(v)
}

// RemoteAddr returns the network address of the connected peer.
func (c *Context) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// PeerCertificates returns the certificate chain provided by the peer.
func (c *Context) PeerCertificates() []*x509.Certificate {
	return c.conn.PeerCertificates()
}

// Client returns a quicframe Client that reuses the underlying connection.
// This is only supported for native QUIC connections; returns nil for WebTransport.
func (c *Context) Client() *Client {
	if cw, ok := c.conn.(*quicConnWrapper); ok {
		return NewClientFromConn(cw.Conn)
	}
	return nil
}

// Context returns the underlying request context.
func (c *Context) Context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// ─── Context locals ──────────────────────────────────────────────────────────

// Set stores an arbitrary value in the request-scoped locals map.
func (c *Context) Set(key string, value interface{}) { c.locals.Store(key, value) }

// Get retrieves a previously stored value from the request-scoped locals map.
// The second return value is false if the key has not been set.
func (c *Context) Get(key string) (interface{}, bool) { return c.locals.Load(key) }

// MustGet retrieves a value from locals and panics if it is missing.
func (c *Context) MustGet(key string) interface{} {
	v, ok := c.locals.Load(key)
	if !ok {
		panic(fmt.Sprintf("quicframe: MustGet: key %q not found in locals", key))
	}
	return v
}

// ─── Response helpers ────────────────────────────────────────────────────────

// Send writes a raw-byte response with the given status code.
func (c *Context) Send(status int, body []byte) error {
	return c.writeResponse(status, nil, body, false)
}

// MsgPack encodes v as msgpack and sends it as a response.
func (c *Context) MsgPack(status int, v interface{}) error {
	data, err := msgpack.Marshal(v)
	if err != nil {
		return fmt.Errorf("quicframe: MsgPack: %w", err)
	}
	headers := map[string]string{"content-type": "application/x-msgpack"}
	return c.writeResponse(status, headers, data, false)
}

// NoContent sends a 204 No Content response with an empty body.
func (c *Context) NoContent() error {
	return c.writeResponse(protocol.StatusNoContent, nil, nil, false)
}

// Error sends an error frame for the current request.
func (c *Context) Error(code int, message string) error {
	if !c.sent.CompareAndSwap(false, true) {
		return nil // already sent
	}
	return protocol.WriteError(c.stream, c.Request.ID, code, message)
}

// NewStream opens a streaming response.  The caller MUST call StreamWriter.Close()
// when done.  Once NewStream is called no other response method may be used.
func (c *Context) NewStream(status int, headers map[string]string) (*StreamWriter, error) {
	if !c.sent.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("quicframe: response already sent")
	}
	resp := &protocol.Response{
		ID:      c.Request.ID,
		Status:  status,
		Headers: headers,
		Stream:  true,
	}
	if err := protocol.WriteFrame(c.stream, protocol.FrameTypeResponse, resp); err != nil {
		return nil, err
	}
	return &StreamWriter{stream: c.stream, requestID: c.Request.ID}, nil
}

func (c *Context) writeResponse(status int, headers map[string]string, body []byte, stream bool) error {
	if !c.sent.CompareAndSwap(false, true) {
		return fmt.Errorf("quicframe: response already sent")
	}
	resp := &protocol.Response{
		ID:      c.Request.ID,
		Status:  status,
		Headers: headers,
		Body:    body,
		Stream:  stream,
	}
	return protocol.WriteFrame(c.stream, protocol.FrameTypeResponse, resp)
}

// setParams is called by the router after a successful match.
func (c *Context) setParams(params map[string]string) { c.params = params }

// ─── StreamWriter ────────────────────────────────────────────────────────────

// StreamWriter sends a sequence of chunks over a QUIC stream.
// Obtain one via Context.NewStream.
type StreamWriter struct {
	stream    io.Writer
	requestID string
	seq       uint64
	mu        sync.Mutex
	closed    bool
}

// Write sends a single chunk.  May be called from multiple goroutines.
func (sw *StreamWriter) Write(data []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.closed {
		return 0, fmt.Errorf("quicframe: StreamWriter already closed")
	}
	chunk := &protocol.StreamChunk{
		ID:   sw.requestID,
		Seq:  sw.seq,
		Data: data,
	}
	sw.seq++
	if err := protocol.WriteFrame(sw.stream, protocol.FrameTypeStreamData, chunk); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Close sends the terminal StreamEnd frame.
func (sw *StreamWriter) Close() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.closed {
		return nil
	}
	sw.closed = true
	chunk := &protocol.StreamChunk{
		ID:    sw.requestID,
		Seq:   sw.seq,
		Final: true,
	}
	return protocol.WriteFrame(sw.stream, protocol.FrameTypeStreamEnd, chunk)
}
