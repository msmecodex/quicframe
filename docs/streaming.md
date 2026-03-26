# Streaming

QuicFrame supports server-to-client streaming over the same QUIC stream that carried the request.

## When to Use It

Streaming works well for:

- live events
- progress updates
- incremental search results
- telemetry feeds
- batched exports

## Starting a Stream

Use `Context.NewStream`:

```go
app.GET("/events", func(c *qf.Context) error {
    sw, err := c.NewStream(200, map[string]string{"x-stream": "events"})
    if err != nil {
        return err
    }
    defer sw.Close()

    for i := 0; i < 5; i++ {
        payload, _ := msgpack.Marshal(map[string]int{"seq": i})
        if _, err := sw.Write(payload); err != nil {
            return nil
        }
    }
    return nil
})
```

`NewStream` immediately sends a response header frame with `stream=true`.

## Writing Chunks

`StreamWriter.Write` sends one `StreamData` frame.

Important details:

- data should already be encoded for your application format
- most handlers use MsgPack per chunk
- `Write` returns an error if the stream is already closed or the peer is gone

## Closing the Stream

Call `Close()` when done:

```go
defer sw.Close()
```

That emits the terminal `StreamEnd` frame.

Calling `Close` more than once is safe.

## Backpressure

QuicFrame relies on QUIC flow control for backpressure. In practice that means:

- a fast sender will slow down when the client cannot keep up
- `Write` may block or fail depending on stream state
- you do not need a separate app-level ack protocol for basic streaming

## Error Handling

A common pattern is to stop quietly if the client disconnects:

```go
if _, err := sw.Write(data); err != nil {
    return nil
}
```

That avoids noisy logs for normal disconnect behavior.

## Client Expectations

A streaming client should:

1. send a request with `stream=true`
2. read the initial response frame
3. confirm `stream=true` in that response
4. read `StreamData` frames until `StreamEnd`

## Examples in This Repository

- `examples/streaming-demo`
- `/stream/:count` in `cmd/example`
- `/events` in `cmd/example`

## Best Practices

- Keep chunk payloads small and regular.
- Include sequence numbers or timestamps in chunk data if the client needs them.
- Close streams explicitly with `defer sw.Close()`.
- Treat disconnect write errors as expected unless they indicate a server bug.
