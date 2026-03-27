# TLS

QUIC requires TLS 1.3, so every QuicFrame server needs a valid `*tls.Config`.

The `tlsutil` package provides three setup paths.

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

Use this for local work only. Native and browser clients may need explicit trust bypass or local certificate trust setup.

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

## Production Notes

- The server must present names that match the client SNI.
- Native QUIC clients must connect with a matching `server_name`.
- Browser WebTransport clients require HTTPS and a certificate the browser accepts.
- Self-signed certificates are usually not enough for public browser usage.

## Recommended Setup by Environment

| Environment | TLS helper |
| --- | --- |
| Local development | `SelfSigned` |
| Existing cert management | `FromFiles` |
| Public internet service | `Autocert` |

## Client Side Considerations

Rust SDK:

- use normal verification in production
- use `skip_cert_verify` only for development

JS SDK:

- WebTransport requires a secure context
- fallback `fetch` mode also benefits from a valid HTTPS certificate
