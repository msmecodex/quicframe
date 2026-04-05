# Routing

QuicFrame provides a simple method-and-path router with support for static paths, named params, wildcards, and groups.

Examples in this guide assume:

```go
import qf "github.com/msmecodex/quicframe/framework"
```

## Registering Routes

```go
app.GET("/ping", pingHandler)
app.POST("/users", createUser)
app.PUT("/users/:id", updateUser)
app.DELETE("/users/:id", deleteUser)
app.PATCH("/users/:id", patchUser)
```

Supported methods in the current API:

- `GET`
- `POST`
- `PUT`
- `DELETE`
- `PATCH`

## Static Routes

Static routes match literal path segments:

```go
app.GET("/health", healthHandler)
app.GET("/api/version", versionHandler)
```

## Named Params

Use `:name` for a single path segment:

```go
app.GET("/users/:id", func(c *qf.Context) error {
    return c.MsgPack(200, map[string]string{
        "id": c.Param("id"),
    })
})
```

`/users/42` makes `c.Param("id") == "42"`.

## Wildcards

Use `*name` to capture the rest of the path:

```go
app.GET("/files/*path", func(c *qf.Context) error {
    return c.MsgPack(200, map[string]string{
        "path": c.Param("path"),
    })
})
```

`/files/assets/images/logo.png` makes `c.Param("path") == "assets/images/logo.png"`.

Wildcards are tail captures. They consume the remainder of the path.

## Route Groups

Groups let you share a prefix and optional middleware:

```go
api := app.Group("/api/v1")
api.GET("/users", listUsers)
api.GET("/users/:id", getUser)
```

With middleware:

```go
api := app.Group("/api/v1", middleware.JWTSimple([]byte("secret")))
api.GET("/profile", profileHandler)
```

## Group Middleware

You can add more middleware after group creation:

```go
api := app.Group("/api")
api.Use(middleware.RateLimitSimple(100))
api.GET("/stats", statsHandler)
```

That middleware is applied only to routes registered through that group.

## Match Behavior

The router:

- matches by path first
- then checks method
- returns `404` if no path matches
- returns `405` if the path matches but the method does not

## Path Normalization

Leading and trailing slashes are trimmed before matching. That means route parsing is segment-based rather than raw-string based.

Examples:

- `"/users/42"` and `"users/42"` split into the same segments internally
- the empty path maps to the root

## Best Practices

- Keep route shapes consistent across versions.
- Use groups for versioning and auth boundaries.
- Prefer params over manual string parsing.
- Use wildcards for file-like or nested resource paths only.
