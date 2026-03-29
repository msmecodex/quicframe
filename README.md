# QuicFrame

**Production-grade QUIC-native API framework for Go** — Express/Fiber-style developer experience over raw QUIC streams with MsgPack binary framing. No HTTP/1.1. No JSON.

```
Client (Rust/Go)  ──── QUIC/qf1 ────┐
                                     ├── QuicFrame Server (Go)
Browser           ── WebTransport ───┘      MsgPack · Router · Middleware
```

---

## Why QuicFrame

| | HTTP/1.1 | HTTP/2 | QuicFrame |
|---|---|---|---|
| Transport | TCP | TCP | **QUIC (UDP)** |
| Multiplexing | ❌ | ✓ (HOL blocking) | **✓ (stream-level)** |
| Head-of-line blocking | ✗ | ✗ | **✓ eliminated** |
| 0-RTT reconnect | ❌ | ❌ | **✓** |
| Encoding | Text/JSON | Text/JSON | **MsgPack (binary)** |
| Streaming | SSE / chunked | Push / chunked | **Native QUIC streams** |
| CGNAT friendly | partial | partial | **✓ (relay-ready)** |

---

## Features

- **Express-style API** — `app.GET`, `app.POST`, `app.Group`, `app.Use`
- **Binary protocol** — MsgPack over QUIC streams, never JSON
- **Two transports** — raw QUIC (`qf/1`) for native clients, WebTransport (HTTP/3) for browsers
- **Routing** — static, dynamic (`:id`), wildcard (`*path`), route groups
- **Middleware** — chainable, Express-style: Logger, Recovery, JWT, Rate Limiter
- **Streaming** — server-push via `StreamWriter`, with backpressure
- **TLS** — self-signed (dev) or Let's Encrypt autocert (production)
- **Rust SDK** — quinn + rmp-serde, connection pool, retry, streaming
- **Browser SDK** — WebTransport + `@msgpack/msgpack`, automatic fetch fallback

---

## Project Structure

```
twaritam/
├── app.go                    # App entry point — both transports
├── router.go                 # Radix router + Group + middleware chaining
├── context.go                # Per-request Context + StreamWriter
│
├── protocol/
│   ├── types.go              # Wire types: Request, Response, StreamChunk…
│   └── codec.go              # WriteFrame / ReadFrame (MsgPack codec)
│
├── middleware/
│   ├── logger.go             # slog-based request logger
│   ├── recovery.go           # Panic → 500 error frame
│   ├── auth.go               # JWT middleware (HS256 / RS256 / ES256)
│   └── ratelimit.go          # Token-bucket rate limiter (per-IP, self-evicting)
│
├── tlsutil/certs.go          # Self-signed + Let's Encrypt helpers
│
├── cmd/example/main.go       # Full-featured demo server
├── examples/
│   ├── basic-server/         # Minimal CRUD server
│   └── streaming-demo/       # Streaming + backpressure demo
│
└── sdk/
    ├── rust/                 # Rust client SDK (quinn)
    │   └── src/
    │       ├── lib.rs
    │       ├── client.rs     # Client, QfResponse, StreamHandle
    │       ├── transport.rs  # Connection pool
    │       ├── protocol.rs   # Wire codec (rmp-serde)
    │       └── error.rs      # QfError
    └── js/                   # Browser client SDK
        └── src/
            ├── client.js     # QuicFrameClient (WebTransport + fetch fallback)
            └── protocol.js   # FrameBuffer, encodeFrame/encodeRequest
```

---

## Wire Protocol

Every message is a length-prefixed frame:

```
┌──────────────────┐
│  FrameLen  (4B)  │  ← big-endian uint32: byte count of Type + Payload
├──────────────────┤
│  FrameType (1B)  │  ← 0x01 Request  0x02 Response  0x03 StreamData
├──────────────────┤     0x04 StreamEnd  0x05 Error  0x06 Ping  0x07 Pong
│  Payload (msgpack│  ← msgpack-encoded struct (never JSON)
└──────────────────┘
```

**Request** (client → server):
```
id      string             # client-generated UUID
method  string             # GET | POST | PUT | DELETE | PATCH
path    string             # /users/42
headers map[string]string  # arbitrary metadata (JWT, content-type…)
body    bytes              # msgpack-encoded request body
stream  bool               # true = server should stream the response
```

**Response** (server → client):
```
id      string             # echoes request ID
status  int                # 200, 404, 500… (HTTP-semantic codes)
headers map[string]string
body    bytes              # msgpack-encoded response body
stream  bool               # true = StreamData frames follow
```

For streaming responses the server emits a chain of `StreamData` frames ending with `StreamEnd`:
```
StreamChunk { id, seq uint64, data bytes, final bool }
```

---

## Getting Started

### Prerequisites

- Go 1.24+
- Rust 1.80+ (for Rust SDK)
- Node.js 20+ (for browser SDK)

### Run the example server

```bash
git clone https://github.com/msmecodex/quicframe/
cd twaritam

go run ./cmd/example
# Native QUIC: :4433   (ALPN qf/1)
# WebTransport: :4434  (ALPN h3, endpoint /wt)
```

### Run the basic server

```bash
go run ./examples/basic-server
```

### Run the streaming demo

```bash
go run ./examples/streaming-demo
# GET /stream/ticks     — tick every 200 ms for 60 s
# GET /stream/batch/:n  — n items as fast as possible
# GET /stream/slow/:n   — 1 item/s (backpressure demo)
```

---

## Go Server API

### App

```go
app := quicframe.New()

// Application-level middleware (runs before every handler)
app.Use(
    middleware.Recovery(),
    middleware.Logger(),
    middleware.RateLimitSimple(500), // 500 req/s per IP
)

// Register routes
app.GET("/ping", pingHandler)
app.POST("/users", createUser)
app.PUT("/users/:id", updateUser)
app.DELETE("/users/:id", deleteUser)
app.GET("/files/*path", serveFile)  // wildcard

// Route groups (shared prefix + middleware)
api := app.Group("/api/v1", middleware.JWTSimple([]byte("secret")))
api.GET("/profile", profileHandler)

// Start both transports
tlsCfg, _ := tlsutil.SelfSigned("localhost")
app.ListenAddr(ctx, ":4433", ":4434", tlsCfg)
```

### Context

```go
func handler(c *quicframe.Context) error {
    // Request
    c.Method()      // "GET"
    c.Path()        // "/users/42"
    c.Param("id")   // "42"
    c.Header("authorization")
    c.RequestID()
    c.RemoteAddr()

    // Decode msgpack body
    var payload MyStruct
    c.Bind(&payload)

    // Locals (middleware → handler communication)
    c.Set("user_id", "abc")
    v, _ := c.Get("user_id")

    // Responses
    c.MsgPack(200, map[string]string{"ok": "true"})  // encode + send
    c.Send(200, rawBytes)
    c.NoContent()
    c.Error(404, "not found")

    // Streaming response
    sw, _ := c.NewStream(200, nil)
    defer sw.Close()
    sw.Write(chunk1)
    sw.Write(chunk2)
    return nil
}
```

### Middleware

```go
// Signature
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// Built-in
middleware.Logger()                        // slog request log
middleware.Recovery()                      // panic → 500
middleware.RateLimitSimple(100)            // 100 req/s per IP
middleware.JWTSimple([]byte("secret"))     // Bearer token validation

// Custom middleware
func Tracing() quicframe.MiddlewareFunc {
    return func(next quicframe.HandlerFunc) quicframe.HandlerFunc {
        return func(c *quicframe.Context) error {
            c.Set("trace_id", uuid.New().String())
            return next(c)
        }
    }
}
```

### TLS

```go
// Development — self-signed
tlsCfg, err := tlsutil.SelfSigned("localhost", "127.0.0.1")

// Production — Let's Encrypt
tlsCfg, manager, err := tlsutil.Autocert(tlsutil.AutocertConfig{
    Domains:  []string{"api.example.com"},
    CacheDir: "/var/cache/quicframe",
    Email:    "ops@example.com",
})

// Bring your own cert
tlsCfg, err := tlsutil.FromFiles("cert.pem", "key.pem")
```

---

## Rust SDK

```toml
# Cargo.toml
[dependencies]
quicframe-client = { path = "sdk/rust" }
tokio = { version = "1", features = ["full"] }
```

```rust
use quicframe::{Client, ClientConfig, PoolConfig};
use std::collections::HashMap;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut cfg = ClientConfig::default();
    cfg.pool.skip_cert_verify = true; // dev only

    let client = Client::connect("127.0.0.1:4433", "localhost", cfg).await?;

    // GET
    let resp = client.get("/ping", HashMap::new()).await?;
    println!("status={}", resp.status);
    let body: HashMap<String, String> = resp.decode()?;

    // POST with msgpack body
    let user = std::collections::HashMap::from([("name", "Alice")]);
    let resp = client.post("/api/v1/users", &user, headers).await?;

    // Streaming
    let (meta, mut stream) = client
        .stream("GET", "/stream/10", HashMap::new(), vec![])
        .await?;
    while let Some(chunk) = stream.next().await? {
        println!("chunk data={:?}", chunk.data);
    }

    // Ping (RTT measurement)
    let rtt = client.ping().await?;
    println!("RTT = {:?}", rtt);

    client.close().await;
    Ok(())
}
```

Run the bundled example:

```bash
cd sdk/rust
cargo run --bin qf-example -- --addr 127.0.0.1:4433
```

---

## Browser SDK (WebTransport)

```bash
cd sdk/js
npm install
npm run build
```

```js
import { QuicFrameClient } from '@quicframe/client';

const client = new QuicFrameClient('https://api.example.com:4434/wt', {
    defaultHeaders: { authorization: 'Bearer <token>' },
    fallbackBase:   'https://api.example.com',   // fetch fallback
    webTransportOptions: {
        serverCertificateHashes: [
            {
                algorithm: 'sha-256',
                value: new Uint8Array([/* local cert hash bytes */]),
            },
        ],
    },
});
await client.connect();

// GET
const resp = await client.get('/ping');
console.log(resp.status, resp.decode()); // decoded from MsgPack

// POST (body is MsgPack-encoded automatically)
await client.post('/api/v1/users', { name: 'Alice' });

// Streaming
const stream = await client.stream('GET', '/events');
for await (const event of stream) {
    console.log(event); // each chunk decoded from MsgPack
}

// Latency
const rtt = await client.ping();
console.log(`RTT = ${rtt.toFixed(1)} ms`);

await client.close();
```

The client automatically falls back to `fetch` (HTTP with `application/x-msgpack`) if `WebTransport` is unavailable in the browser.

For a runnable browser demo, see `examples/react-basic`:

```bash
go run ./examples/basic-server
cd examples/react-basic
npm install
npm run dev
```

The basic server prints a `webtransport_cert_sha256` value on startup. Paste that into `examples/react-basic/src/App.jsx` when using the local self-signed certificate.

---

## Transport Architecture

```
                    ┌─────────────────────────────────┐
                    │         QuicFrame Server         │
                    │                                  │
Rust/Go Client ─────┤  :4433  QUIC (ALPN qf/1)        │
                    │    AcceptStream → dispatchStream  │
                    │                                  │
Browser ────────────┤  :4434  HTTP/3 + WebTransport    │
                    │    /wt endpoint                   │
                    │    AcceptStream → dispatchStream  │
                    │                                  │
                    │  ┌──────────────────────────┐   │
                    │  │  Router + Middleware chain │   │
                    │  │  MsgPack codec            │   │
                    │  └──────────────────────────┘   │
                    └─────────────────────────────────┘
```

Both transports converge on the same `dispatchStream` function — the routing, middleware, and handlers are identical regardless of whether the connection came from a native client or a browser.

---

## Status Codes

QuicFrame uses HTTP-semantic status codes for developer familiarity:

| Code | Meaning |
|------|---------|
| 200 | OK |
| 201 | Created |
| 204 | No Content |
| 400 | Bad Request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not Found |
| 405 | Method Not Allowed |
| 429 | Too Many Requests |
| 500 | Internal Server Error |
| 503 | Service Unavailable |

---

## Performance Considerations

- **QUIC multiplexing** — thousands of concurrent streams per connection; no per-request connection overhead
- **MsgPack** — ~30% smaller payloads than JSON; zero-allocation decode possible with pre-allocated structs
- **Connection pooling** (Rust SDK) — configurable pool of up to N connections; round-robin stream distribution
- **Goroutine-per-stream** — each QUIC stream dispatches to a lightweight goroutine; the scheduler handles backpressure naturally
- **0-RTT on reconnect** — QUIC session tickets allow resuming sessions with zero round trips after a brief disconnect
- **UDP / CGNAT** — QUIC's UDP base works through most NATs; for carrier-grade NAT environments pair with a QUIC relay (e.g. MASQUE)

---

## Scalability

- **Horizontal** — stateless handlers; put a QUIC-aware load balancer (HAProxy 2.9+, Nginx with QUIC, Caddy) in front
- **Connection migration** — QUIC supports connection migration on IP change (mobile roaming) without re-establishing sessions
- **Backpressure** — QUIC's flow-control windows propagate backpressure from slow consumers to `StreamWriter.Write` automatically
- **TLS session tickets** — Let's Encrypt certs rotate automatically; `autocert.Manager` handles renewal transparently

---

## Dependencies

### Go server

| Package | Purpose |
|---------|---------|
| `github.com/quic-go/quic-go` | QUIC transport |
| `github.com/quic-go/webtransport-go` | WebTransport / HTTP3 |
| `github.com/vmihailenco/msgpack/v5` | MsgPack serialisation |
| `github.com/golang-jwt/jwt/v5` | JWT middleware |
| `golang.org/x/crypto/acme/autocert` | Let's Encrypt TLS |
| `golang.org/x/time/rate` | Rate limiter |

### Rust SDK

| Crate | Purpose |
|-------|---------|
| `quinn` | QUIC transport |
| `rustls` | TLS 1.3 |
| `rmp-serde` | MsgPack codec |
| `tokio` | Async runtime |
| `uuid` | Request IDs |
| `thiserror` | Error types |

### Browser SDK

| Package | Purpose |
|---------|---------|
| `@msgpack/msgpack` | MsgPack encode/decode |
| Native `WebTransport` | QUIC transport (browser API) |
| Native `fetch` | Fallback transport |

---

## License

MIT
