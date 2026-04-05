// Package middleware provides production-ready middleware for QuicFrame.
package middleware

import (
	"log/slog"
	"time"

	qf "github.com/msmecodex/quicframe/framework"
)

// Logger returns middleware that logs every request using slog.
// If logger is nil the default slog logger is used.
func Logger(logger ...*slog.Logger) qf.MiddlewareFunc {
	var log *slog.Logger
	if len(logger) > 0 && logger[0] != nil {
		log = logger[0]
	} else {
		log = slog.Default()
	}

	return func(next qf.HandlerFunc) qf.HandlerFunc {
		return func(ctx *qf.Context) error {
			start := time.Now()

			err := next(ctx)

			level := slog.LevelInfo
			if err != nil {
				level = slog.LevelError
			}

			log.Log(
				nil, level,
				"request",
				"id", ctx.RequestID(),
				"method", ctx.Method(),
				"path", ctx.Path(),
				"remote", ctx.RemoteAddr(),
				"latency", time.Since(start),
				"err", err,
			)
			return err
		}
	}
}
