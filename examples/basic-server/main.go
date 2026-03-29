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
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/vmihailenco/msgpack/v5"

	qf "github.com/msmecodex/quicframe"
	"github.com/msmecodex/quicframe/middleware"
	"github.com/msmecodex/quicframe/tlsutil"
)

func main() {
	var (
		usersMu sync.Mutex
		nextID  = 3
		users   = []map[string]string{
			{"id": "1", "name": "Alice"},
			{"id": "2", "name": "Bob"},
		}
	)

	copyUsers := func() []map[string]string {
		usersMu.Lock()
		defer usersMu.Unlock()

		cloned := make([]map[string]string, 0, len(users))
		for _, user := range users {
			cloned = append(cloned, map[string]string{
				"id":   user["id"],
				"name": user["name"],
			})
		}
		return cloned
	}

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
		return c.MsgPack(200, map[string]interface{}{"users": copyUsers()})
	})

	app.GET("/users/:id", func(c *qf.Context) error {
		usersMu.Lock()
		defer usersMu.Unlock()

		for _, user := range users {
			if user["id"] == c.Param("id") {
				return c.MsgPack(200, user)
			}
		}
		return c.Error(404, "user not found")
	})

	app.POST("/users", func(c *qf.Context) error {
		var body struct {
			Name string `msgpack:"name"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Error(400, err.Error())
		}
		if body.Name == "" {
			return c.Error(400, "name is required")
		}

		usersMu.Lock()
		user := map[string]string{
			"id":   fmt.Sprintf("%d", nextID),
			"name": body.Name,
		}
		nextID++
		users = append(users, user)
		usersMu.Unlock()

		return c.MsgPack(201, user)
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
