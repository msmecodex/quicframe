# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog and follows semantic versioning for tagged releases.

## [Unreleased]

### Added
- Root package documentation in `doc.go` for `pkg.go.dev` and module consumers.
- End-user guides in `docs/` covering setup, routing, middleware, context usage, streaming, TLS, protocol details, SDKs, architecture, and configuration.
- Initial changelog for repository and release tracking.

## [0.1.0]

### Added
- Core Go framework package with `App`, `Context`, `Group`, `HandlerFunc`, and `MiddlewareFunc`.
- Native QUIC server transport using ALPN `qf/1`.
- WebTransport server transport on HTTP/3 for browser-compatible clients.
- MsgPack-based binary protocol with request, response, stream, error, ping, and pong frames.
- Route matching with static segments, named parameters, wildcards, and route groups.
- Built-in middleware for logging, panic recovery, JWT auth, and per-key rate limiting.
- Streaming response support via `Context.NewStream` and `StreamWriter`.
- TLS helpers for self-signed certificates, file-based cert loading, and Let's Encrypt autocert.
- Example servers in `examples/` and `cmd/example`.
- Rust SDK in `sdk/rust`.
- JavaScript browser SDK in `sdk/js`.
