# Configuration

QuicFrame keeps the core API small, but there are still a few important knobs available in the framework and companion packages.

If you are consuming the framework from another Go project, install the framework package path:

```bash
go get github.com/msmecodex/quicframe/framework@latest
```

Use `go install github.com/msmecodex/quicframe@latest` when you want the CLI binary instead of the framework package.

## App Configuration

## Logger

Every app starts with `slog.Default()` as its logger.

```go
logger := slog.Default()
app.SetLogger(logger)
```

You can read it back with:

```go
log := app.Logger()
```

## QUIC Transport

You can replace the default `quic.Config`:

```go
app.SetQUICConfig(&quic.Config{
    MaxIncomingStreams:    8192,
    MaxIncomingUniStreams: 512,
    KeepAlivePeriod:       15 * time.Second,
    MaxIdleTimeout:        2 * time.Minute,
    EnableDatagrams:       true,
})
```

The defaults from `New()` are:

- `MaxIncomingStreams: 4096`
- `MaxIncomingUniStreams: 256`
- `KeepAlivePeriod: 30s`
- `MaxIdleTimeout: 5m`
- `EnableDatagrams: true`

## Listener Selection

You can start either transport independently:

```go
app.ListenNative(ctx, ":4433", tlsCfg)
app.ListenWebTransport(":4434", tlsCfg)
```

Or both together:

```go
app.ListenAddr(ctx, ":4433", ":4434", tlsCfg)
```

Pass an empty string as the WebTransport address to skip it when using `ListenAddr`.

### Direct Connection Handling

If you are using a custom listener or receiving connections from a proxy/relay, you can pass existing QUIC connections directly to the framework:

```go
// Takes ownership of the connection and handles all incoming streams
app.HandleNativeConn(conn)
```

## Middleware Configuration

### JWT

`middleware.JWTConfig`:

- `SigningKey`
- `SigningMethod`
- `TokenHeader`
- `ContextKey`
- `NewClaims`

Defaults:

- signing method: `HS256`
- token header: `authorization`
- context key: `jwt_claims`
- claims type: `jwt.MapClaims`

### Rate Limit

`middleware.RateLimitConfig`:

- `RequestsPerSecond`
- `Burst`
- `KeyFunc`
- `CleanupInterval`
- `IdleTimeout`

Defaults:

- `Burst`: derived from requests per second, minimum `1`
- `KeyFunc`: remote IP address
- `CleanupInterval`: `5m`
- `IdleTimeout`: `10m`

## TLS Configuration

Available helpers:

- `tlsutil.SelfSigned(hosts...)`
- `tlsutil.FromFiles(certFile, keyFile)`
- `tlsutil.Autocert(cfg)`

`tlsutil.AutocertConfig`:

- `Domains`
- `CacheDir`
- `Email`
- `Staging`

## Protocol Limits

The current protocol package defines:

- `MaxFrameSize = 64 * 1024 * 1024`

If your payloads are larger than that, split them into streamed chunks instead of single large response bodies.

## Operational Recommendations

- Use `Recovery` and `Logger` globally.
- Tune `RateLimit` based on real client identity, not just IP, when needed.
- Keep TLS settings strict in production.
- Use streaming for large or incremental results instead of oversized unary responses.
