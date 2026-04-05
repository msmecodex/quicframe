// Command example demonstrates QuicFrame with JWT auth, rate limiting,
// a streaming endpoint, and a wildcard file route.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vmihailenco/msgpack/v5"

	qf "github.com/msmecodex/quicframe/framework"
	"github.com/msmecodex/quicframe/middleware"
	"github.com/msmecodex/quicframe/tlsutil"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	app := qf.New()
	app.SetLogger(logger)

	// ── Application-level middleware ────────────────────────────────────────
	app.Use(
		middleware.Recovery(logger),
		middleware.Logger(logger),
		middleware.RateLimitSimple(500),
	)

	// ── Public routes ────────────────────────────────────────────────────────
	app.GET("/ping", func(ctx *qf.Context) error {
		return ctx.MsgPack(200, map[string]string{"pong": "ok"})
	})

	app.GET("/info", func(ctx *qf.Context) error {
		return ctx.MsgPack(200, map[string]interface{}{
			"framework": "QuicFrame",
			"transport": "QUIC/1",
			"encoding":  "MsgPack",
			"time":      time.Now().UTC().UnixNano(),
		})
	})

	// ── Protected API group ──────────────────────────────────────────────────
	jwtSecret := []byte(envOr("JWT_SECRET", "change-me-in-production"))
	api := app.Group("/api/v1", middleware.JWTSimple(jwtSecret))

	api.GET("/users", func(ctx *qf.Context) error {
		users := []map[string]interface{}{
			{"id": "1", "name": "Alice", "role": "admin"},
			{"id": "2", "name": "Bob", "role": "user"},
		}
		return ctx.MsgPack(200, map[string]interface{}{"users": users, "total": 2})
	})

	api.GET("/users/:id", func(ctx *qf.Context) error {
		return ctx.MsgPack(200, map[string]interface{}{
			"id":   ctx.Param("id"),
			"name": "Alice",
		})
	})

	api.POST("/users", func(ctx *qf.Context) error {
		var payload map[string]interface{}
		if err := ctx.Bind(&payload); err != nil {
			return ctx.Error(400, "invalid body: "+err.Error())
		}
		payload["id"] = fmt.Sprintf("%d", time.Now().UnixNano())
		return ctx.MsgPack(201, payload)
	})

	api.DELETE("/users/:id", func(ctx *qf.Context) error {
		return ctx.NoContent()
	})

	api.POST("/upload", func(ctx *qf.Context) error {
		body := ctx.Body()
		logger.Info("upload received", "bytes", len(body))
		preview := body
		if len(preview) > 8 {
			preview = preview[:8]
		}
		return ctx.MsgPack(200, map[string]interface{}{
			"received_bytes": len(body),
			"preview_hex":    fmt.Sprintf("%x", preview),
		})
	})

	// ── Streaming response ───────────────────────────────────────────────────
	app.GET("/stream/:count", func(ctx *qf.Context) error {
		sw, err := ctx.NewStream(200, map[string]string{"x-format": "msgpack-stream"})
		if err != nil {
			return err
		}
		defer sw.Close()

		count := 5
		fmt.Sscanf(ctx.Param("count"), "%d", &count)
		if count > 100 {
			count = 100
		}

		for i := 0; i < count; i++ {
			data, err := msgpack.Marshal(map[string]interface{}{
				"seq":  i,
				"data": fmt.Sprintf("chunk %d of %d", i+1, count),
				"ts":   time.Now().UnixNano(),
			})
			if err != nil {
				return err
			}
			if _, err := sw.Write(data); err != nil {
				return nil // client gone
			}
			time.Sleep(100 * time.Millisecond)
		}
		return nil
	})

	// ── SSE-style push stream ────────────────────────────────────────────────
	app.GET("/events", func(ctx *qf.Context) error {
		sw, err := ctx.NewStream(200, nil)
		if err != nil {
			return err
		}
		defer sw.Close()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.After(10 * time.Second)

		for seq := 0; ; seq++ {
			select {
			case <-deadline:
				return nil
			case t := <-ticker.C:
				data, err := msgpack.Marshal(map[string]interface{}{
					"event": "tick",
					"seq":   seq,
					"ts":    t.UnixNano(),
				})
				if err != nil {
					return err
				}
				if _, err := sw.Write(data); err != nil {
					return nil // client disconnected
				}
			}
		}
	})

	// ── Wildcard file route ──────────────────────────────────────────────────
	app.GET("/files/*path", func(ctx *qf.Context) error {
		return ctx.MsgPack(200, map[string]string{
			"path": ctx.Param("path"),
			"note": "demo placeholder",
		})
	})

	// ── TLS (self-signed for development) ───────────────────────────────────
	tlsCfg, err := tlsutil.SelfSigned("localhost", "127.0.0.1")
	if err != nil {
		logger.Error("tls setup failed", "err", err)
		os.Exit(1)
	}

	// ── Graceful shutdown ────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		logger.Info("shutdown signal received")
		cancel()
	}()

	nativeAddr := envOr("NATIVE_ADDR", ":4433")
	wtAddr := envOr("WT_ADDR", ":4434")

	logger.Info("QuicFrame example server starting",
		"native_addr", nativeAddr,
		"webtransport_addr", wtAddr,
	)

	if err := app.ListenAddr(ctx, nativeAddr, wtAddr, tlsCfg); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}

	app.Shutdown()
	logger.Info("server stopped cleanly")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
