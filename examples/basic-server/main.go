// Basic server example — the minimal QuicFrame setup.
//
// Run:
//
//	go run ./examples/basic-server
//
// Then connect with the Rust SDK:
//
//	cargo run --bin qf-example -- --addr 127.0.0.1:4433
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/vmihailenco/msgpack/v5"

	qf "github.com/msmecodex/quicframe"
	"github.com/msmecodex/quicframe/middleware"
	"github.com/msmecodex/quicframe/tlsutil"
)

func main() {
	app := qf.New()

	app.Use(
		middleware.Recovery(),
		middleware.Logger(),
		middleware.RateLimitSimple(1000),
	)

	// ── Routes ───────────────────────────────────────────────────────────────
	app.GET("/ping", func(c *qf.Context) error {
		return c.MsgPack(200, map[string]string{"pong": "ok"})
	})

	app.GET("/users", func(c *qf.Context) error {
		users := []map[string]string{
			{"id": "1", "name": "Alice"},
			{"id": "2", "name": "Bob"},
		}
		return c.MsgPack(200, map[string]interface{}{"users": users})
	})

	app.GET("/users/:id", func(c *qf.Context) error {
		return c.MsgPack(200, map[string]string{"id": c.Param("id"), "name": "Alice"})
	})

	app.POST("/users", func(c *qf.Context) error {
		var body map[string]interface{}
		if err := c.Bind(&body); err != nil {
			return c.Error(400, err.Error())
		}
		body["id"] = "new-id"
		return c.MsgPack(201, body)
	})

	app.DELETE("/users/:id", func(c *qf.Context) error {
		return c.NoContent()
	})

	// Wildcard route
	app.GET("/files/*path", func(c *qf.Context) error {
		return c.MsgPack(200, map[string]string{"path": c.Param("path")})
	})

	// ── TLS ───────────────────────────────────────────────────────────────────
	certDir := filepath.Join(".local", "certs", "basic-server")
	certFile := filepath.Join(certDir, "localhost-cert.pem")
	keyFile := filepath.Join(certDir, "localhost-key.pem")

	tlsCfg, err := tlsutil.LoadOrCreateSelfSigned(certFile, keyFile, "localhost", "127.0.0.1")
	if err != nil {
		slog.Error("tls", "err", err)
		os.Exit(1)
	}

	certHash, err := tlsutil.CertificateSHA256Hex(tlsCfg)
	if err != nil {
		slog.Error("tls cert hash", "err", err)
		os.Exit(1)
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	slog.Info("basic server listening",
		"native", ":4433",
		"webtransport", ":4434",
		"cert_file", certFile,
		"webtransport_cert_sha256", certHash,
	)
	if err := app.ListenAddr(ctx, ":4433", ":4434", tlsCfg); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// suppress unused import — msgpack is used transitively via quicframe.
var _ = msgpack.Marshal
