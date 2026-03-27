# Rust SDK

The Rust SDK lives in `sdk/rust` and provides a native QUIC client built on `quinn` and `rmp-serde`.

Crate metadata:

- package name: `quicframe-client`
- library import path: `quicframe`
- repository: `https://github.com/msmecodex/quicframe/`

## Features

- connection pooling
- retry support for transport failures
- unary requests
- streaming responses
- ping support
- MsgPack body encoding and decoding

## Add to a Rust Project

If you publish the crate separately, consumers can add it from crates.io. In the repository form, the source lives under `sdk/rust`.

For local development from this repository:

```bash
git clone https://github.com/msmecodex/quicframe/
```

Then reference the crate by path:

```toml
[dependencies]
quicframe-client = { path = "../quicframe/sdk/rust" }
```

Core dependencies used by the SDK:

- `quinn`
- `tokio`
- `rmp-serde`
- `serde`

## Connect

```rust
use quicframe::{Client, ClientConfig};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let cfg = ClientConfig::default();
    let client = Client::connect("127.0.0.1:4433", "localhost", cfg).await?;
    client.close().await;
    Ok(())
}
```

The second argument is the TLS server name and must match the certificate.

## Development with Self-Signed Certs

```rust
let mut cfg = ClientConfig::default();
cfg.pool.skip_cert_verify = true;
```

Use this only for local development.

## Unary Requests

```rust
let resp = client.get("/ping", Default::default()).await?;
println!("status={}", resp.status);
```

POST and PUT automatically MsgPack-encode serializable bodies:

```rust
let resp = client.post("/users", &payload, Default::default()).await?;
```

## Decoding Responses

`QfResponse.body` contains raw MsgPack bytes.

To decode:

```rust
let value: MyType = resp.decode()?;
```

## Default Headers

You can attach headers to every request:

```rust
let client = client.with_header("authorization", "Bearer token");
```

## Streaming

```rust
let (_meta, mut stream) = client
    .stream("GET", "/events", Default::default(), vec![])
    .await?;

while let Some(chunk) = stream.next().await? {
    println!("seq={} bytes={}", chunk.seq, chunk.data.len());
}
```

`stream(...)` returns:

- initial response metadata
- a `StreamHandle` for consuming chunks

## Ping

```rust
let rtt = client.ping().await?;
println!("latency = {:?}", rtt);
```

## Configuration

`ClientConfig` includes:

- `pool`
- `request_timeout`
- `max_retries`
- `retry_base_delay`

`PoolConfig` includes:

- `max_connections`
- `alpn`
- `idle_timeout`
- `skip_cert_verify`

Default ALPN is `qf/1`.

## Error Model

The SDK exposes `QfError` for:

- I/O failures
- codec errors
- transport read/write errors
- server error frames
- TLS issues
- timeouts
- unexpected frame types
