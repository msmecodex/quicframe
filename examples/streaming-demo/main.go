// Streaming demo — shows server-push streams and backpressure handling.
//
// Run:
//
//	go run ./examples/streaming-demo
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

	qf "github.com/twaritam/quicframe"
	"github.com/twaritam/quicframe/middleware"
	"github.com/twaritam/quicframe/tlsutil"
)

func main() {
	app := qf.New()
	app.Use(middleware.Recovery(), middleware.Logger())

	// ── Tick stream: emits a message every 200 ms for up to 60 s ────────────
	app.GET("/stream/ticks", func(c *qf.Context) error {
		sw, err := c.NewStream(200, map[string]string{"x-stream": "ticks"})
		if err != nil {
			return err
		}
		defer sw.Close()

		ticker  := time.NewTicker(200 * time.Millisecond)
		deadline := time.After(60 * time.Second)
		defer ticker.Stop()

		for i := 0; ; i++ {
			select {
			case <-deadline:
				return nil
			case t := <-ticker.C:
				payload, err := msgpack.Marshal(map[string]interface{}{
					"seq": i,
					"ts":  t.UnixNano(),
					"msg": fmt.Sprintf("tick #%d", i),
				})
				if err != nil {
					return err
				}
				if _, err := sw.Write(payload); err != nil {
					// Client disconnected — exit cleanly.
					return nil
				}
			}
		}
	})

	// ── Batch stream: emits N items as fast as possible ─────────────────────
	app.GET("/stream/batch/:n", func(c *qf.Context) error {
		n := 100
		fmt.Sscanf(c.Param("n"), "%d", &n)
		if n > 10_000 {
			return c.Error(400, "n must be ≤ 10000")
		}

		sw, err := c.NewStream(200, nil)
		if err != nil {
			return err
		}
		defer sw.Close()

		for i := 0; i < n; i++ {
			data, _ := msgpack.Marshal(map[string]interface{}{
				"index": i,
				"value": i * i,
			})
			if _, err := sw.Write(data); err != nil {
				return nil // client gone
			}
		}
		return nil
	})

	// ── Slow stream: backpressure demo (1 item/s) ────────────────────────────
	app.GET("/stream/slow/:n", func(c *qf.Context) error {
		n := 5
		fmt.Sscanf(c.Param("n"), "%d", &n)
		if n > 60 {
			return c.Error(400, "n must be ≤ 60")
		}

		sw, err := c.NewStream(200, nil)
		if err != nil {
			return err
		}
		defer sw.Close()

		for i := 0; i < n; i++ {
			data, _ := msgpack.Marshal(map[string]interface{}{
				"seq": i,
				"msg": fmt.Sprintf("slow chunk %d/%d", i+1, n),
			})
			if _, err := sw.Write(data); err != nil {
				return nil
			}
			time.Sleep(time.Second) // intentional 1 s delay per chunk
		}
		return nil
	})

	// ── Ping ──────────────────────────────────────────────────────────────────
	app.GET("/ping", func(c *qf.Context) error {
		return c.MsgPack(200, map[string]string{"pong": "ok"})
	})

	// ── TLS / start ──────────────────────────────────────────────────────────
	tlsCfg, err := tlsutil.SelfSigned("localhost", "127.0.0.1")
	if err != nil {
		slog.Error("tls", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	slog.Info("streaming demo listening", "native", ":4435", "webtransport", ":4436")
	if err := app.ListenAddr(ctx, ":4435", ":4436", tlsCfg); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
