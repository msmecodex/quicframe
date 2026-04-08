# TLS

QUIC requires TLS 1.3, so every QuicFrame server needs a valid `*tls.Config`.

The `tlsutil` package provides three setup paths, while the framework's `pki` module provides a fully automated path for distributed nodes.

Examples in this guide assume:

```go
import "github.com/msmecodex/quicframe/tlsutil"
```

## Self-Signed Certificates

Best for local development and demos:

```go
tlsCfg, err := tlsutil.SelfSigned("localhost", "127.0.0.1")
if err != nil {
    return err
}
```

Behavior:

- generates an in-memory ECDSA P-256 certificate
- supports DNS names and IP SANs from the `hosts` arguments
- sets `MinVersion` to TLS 1.3
- uses a short validity window suitable for browser WebTransport certificate pinning

Use this for local work only. Native and browser clients may need explicit trust bypass or local certificate trust setup.

If you want a stable local certificate across restarts, persist it:

```go
tlsCfg, err := tlsutil.LoadOrCreateSelfSigned(
    ".local/certs/basic-server/localhost-cert.pem",
    ".local/certs/basic-server/localhost-key.pem",
    "localhost",
    "127.0.0.1",
)
if err != nil {
    return err
}
```

That keeps the same certificate on disk unless it is no longer suitable for local WebTransport development.

## Loading PEM Files

If you already have a certificate and key:

```go
tlsCfg, err := tlsutil.FromFiles("server.crt", "server.key")
if err != nil {
    return err
}
```

This is a good fit for:

- reverse-proxy-free deployments
- containerized workloads with mounted certs
- internal PKI environments

## Let's Encrypt Autocert

For internet-facing production services:

```go
tlsCfg, manager, err := tlsutil.Autocert(tlsutil.AutocertConfig{
    Domains:  []string{"api.example.com"},
    CacheDir: "./autocert-cache",
    Email:    "ops@example.com",
})
_ = manager
```

Options:

- `Domains` is required
- `CacheDir` defaults to `/var/cache/quicframe/autocert`
- `Email` is optional but recommended
- `Staging` switches to the Let's Encrypt staging environment

## Automated PKI (Self-Managed)

For distributed systems and control channels, QuicFrame can automatically manage certificates using the `pki` module. This handles CSR generation, signing, persistence, and rolling 14-day validity updates for WebTransport.

```go
// Start server with automated PKI and mTLS
err := app.ListenWithPKI(ctx, ":4433", ":4434", "satellite-01", "localhost")
```

See the [mTLS Guide](middleware.md#mtlsauth) for securing connections using these certificates.

## Production Notes

- The server must present names that match the client SNI.
- Native QUIC clients must connect with a matching `server_name`.
- Browser WebTransport clients require HTTPS and a certificate the browser accepts.
- Self-signed certificates are usually not enough for public browser usage.

## Recommended Setup by Environment

| Environment | TLS helper |
| --- | --- |
| Local development | `SelfSigned` |
| Distributed nodes / Control channels | **Automated PKI** |
| Existing cert management | `FromFiles` |
| Public internet service | `Autocert` |

## Client Side Considerations

Rust SDK:

- use normal verification in production
- use `skip_cert_verify` only for development

JS SDK:

- WebTransport requires a secure context
- fallback `fetch` mode also benefits from a valid HTTPS certificate
- local self-signed WebTransport flows can use `serverCertificateHashes`
- hash-pinned dev certificates need to be short-lived for Chrome compatibility
