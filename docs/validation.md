# Validation

QuicFrame supports request validation in two layers:

- body validation with `Context.BindAndValidate`
- header validation with middleware helpers

## Body Validation

Use MsgPack tags for decoding and `validate` tags for constraints:

```go
type CreateUserBody struct {
    Name  string `msgpack:"name" validate:"required,min=2"`
    Email string `msgpack:"email" validate:"required,email"`
    Age   int    `msgpack:"age" validate:"gte=18,lte=120"`
}

func createUser(c *quicframe.Context) error {
    var body CreateUserBody
    if err := c.BindAndValidate(&body); err != nil {
        return c.Error(400, err.Error())
    }

    return c.MsgPack(201, map[string]any{"ok": true})
}
```

If validation fails, `BindAndValidate` returns a `*quicframe.ValidationError`.

## Header Validation

Require presence:

```go
app.Use(middleware.RequireHeaders("x-tenant-id", "x-api-key"))
```

Custom rules:

```go
app.Use(middleware.ValidateHeaders(map[string]middleware.HeaderRule{
    "x-tenant-id": func(value string) error {
        if value == "" {
            return fmt.Errorf("missing required header: x-tenant-id")
        }
        if len(value) < 3 {
            return fmt.Errorf("invalid x-tenant-id")
        }
        return nil
    },
}))
```

## Direct Validation

You can also validate any struct manually:

```go
if err := quicframe.Validate(&body); err != nil {
    return c.Error(400, err.Error())
}
```
