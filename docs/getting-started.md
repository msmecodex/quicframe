# Getting Started

QuicFrame is a Go module that you can add to any application with `go get`, then use to build QUIC-native APIs, browser-facing WebTransport endpoints, and streaming services with a single handler model.

Repository:

- `https://github.com/msmecodex/quicframe/`

## Requirements

| Tool | Version |
| --- | --- |
| Go | 1.24+ |
| Rust | Optional, for the Rust SDK |
| Node.js | Optional, for the JS SDK |

## Install

Current Go module path:

```bash
go get github.com/msmecodex/quicframe@latest
```

Note:

- the source repository is `https://github.com/msmecodex/quicframe/`
- the Go module path is `github.com/msmecodex/quicframe`

Typical imports:

```go
import (
    qf "github.com/msmecodex/quicframe"
    "github.com/msmecodex/quicframe/middleware"
    "github.com/msmecodex/quicframe/tlsutil"
)
```

## Minimal Server

```go
package main

import (
    "context"
    "log"

    qf "github.com/msmecodex/quicframe"
    "github.com/msmecodex/quicframe/middleware"
    "github.com/msmecodex/quicframe/tlsutil"
)

func main() {
    app := qf.New()
    app.Use(middleware.Recovery(), middleware.Logger())

    app.GET("/ping", func(c *qf.Context) error {
        return c.MsgPack(200, map[string]string{"pong": "ok"})
    })

    app.GET("/users/:id", func(c *qf.Context) error {
        return c.MsgPack(200, map[string]string{
            "id": c.Param("id"),
        })
    })

    app.POST("/users", func(c *qf.Context) error {
        var body map[string]interface{}
        if err := c.Bind(&body); err != nil {
            return c.Error(400, err.Error())
        }
        body["created"] = true
        return c.MsgPack(201, body)
    })

    tlsCfg, err := tlsutil.LoadOrCreateSelfSigned(
        ".local/certs/myapp/localhost-cert.pem",
        ".local/certs/myapp/localhost-key.pem",
        "localhost",
        "127.0.0.1",
    )
    if err != nil {
        log.Fatal(err)
    }

    if err := app.ListenAddr(context.Background(), ":4433", ":4434", tlsCfg); err != nil {
        log.Fatal(err)
    }
}
```

This starts:

- Native QUIC on `:4433`
- WebTransport on `:4434` at `/wt`

## Project Layout for Consumers

One common way to use QuicFrame in another app:

```text
myapp/
├── go.mod
├── main.go
├── handlers/
├── middleware/
└── internal/
```

Your app owns business logic and data access. QuicFrame handles transport, routing, request decoding, and response framing.

## Common Next Steps

1. Add global middleware with `app.Use(...)`.
2. Create grouped routes with `app.Group("/api")`.
3. Use `tlsutil.SelfSigned` for local development.
4. Switch to `tlsutil.Autocert` or `tlsutil.FromFiles` for deployment.
5. Add a Rust or JS client if you need native or browser consumers.

## Examples in This Repository

```bash
git clone https://github.com/msmecodex/quicframe/
cd quicframe

go run ./examples/basic-server
go run ./examples/streaming-demo
go run ./cmd/example
```

React browser example:

```bash
go run ./examples/basic-server
cd examples/react-basic
npm install
npm run dev
```

When the basic server starts, it logs `webtransport_cert_sha256`. Paste that hash into `examples/react-basic/src/App.jsx` for local WebTransport with the generated self-signed certificate.

The React example also enables console debug logs for QuicFrame requests, and the basic server keeps submitted `POST /users` names in memory so the updated list is visible on the next `GET /users`.

## Related Guides

- [architecture.md](architecture.md)
- [routing.md](routing.md)
- [middleware.md](middleware.md)
- [context-api.md](context-api.md)
- [streaming.md](streaming.md)
- [tls.md](tls.md)
- [protocol.md](protocol.md)
- [rust-sdk.md](rust-sdk.md)
- [js-sdk.md](js-sdk.md)
- [configuration.md](configuration.md)
