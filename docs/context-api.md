# Context API

`Context` represents one request/response exchange. Handlers and middleware use it to inspect the request, read route params, store locals, and send replies.

Examples in this guide assume you imported the framework as:

```go
import qf "github.com/msmecodex/quicframe/framework"
```

## Request Access

### Method and Path

```go
c.Method()
c.Path()
```

### Route Params

```go
id := c.Param("id")
```

### Query Parameters

Use `Query` to access URL query strings (e.g. `?starred=true`):

```go
isStarred := c.Query("starred") // "true"
```

To access all parameters as `url.Values`:

```go
params := c.QueryParams()
limit := params.Get("limit")
```

### Headers

```go
auth := c.Header("authorization")
```

Headers are stored as `map[string]string` and looked up exactly as sent. There is no canonicalization layer in the current implementation, so keep key casing consistent across clients and middleware.

### Request ID

```go
requestID := c.RequestID()
```

### Raw Body

```go
body := c.Body()
```

`Body()` returns raw bytes from the protocol request body.

## Binding MsgPack

Use `Bind` to decode the request body into a Go value:

```go
var payload CreateUserRequest
if err := c.Bind(&payload); err != nil {
    return c.Error(400, err.Error())
}
```

`Bind` assumes the body contains MsgPack bytes.

## Locals

`Context` includes a request-scoped key/value store for middleware and handlers.

### Set

```go
c.Set("user_id", "123")
```

### Get

```go
v, ok := c.Get("user_id")
```

### MustGet

```go
userID := c.MustGet("user_id")
```

`MustGet` panics if the key does not exist, so it is best used only when a previous middleware guarantees the value.

## Remote Address

```go
addr := c.RemoteAddr()
```

Useful for audit logging, allowlists, and rate-limiting strategies.

### Peer Certificates

```go
certs := c.PeerCertificates()
```

Returns the certificate chain provided by the peer during the TLS handshake. This is primarily used for [mTLS authentication](middleware.md#mtls-auth).

### Standard Context

```go
ctx := c.Context()
```

Returns the underlying request-scoped `context.Context`. Use this when calling external libraries or standard library functions (like logging) that require a `context.Context`.

## Sending Responses

### Raw Bytes

```go
return c.Send(200, rawBytes)
```

### MsgPack Response

```go
return c.MsgPack(200, map[string]string{"ok": "true"})
```

`MsgPack` automatically encodes the value and sets `content-type` to `application/x-msgpack`.

### Empty Response

```go
return c.NoContent()
```

This sends status `204`.

### Error Response

```go
return c.Error(404, "not found")
```

This writes an error frame instead of a normal response frame.

## Streaming

To start a streaming response:

```go
sw, err := c.NewStream(200, nil)
if err != nil {
    return err
}
defer sw.Close()
```

Once a response is sent, the context will reject any second response attempt. That includes mixing `MsgPack`, `Send`, `Error`, and `NewStream` in the same request.

## Handler Pattern

Typical pattern:

```go
func createUser(c *qf.Context) error {
    var req CreateUserRequest
    if err := c.Bind(&req); err != nil {
        return c.Error(400, err.Error())
    }

    user := createUserInStore(req)
    return c.MsgPack(201, user)
}
```
