// Package quicframe provides a QUIC-native application framework for Go.
//
// QuicFrame is designed to be used as a normal Go module dependency:
//
//	go get github.com/twaritam/quicframe@latest
//
// It exposes an Express-style API for registering routes, attaching
// middleware, and returning MsgPack responses over QUIC streams.
//
// The framework supports two transports with the same handler surface:
//
//   - Native QUIC with ALPN "qf/1" for Go, Rust, and other native clients
//   - WebTransport over HTTP/3 for browser clients
//
// # Quick Start
//
//	package main
//
//	import (
//		"context"
//		"log"
//
//		qf "github.com/twaritam/quicframe"
//		"github.com/twaritam/quicframe/middleware"
//		"github.com/twaritam/quicframe/tlsutil"
//	)
//
//	func main() {
//		app := qf.New()
//		app.Use(middleware.Recovery(), middleware.Logger())
//
//		app.GET("/ping", func(c *qf.Context) error {
//			return c.MsgPack(200, map[string]string{"pong": "ok"})
//		})
//
//		tlsCfg, err := tlsutil.SelfSigned("localhost", "127.0.0.1")
//		if err != nil {
//			log.Fatal(err)
//		}
//
//		if err := app.ListenAddr(context.Background(), ":4433", ":4434", tlsCfg); err != nil {
//			log.Fatal(err)
//		}
//	}
//
// # Core Concepts
//
// Each incoming request is carried on a QUIC bidirectional stream, decoded into
// a [Context], routed through middleware, and completed with either a response
// frame, an error frame, or a streamed response sequence.
//
// # Packages
//
//   - [github.com/twaritam/quicframe/protocol] defines wire frames and codec helpers
//   - [github.com/twaritam/quicframe/middleware] provides logger, recovery, JWT, and rate limiting middleware
//   - [github.com/twaritam/quicframe/tlsutil] provides TLS helpers for development and production
//
// Additional guides live in the repository's docs directory.
package quicframe
