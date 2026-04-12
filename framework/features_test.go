package quicframe

import (
	"bytes"
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"testing"

	"github.com/msmecodex/quicframe/protocol"
	"github.com/vmihailenco/msgpack/v5"
)

type mockPeer struct{}
func (m *mockPeer) RemoteAddr() net.Addr { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234} }
func (m *mockPeer) PeerCertificates() []*x509.Certificate { return nil }

func TestContext_HeadersAndCookies(t *testing.T) {
	req := &protocol.Request{
		ID: "test-req",
		Headers: map[string]string{
			"Cookie": "session=abc; theme=dark",
		},
	}
	stream := new(bytes.Buffer)
	ctx := newContext(context.Background(), req, &mockPeerWrapper{}, stream)

	// Test GetHeader (renamed from Header)
	if ctx.GetHeader("Cookie") != req.Headers["Cookie"] {
		t.Errorf("GetHeader failed to retrieve request header")
	}

	// Test Header (setter)
	ctx.Header("X-Response", "active")
	if ctx.resHeaders["X-Response"] != "active" {
		t.Errorf("Header setter failed to store pending header")
	}

	// Test Cookie (getter)
	cookie, err := ctx.Cookie("session")
	if err != nil || cookie.Value != "abc" {
		t.Errorf("Cookie getter failed: %v", err)
	}

	// Test SetCookie (setter)
	ctx.SetCookie(&http.Cookie{Name: "pref", Value: "1"})
	if ctx.resHeaders["Set-Cookie"] == "" {
		t.Errorf("SetCookie failed to set Set-Cookie header")
	}

	// Test writeResponse merging
	err = ctx.writeResponse(200, map[string]string{"X-Immediate": "yes"}, nil, false)
	if err != nil {
		t.Fatalf("writeResponse failed: %v", err)
	}

	ft, data, _ := protocol.ReadFrame(stream)
	if ft != protocol.FrameTypeResponse {
		t.Fatalf("Expected response frame")
	}
	resp, _ := protocol.DecodeResponse(data)
	
	if resp.Headers["X-Response"] != "active" || resp.Headers["X-Immediate"] != "yes" {
		t.Errorf("Header merging failed: %v", resp.Headers)
	}
	if resp.Headers["Set-Cookie"] == "" {
		t.Errorf("Cookie header missing in merged response")
	}
}

func TestContext_ErrorWithData(t *testing.T) {
	stream := new(bytes.Buffer)
	ctx := newContext(context.Background(), &protocol.Request{ID: "req1"}, &mockPeerWrapper{}, stream)

	type extra struct {
		Field string `msgpack:"field"`
	}
	errData := extra{Field: "context"}
	
	err := ctx.Error(400, "bad request", errData)
	if err != nil {
		t.Fatalf("Error call failed: %v", err)
	}

	_, data, _ := protocol.ReadFrame(stream)
	ef, _ := protocol.DecodeError(data)
	
	if ef.Code != 400 || ef.Message != "bad request" {
		t.Errorf("Error frame basic fields mismatch: %+v", ef)
	}

	var decodedExtra extra
	if err := msgpack.Unmarshal(ef.Data, &decodedExtra); err != nil {
		t.Fatalf("Failed to decode error data: %v", err)
	}
	if decodedExtra.Field != "context" {
		t.Errorf("Error data content mismatch")
	}
}

func TestApp_AfterMiddleware(t *testing.T) {
	app := New()
	
	var afterCalled bool
	app.After(func(c *Context) error {
		afterCalled = true
		c.Set("after", "ok")
		return nil
	})

	app.GET("/test", func(c *Context) error {
		return c.Send(200, []byte("ok"))
	})

	// Simulate dispatchStream
	req := &protocol.Request{Method: "GET", Path: "/test", ID: "1"}
	stream := new(bytes.Buffer)
	qfCtx := newContext(context.Background(), req, &mockPeerWrapper{}, stream)
	
	handler, _ := app.router.match("GET", "/test")
	h := applyMiddleware(handler, app.middleware)
	
	_ = h(qfCtx)
	
	// Manually run After middleware as in dispatchStream
	for _, after := range app.afterMiddleware {
		_ = after(qfCtx)
	}

	if !afterCalled {
		t.Errorf("After middleware was not called")
	}
	val, _ := qfCtx.Get("after")
	if val != "ok" {
		t.Errorf("After middleware fail to set context value")
	}
}

type mockPeerWrapper struct{}

func (m *mockPeerWrapper) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
}

func (m *mockPeerWrapper) PeerCertificates() []*x509.Certificate {
	return nil
}
