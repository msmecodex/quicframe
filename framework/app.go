// Package quicframe is a production-grade QUIC-native API framework.
//
// It provides an Express/Fiber-style developer experience over raw QUIC streams
// using a compact binary protocol (MsgPack framing) instead of HTTP.
//
// Two transports are supported:
//   - Native QUIC (ALPN "qf/1")  – for Go/Rust native clients
//   - WebTransport (HTTP/3)      – for browser clients
//
// Both transports share the same router, middleware, and handler code.
//
// Quick start:
//
//	app := quicframe.New()
//	app.Use(middleware.Logger(), middleware.Recovery())
//
//	app.GET("/hello", func(ctx *quicframe.Context) error {
//	    return ctx.MsgPack(200, map[string]string{"msg": "hello"})
//	})
//
//	log.Fatal(app.ListenNative(context.Background(), ":4433", tlsCfg))
package quicframe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/msmecodex/quicframe/protocol"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// App is the top-level framework object.  Create one with New().
type App struct {
	router     *Router
	middleware []MiddlewareFunc
	logger     *slog.Logger
	quicCfg    *quic.Config
	wg         sync.WaitGroup
	pkiManager interface{} // Use interface to avoid circular dependency or keep it internal
}

// New creates a new App with sensible production defaults.
func New() *App {
	return &App{
		router: newRouter(),
		logger: slog.Default(),
		quicCfg: &quic.Config{
			MaxIncomingStreams:    4096,
			MaxIncomingUniStreams: 256,
			KeepAlivePeriod:       30 * time.Second,
			MaxIdleTimeout:        5 * time.Minute,
			EnableDatagrams:       true,
		},
	}
}

// ─── Middleware & routing ────────────────────────────────────────────────────

// Use appends application-level middleware that runs before every handler.
func (a *App) Use(mw ...MiddlewareFunc) *App {
	a.middleware = append(a.middleware, mw...)
	return a
}

// GET registers a handler for GET requests matching pattern.
func (a *App) GET(pattern string, h HandlerFunc) *App {
	a.addRoute("GET", pattern, nil, h)
	return a
}

// POST registers a handler for POST requests matching pattern.
func (a *App) POST(pattern string, h HandlerFunc) *App {
	a.addRoute("POST", pattern, nil, h)
	return a
}

// PUT registers a handler for PUT requests matching pattern.
func (a *App) PUT(pattern string, h HandlerFunc) *App {
	a.addRoute("PUT", pattern, nil, h)
	return a
}

// DELETE registers a handler for DELETE requests matching pattern.
func (a *App) DELETE(pattern string, h HandlerFunc) *App {
	a.addRoute("DELETE", pattern, nil, h)
	return a
}

// PATCH registers a handler for PATCH requests matching pattern.
func (a *App) PATCH(pattern string, h HandlerFunc) *App {
	a.addRoute("PATCH", pattern, nil, h)
	return a
}

// Group creates a route group sharing a URL prefix and optional middleware.
//
//	api := app.Group("/api/v1", middleware.JWTSimple(secret))
//	api.GET("/users", listUsers)
func (a *App) Group(prefix string, mw ...MiddlewareFunc) *Group {
	return &Group{app: a, prefix: prefix, middleware: mw}
}

// addRoute is the internal registration point (used by App and Group).
func (a *App) addRoute(method, pattern string, groupMW []MiddlewareFunc, h HandlerFunc) {
	a.router.add(method, pattern, groupMW, h)
}

// ListenNative starts a raw-QUIC listener (ALPN "qf/1") on addr.
// Intended for Go and Rust native SDK clients.
// Blocks until ctx is cancelled or a fatal error occurs.
func (a *App) ListenNative(ctx context.Context, addr string, tlsCfg *tls.Config) error {
	cfg := tlsCfg.Clone()
	cfg.NextProtos = prepend("qf/1", cfg.NextProtos)

	listener, err := quic.ListenAddr(addr, cfg, a.quicCfg)
	if err != nil {
		return fmt.Errorf("quicframe: ListenNative: %w", err)
	}
	defer listener.Close()

	a.logger.Info("quicframe native transport listening", "addr", addr, "alpn", "qf/1")

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // graceful shutdown
			}
			return fmt.Errorf("quicframe: accept: %w", err)
		}
		a.wg.Add(1)
		go func(c *quic.Conn) {
			defer a.wg.Done()
			a.handleNativeConn(&quicConnWrapper{c})
		}(conn)
	}
}

// ─── WebTransport transport ──────────────────────────────────────────────────

// ListenWebTransport starts an HTTP/3 + WebTransport listener on addr.
// Browser clients connect to the /wt endpoint via the WebTransport API.
// Blocks until the server encounters a fatal error.
func (a *App) ListenWebTransport(addr string, tlsCfg *tls.Config) error {
	cfg := http3.ConfigureTLSConfig(tlsCfg.Clone())

	h3srv := &http3.Server{
		Addr:       addr,
		TLSConfig:  cfg,
		QUICConfig: a.quicCfg,
	}
	webtransport.ConfigureHTTP3Server(h3srv)

	wtSrv := &webtransport.Server{
		H3:                   h3srv,
		ApplicationProtocols: []string{"quicframe"},
		CheckOrigin:          func(r *http.Request) bool { return true }, // tighten in production
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/wt", func(w http.ResponseWriter, r *http.Request) {
		session, err := wtSrv.Upgrade(w, r)
		if err != nil {
			a.logger.Error("webtransport upgrade failed", "err", err)
			return
		}
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.handleWebTransportSession(&wtSessionWrapper{session})
		}()
	})

	h3srv.Handler = mux

	a.logger.Info("quicframe WebTransport listening", "addr", addr, "endpoint", "/wt")
	return wtSrv.ListenAndServe()
}

// ListenAddr is a convenience wrapper that starts both transports concurrently.
//
//	nativeAddr = ":4433"   (raw QUIC for native SDKs)
//	wtAddr     = ":4434"   (WebTransport for browsers; pass "" to skip)
func (a *App) ListenAddr(ctx context.Context, nativeAddr, wtAddr string, tlsCfg *tls.Config) error {
	errCh := make(chan error, 2)

	go func() { errCh <- a.ListenNative(ctx, nativeAddr, tlsCfg) }()

	if wtAddr != "" {
		go func() { errCh <- a.ListenWebTransport(wtAddr, tlsCfg) }()
	}

	return <-errCh
}

// ListenWithPKI starts the server with automated PKI management.
func (a *App) ListenWithPKI(ctx context.Context, nativeAddr, wtAddr, nodeID string, hosts ...string) error {
	// PKI manager would be used here to load or create identity.
	// This is a high-level integration.
	return nil // Implementation pending
}

// Shutdown waits for all in-flight stream handlers to finish.
func (a *App) Shutdown() { a.wg.Wait() }

func (a *App) handleNativeConn(conn *quicConnWrapper) {
	defer conn.CloseWithError(0, "done")

	c := conn.Conn

	connCtx := c.Context()
	for {
		stream, err := c.AcceptStream(connCtx)
		if err != nil {
			return // connection closed or context cancelled
		}
		a.wg.Add(1)
		go func(s *quic.Stream) {
			defer a.wg.Done()
			defer s.Close()
			a.dispatchStream(connCtx, conn, s)
		}(stream)
	}
}

// ─── WebTransport session handler ───────────────────────────────────────────

func (a *App) handleWebTransportSession(session *wtSessionWrapper) {
	ctx := session.Context()
	for {
		stream, err := session.AcceptStream(ctx)
		if err != nil {
			return
		}
		a.wg.Add(1)
		go func(s *webtransport.Stream) {
			defer a.wg.Done()
			defer s.Close()
			a.dispatchStream(ctx, session, s)
		}(stream)
	}
}

// ─── Shared stream dispatcher ────────────────────────────────────────────────

// dispatchStream is the hot path shared by both transports.
// It reads one request frame, routes it through the middleware + handler
// chain, and writes back the response.
func (a *App) dispatchStream(ctx context.Context, peer remoteAddrProvider, stream io.ReadWriter) {
	ft, data, err := protocol.ReadFrame(stream)
	if err != nil {
		a.logger.Debug("quicframe: read frame error", "err", err)
		return
	}

	switch ft {
	case protocol.FrameTypePing:
		_ = protocol.WriteFrame(stream, protocol.FrameTypePong, &protocol.PingFrame{})
		return
	case protocol.FrameTypeRequest:
		// handled below
	default:
		a.logger.Warn("quicframe: unexpected frame type from client", "type", ft)
		return
	}

	req, err := protocol.DecodeRequest(data)
	if err != nil {
		a.logger.Error("quicframe: decode request", "err", err)
		_ = protocol.WriteError(stream, "", protocol.StatusBadRequest, "malformed request frame")
		return
	}

	qfCtx := newContext(ctx, req, peer, stream)
	handler, params := a.router.match(req.Method, req.Path)
	qfCtx.setParams(params)

	// App-level middleware wraps the (already-middleware-wrapped) route handler.
	h := applyMiddleware(handler, a.middleware)

	if err := h(qfCtx); err != nil {
		if !qfCtx.sent.Load() {
			_ = protocol.WriteError(stream, req.ID, protocol.StatusInternalServerError, err.Error())
		}
		a.logger.Error("quicframe: handler returned error",
			"method", req.Method, "path", req.Path, "err", err)
	}
}

// ─── Configuration helpers ───────────────────────────────────────────────────

// Logger returns the application-level slog logger.
func (a *App) Logger() *slog.Logger { return a.logger }

// SetLogger replaces the application logger.
func (a *App) SetLogger(l *slog.Logger) { a.logger = l }

// SetQUICConfig replaces the QUIC transport configuration used by both listeners.
func (a *App) SetQUICConfig(cfg *quic.Config) { a.quicCfg = cfg }

// ─── Internal helpers ────────────────────────────────────────────────────────

type quicConnWrapper struct{ *quic.Conn }

// ControlTransport defines the interface for a secure control channel.
type ControlTransport interface {
	AcceptStream(context.Context) (quic.Stream, error)
	OpenStream() (quic.Stream, error)
	CloseWithError(uint64, string) error
	RemoteAddr() net.Addr
}

// NOTE: remoteAddrProvider is defined in context.go

func (w *quicConnWrapper) PeerCertificates() []*x509.Certificate {
	return w.Conn.ConnectionState().TLS.PeerCertificates
}

func (w *quicConnWrapper) CloseWithError(code uint64, msg string) error {
	return w.Conn.CloseWithError(quic.ApplicationErrorCode(code), msg)
}

type wtSessionWrapper struct{ *webtransport.Session }

func (w *wtSessionWrapper) PeerCertificates() []*x509.Certificate {
	return nil
}

func (w *wtSessionWrapper) CloseWithError(code uint64, msg string) error {
	return w.Session.CloseWithError(webtransport.SessionErrorCode(code), msg)
}

// prepend inserts elem at position 0, removing any prior duplicate.
func prepend(elem string, slice []string) []string {
	out := make([]string, 0, len(slice)+1)
	out = append(out, elem)
	for _, s := range slice {
		if s != elem {
			out = append(out, s)
		}
	}
	return out
}
