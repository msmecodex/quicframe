# Architecture

QuicFrame is split into a small core and a few focused supporting packages so it can be imported cleanly as a dependency in another Go application.

## High-Level Flow

1. A client opens a QUIC bidirectional stream.
2. The server reads one frame from that stream.
3. The frame is decoded into a `protocol.Request`.
4. The router resolves a handler and path params.
5. App middleware wraps the matched route handler.
6. The handler writes either:
   - a normal response frame,
   - an error frame, or
   - a streaming response made of multiple frames.

## Packages

## `quicframe`

The root package contains:

- `App` for listener startup and top-level configuration
- `Router` and `Group` for route registration
- `Context` for request and response helpers
- `StreamWriter` for streaming responses

## `protocol`

The `protocol` package defines the wire contract:

- frame types
- request and response structs
- stream and error frames
- MsgPack encoding helpers

## `middleware`

The middleware package contains reusable production helpers:

- request logging
- panic recovery
- JWT auth
- rate limiting

## `tlsutil`

The TLS package keeps certificate setup out of app code:

- self-signed certificates for local work
- PEM file loading
- Let's Encrypt autocert support

## Transport Model

QuicFrame supports two entry points:

- Native QUIC with ALPN `qf/1`
- WebTransport over HTTP/3

Both paths eventually call the same internal stream dispatcher, so handlers and middleware do not need to care which transport a request came from.

## Request Lifecycle

For each accepted stream:

- the server reads a frame with `protocol.ReadFrame`
- ping frames are answered immediately with pong
- request frames are decoded and wrapped in a `Context`
- the router matches by method and path
- handlers write one response per request

If a handler returns an error without having written a response, the framework emits an error frame with a `500` status-style code.

## Middleware Order

Middleware is applied in two layers:

- route-group middleware is attached when the route is registered
- app-level middleware wraps the final route handler during dispatch

Within each layer, the first middleware you pass becomes the outermost wrapper.

## Streaming Model

Streaming is server-to-client. A handler calls `Context.NewStream`, which:

- sends an initial response frame with `stream=true`
- returns a `StreamWriter`
- allows the handler to write chunk frames in sequence
- ends with a final `StreamEnd` frame on `Close()`

Backpressure is delegated to QUIC flow control, so `Write` naturally slows if the receiver cannot keep up.

## Deployment Shape

In a real application, QuicFrame sits at the edge of your service:

- handlers call into your domain layer
- middleware handles cross-cutting concerns
- clients connect over native QUIC or WebTransport

That makes it suitable as a framework dependency rather than a monolithic app template.
