# Middleware

QuicFrame middleware wraps handlers using the type:

```go
type MiddlewareFunc func(next HandlerFunc) HandlerFunc
```

Middleware is a good place for logging, auth, rate limiting, metrics, and request-scoped values.

## Applying Middleware

Global middleware:

```go
app.Use(
    middleware.Recovery(),
    middleware.Logger(),
)
```

Group middleware:

```go
api := app.Group("/api", middleware.JWTSimple([]byte("secret")))
```

Additional group middleware:

```go
api.Use(middleware.RateLimitSimple(200))
```

## Execution Order

The first middleware you pass is the outermost wrapper.

Example:

```go
app.Use(A, B, C)
```

Execution order becomes:

1. `A`
2. `B`
3. `C`
4. handler

## Logger

`middleware.Logger()` logs:

- request ID
- method
- path
- remote address
- latency
- returned error

You can pass a custom `*slog.Logger`:

```go
app.Use(middleware.Logger(logger))
```

## Recovery

`middleware.Recovery()` catches panics, logs the stack trace, and sends a `500` error frame.

```go
app.Use(middleware.Recovery())
```

This should usually be one of the first global middleware entries.

## JWT Auth

Simple HMAC usage:

```go
app.Use(middleware.JWTSimple([]byte("my-secret")))
```

Custom configuration:

```go
app.Use(middleware.JWT(middleware.JWTConfig{
    SigningKey:    publicKeyOrSecret,
    SigningMethod: jwt.SigningMethodHS256,
    TokenHeader:   "authorization",
}))
```

On success, the parsed token is stored in request locals.

To read default map claims:

```go
claims := middleware.GetClaims(c)
```

If the header is missing, malformed, expired, or invalid, the middleware returns a `401` error frame.

## Rate Limiting

Simple IP-based limiting:

```go
app.Use(middleware.RateLimitSimple(100))
```

Custom config:

```go
app.Use(middleware.RateLimit(middleware.RateLimitConfig{
    RequestsPerSecond: 50,
    Burst:             100,
    KeyFunc: func(c *qf.Context) string {
        return c.Header("x-api-key")
    },
}))
```

The built-in limiter:

- uses a token bucket
- defaults to remote IP if `KeyFunc` is not provided
- periodically evicts idle limiter entries
- returns `429` when the limit is exceeded

## Writing Custom Middleware

```go
func RequireTenant() qf.MiddlewareFunc {
    return func(next qf.HandlerFunc) qf.HandlerFunc {
        return func(c *qf.Context) error {
            tenant := c.Header("x-tenant-id")
            if tenant == "" {
                return c.Error(400, "missing x-tenant-id")
            }
            c.Set("tenant_id", tenant)
            return next(c)
        }
    }
}
```

Tips:

- use `c.Set` and `c.Get` for request-scoped values
- return `c.Error(...)` for client-visible failures
- return a normal `error` for unexpected server failures
