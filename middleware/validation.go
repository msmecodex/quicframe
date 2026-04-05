package middleware

import (
	"strings"

	qf "github.com/msmecodex/quicframe/framework"
	"github.com/msmecodex/quicframe/protocol"
)

// HeaderRule validates a single header value.
type HeaderRule func(value string) error

// RequireHeaders ensures the provided headers are present and non-empty.
func RequireHeaders(names ...string) qf.MiddlewareFunc {
	return func(next qf.HandlerFunc) qf.HandlerFunc {
		return func(c *qf.Context) error {
			for _, name := range names {
				if strings.TrimSpace(c.Header(name)) == "" {
					return c.Error(protocol.StatusBadRequest, "missing required header: "+name)
				}
			}
			return next(c)
		}
	}
}

// ValidateHeaders runs a set of custom validation rules against request headers.
func ValidateHeaders(rules map[string]HeaderRule) qf.MiddlewareFunc {
	return func(next qf.HandlerFunc) qf.HandlerFunc {
		return func(c *qf.Context) error {
			for name, rule := range rules {
				value := c.Header(name)
				if err := rule(value); err != nil {
					return c.Error(protocol.StatusBadRequest, err.Error())
				}
			}
			return next(c)
		}
	}
}
