package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	qf "github.com/msmecodex/quicframe"
	"github.com/msmecodex/quicframe/protocol"
)

// Recovery returns middleware that catches panics in downstream handlers,
// logs the stack trace, and returns a 500 error frame to the client.
func Recovery(logger ...*slog.Logger) qf.MiddlewareFunc {
	var log *slog.Logger
	if len(logger) > 0 && logger[0] != nil {
		log = logger[0]
	} else {
		log = slog.Default()
	}

	return func(next qf.HandlerFunc) qf.HandlerFunc {
		return func(ctx *qf.Context) (retErr error) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					log.Error("quicframe: panic recovered",
						"panic", r,
						"stack", string(stack),
						"path", ctx.Path(),
						"id", ctx.RequestID(),
					)
					retErr = ctx.Error(
						protocol.StatusInternalServerError,
						fmt.Sprintf("internal server error: %v", r),
					)
				}
			}()
			return next(ctx)
		}
	}
}
